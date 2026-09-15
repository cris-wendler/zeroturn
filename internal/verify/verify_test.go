package verify

import (
	"context"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/cris-wendler/zeroturn/internal/config"
)

func repo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	return dir
}

func steps(s ...config.Step) config.Config {
	c := config.Default()
	c.Verify.Steps = s
	return c
}

func sh(name, script string) config.Step {
	return config.Step{Name: name, Command: []string{"sh", "-c", script}}
}

func skipWithoutSh(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses sh to produce controlled output")
	}
	if _, err := exec.LookPath("sh"); err != nil {
		t.Skip("sh not available")
	}
}

func TestPassAndFail(t *testing.T) {
	skipWithoutSh(t)
	root := repo(t)
	res, err := Run(context.Background(), steps(
		sh("ok", "echo fine"),
		sh("bad", "echo broken >&2; exit 3"),
		sh("after", "echo never"),
	), Options{RepoRoot: root})
	if err != nil {
		t.Fatal(err)
	}
	if res.Passed != 1 || res.Failed != 1 || res.Skipped != 1 {
		t.Fatalf("result %+v", res)
	}
	bad := res.Steps[1]
	if bad.Status != StatusFail || bad.ExitCode != 3 || !strings.Contains(bad.Excerpt, "broken") {
		t.Fatalf("failed step %+v", bad)
	}
	if res.Steps[2].Status != StatusSkipped {
		t.Fatal("a step after a failure ran")
	}
}

func TestMissingExecutable(t *testing.T) {
	c := steps(config.Step{Name: "x", Command: []string{"zeroturn-no-such-executable"}})
	if m := MissingExecutables(c); len(m) != 1 {
		t.Fatalf("missing %v", m)
	}
	res, _ := Run(context.Background(), c, Options{RepoRoot: repo(t)})
	if res.Steps[0].Status != StatusMissing || res.Failed != 1 {
		t.Fatalf("result %+v", res)
	}
}

// Arguments reach the program unchanged and are never interpreted by a shell.
func TestNoShellInterpretation(t *testing.T) {
	skipWithoutSh(t)
	root := repo(t)
	marker := filepath.Join(root, "pwned")
	c := steps(config.Step{Name: "echo", Command: []string{"echo", "; touch " + marker, "$(touch " + marker + ")"}})
	res, _ := Run(context.Background(), c, Options{RepoRoot: root})
	if res.Passed != 1 {
		t.Fatalf("result %+v", res)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("an argument was executed by a shell")
	}
}

func TestRunsInRepositoryRoot(t *testing.T) {
	skipWithoutSh(t)
	root := repo(t)
	res, _ := Run(context.Background(), steps(sh("where", "test -d .git")), Options{RepoRoot: root})
	if res.Passed != 1 {
		t.Fatalf("step did not run in the repository root: %+v", res.Steps)
	}
}

func TestLogPreservedAndRedacted(t *testing.T) {
	skipWithoutSh(t)
	root := repo(t)
	secret := "AKIA" + "QWERTYUIOPASDFGH"
	res, _ := Run(context.Background(), steps(sh("leak", "echo line one; echo key "+secret+"; exit 1")),
		Options{RepoRoot: root})
	s := res.Steps[0]
	if !strings.HasPrefix(s.LogFile, LogDir(root)) {
		t.Fatalf("log at %q", s.LogFile)
	}
	b, err := ioutil.ReadFile(s.LogFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "line one") {
		t.Fatalf("log incomplete: %q", b)
	}
	if strings.Contains(string(b), secret) || strings.Contains(s.Excerpt, secret) {
		t.Fatal("credential written to the log or the excerpt")
	}
}

func TestExcerptIsBounded(t *testing.T) {
	skipWithoutSh(t)
	res, _ := Run(context.Background(), steps(sh("noisy", "i=0; while [ $i -lt 500 ]; do echo line $i; i=$((i+1)); done; exit 1")),
		Options{RepoRoot: repo(t)})
	lines := strings.Split(res.Steps[0].Excerpt, "\n")
	if len(lines) > tailLines || !strings.Contains(lines[len(lines)-1], "line 499") {
		t.Fatalf("excerpt has %d lines, last %q", len(lines), lines[len(lines)-1])
	}
}

func TestCancellationStopsChildProcesses(t *testing.T) {
	skipWithoutSh(t)
	root := repo(t)
	marker := filepath.Join(root, "survived")
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()
	start := time.Now()
	res, _ := Run(ctx, steps(
		sh("slow", "(sleep 2; touch "+marker+") & sleep 5"),
		sh("next", "echo never"),
	), Options{RepoRoot: root})
	if time.Since(start) > 3*time.Second {
		t.Fatal("cancellation did not stop the step")
	}
	if !res.Cancelled || res.Steps[0].Status != StatusCancelled || res.Steps[1].Status == StatusPass {
		t.Fatalf("result %+v", res)
	}
	time.Sleep(2500 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("a child process outlived cancellation")
	}
}

// safeName turns a step name into part of a log file name. Checking only
// that the result holds no separator passed for a function that returned
// nothing at all, which would have sent every step to one log file.
func TestSafeName(t *testing.T) {
	if got := safeName("../x y"); strings.ContainsAny(got, "./ ") {
		t.Fatalf("got %q, which can escape the log directory", got)
	}

	// Two names have to stay two names, or one step overwrites another.
	if safeName("vet") == safeName("test") {
		t.Fatal("two step names produced one file name")
	}
	// And enough of the name has to survive to tell the logs apart by eye.
	for name, want := range map[string]string{
		"vet":        "vet",
		"unit tests": "unit-tests",
		"../x y":     "---x-y",
	} {
		if got := safeName(name); got != want {
			t.Errorf("safeName(%q) is %q, want %q", name, got, want)
		}
	}
}

func TestOnStepStreamsEveryResult(t *testing.T) {
	skipWithoutSh(t)
	var seen []string
	Run(context.Background(), steps(sh("a", "exit 0"), sh("b", "exit 1"), sh("c", "exit 0")),
		Options{RepoRoot: repo(t), OnStep: func(s StepResult) { seen = append(seen, s.Name+":"+s.Status) }})
	if strings.Join(seen, ",") != "a:pass,b:fail,c:skipped" {
		t.Fatalf("got %v", seen)
	}
}
