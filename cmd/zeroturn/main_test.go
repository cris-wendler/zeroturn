package main

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/state"
	"github.com/cris-wendler/zeroturn/internal/testutil"
	"github.com/cris-wendler/zeroturn/internal/trust"
)

var bin string

func TestMain(m *testing.M) {
	dir, err := ioutil.TempDir("", "zeroturn-test-bin-")
	if err != nil {
		panic(err)
	}
	bin = filepath.Join(dir, "zeroturn")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", bin, ".").CombinedOutput(); err != nil {
		panic(string(out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

type result struct {
	stdout, stderr string
	code           int
}

func (r result) all() string { return r.stdout + r.stderr }

func run(t *testing.T, dir, stdin string, args ...string) result {
	t.Helper()
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	code := 0
	if ee, ok := err.(*exec.ExitError); ok {
		code = ee.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return result{out.String(), errb.String(), code}
}

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := ioutil.ReadFile(filepath.Join("..", "..", "fixtures", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// repoWithConfig returns a clone on a working branch with a committed
// configuration, so ship is not refused for branch protection.
func repoWithConfig(t *testing.T, mutate func(*config.Config)) (work, bare string) {
	t.Helper()
	testutil.Isolate(t)
	work, bare = testutil.Remote(t)
	c := config.Default()
	if mutate != nil {
		mutate(&c)
	}
	if err := config.Save(work, c); err != nil {
		t.Fatal(err)
	}
	testutil.Git(t, work, "add", config.FileName)
	testutil.Git(t, work, "commit", "--quiet", "--message", "config")
	testutil.Git(t, work, "push", "--quiet", "origin", "main")
	testutil.Git(t, work, "checkout", "--quiet", "-b", "work")
	testutil.Git(t, work, "push", "--quiet", "--set-upstream", "origin", "work")
	return work, bare
}

func claudeEvent(t *testing.T, fixtureName, cwd, sessionID string, edit func(map[string]interface{})) string {
	t.Helper()
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(fixture(t, fixtureName)), &m); err != nil {
		t.Fatal(err)
	}
	m["cwd"] = cwd
	m["session_id"] = sessionID
	if ws, ok := m["workspace"].(map[string]interface{}); ok {
		ws["current_dir"] = cwd
		ws["project_dir"] = cwd
	}
	if edit != nil {
		edit(m)
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func contextAt(pct float64) func(map[string]interface{}) {
	return func(m map[string]interface{}) {
		m["context_window"] = map[string]interface{}{"used_percentage": pct, "context_window_size": 200000}
		delete(m, "rate_limits")
		m["cost"] = map[string]interface{}{"total_duration_ms": 60000}
	}
}

func gate(t *testing.T, dir, sessionID string) result {
	t.Helper()
	return run(t, dir, claudeEvent(t, "claude/pretooluse-agent.json", dir, sessionID, nil),
		"event", "--harness", "claude", "--event", "PreToolUse")
}

func statusline(t *testing.T, dir, sessionID string, edit func(map[string]interface{})) result {
	t.Helper()
	return run(t, dir, claudeEvent(t, "claude/statusline-full.json", dir, sessionID, edit),
		"status", "--stdin", "--harness", "claude")
}

func decision(t *testing.T, r result) (string, string) {
	t.Helper()
	if strings.TrimSpace(r.stdout) == "" {
		return "", ""
	}
	var out decisionOutput
	if err := json.Unmarshal([]byte(r.stdout), &out); err != nil {
		t.Fatalf("gate output is not the harness decision shape: %q", r.stdout)
	}
	if out.HookSpecificOutput.HookEventName != "PreToolUse" {
		t.Fatalf("hook event name %q", out.HookSpecificOutput.HookEventName)
	}
	return out.HookSpecificOutput.PermissionDecision, out.HookSpecificOutput.PermissionDecisionReason
}

func approveStrict(t *testing.T, root string) {
	t.Helper()
	st, err := state.Open()
	if err != nil {
		t.Fatal(err)
	}
	if err := trust.ApproveStrict(st, root); err != nil {
		t.Fatal(err)
	}
}

// inProcess runs fn from dir with confirmation answered by answer and
// standard output discarded. It is used for paths that need a terminal.
func inProcess(t *testing.T, dir string, answer bool, fn func() error) error {
	t.Helper()
	orig := confirm
	confirm = func(string) (bool, error) { return answer, nil }
	wd, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	stdout := os.Stdout
	devnull, _ := os.Open(os.DevNull)
	os.Stdout = devnull
	defer func() {
		os.Stdout = stdout
		devnull.Close()
		os.Chdir(wd)
		confirm = orig
	}()
	return fn()
}
