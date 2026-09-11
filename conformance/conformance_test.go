// Package conformance checks that what ZeroTurn publishes matches the
// schemas it publishes, and that an adapter following the contract is
// accepted while one that breaks it is refused.
//
// Run it with: go test ./conformance
package conformance

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
	"github.com/cris-wendler/zeroturn/internal/jsonschema"
	"github.com/cris-wendler/zeroturn/internal/state"
	"github.com/cris-wendler/zeroturn/internal/testutil"
	"github.com/cris-wendler/zeroturn/internal/trust"
)

var bin string

func TestMain(m *testing.M) {
	dir, err := ioutil.TempDir("", "zeroturn-conformance-")
	if err != nil {
		panic(err)
	}
	bin = filepath.Join(dir, "zeroturn")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	if out, err := exec.Command("go", "build", "-o", bin, "../cmd/zeroturn").CombinedOutput(); err != nil {
		panic(string(out))
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func schema(t *testing.T, name string) *jsonschema.Schema {
	t.Helper()
	b, err := ioutil.ReadFile(filepath.Join("..", "schemas", name))
	if err != nil {
		t.Fatal(err)
	}
	s, err := jsonschema.Parse(b)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return s
}

func mustFollow(t *testing.T, schemaName string, doc []byte) {
	t.Helper()
	if problems := schema(t, schemaName).Validate(doc); len(problems) > 0 {
		t.Errorf("output does not follow %s:\n  %s\n%s", schemaName, strings.Join(problems, "\n  "), doc)
	}
}

func run(t *testing.T, dir, stdin string, args ...string) (string, string, int) {
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
	return out.String(), errb.String(), code
}

// repo prepares a repository with a session record, so that the outputs
// under test carry real values rather than empty ones.
func repo(t *testing.T, mode string) string {
	t.Helper()
	testutil.Isolate(t)
	work, _ := testutil.Remote(t)
	c := config.Default()
	c.Guard.Mode = mode
	c.Verify.Steps = []config.Step{{Name: "ok", Command: []string{"git", "--version"}}}
	if err := config.Save(work, c); err != nil {
		t.Fatal(err)
	}
	payload, err := ioutil.ReadFile(filepath.Join("..", "fixtures", "claude", "statusline-full.json"))
	if err != nil {
		t.Fatal(err)
	}
	// The document is rebuilt with the JSON encoder rather than by
	// replacing text, because a Windows path contains backslashes, which
	// are escape characters inside a JSON string.
	var doc map[string]interface{}
	if err := json.Unmarshal(payload, &doc); err != nil {
		t.Fatal(err)
	}
	doc["cwd"] = work
	if ws, ok := doc["workspace"].(map[string]interface{}); ok {
		ws["current_dir"] = work
		ws["project_dir"] = work
	}
	run(t, work, encode(t, doc), "status", "--stdin", "--harness", "claude")
	return work
}

func TestPublishedOutputFollowsItsSchema(t *testing.T) {
	work := repo(t, config.ModeConfirm)
	cases := []struct {
		schema string
		args   []string
	}{
		{"capabilities.schema.json", []string{"capabilities", "--json"}},
		{"status.schema.json", []string{"status", "--json"}},
		{"policy-check.schema.json", []string{"policy", "check", "--json"}},
		{"report.schema.json", []string{"report", "current", "--json"}},
		{"report.schema.json", []string{"report", "week", "--json"}},
	}
	for _, c := range cases {
		out, errText, code := run(t, work, "", c.args...)
		if code != 0 {
			t.Errorf("%v exited %d: %s", c.args, code, errText)
			continue
		}
		mustFollow(t, c.schema, []byte(out))
	}
}

func TestVerifyOutputFollowsItsSchema(t *testing.T) {
	work := repo(t, config.ModeObserve)
	if _, errText, code := run(t, work, "", "verify", "--json"); code == 0 {
		t.Fatalf("unapproved commands ran: %s", errText)
	}
	// Approval is recorded through the trust store, which the verify
	// command writes after asking. Running with --json here would need a
	// terminal, so the approval is made directly.
	approve(t, work)
	out, errText, code := run(t, work, "", "verify", "--json")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, errText)
	}
	mustFollow(t, "verify.schema.json", []byte(out))
}

func TestDecisionFollowsItsSchema(t *testing.T) {
	work := repo(t, config.ModeConfirm)
	event := encode(t, map[string]interface{}{
		"session_id": "fixture-full", "hook_event_name": "PreToolUse", "cwd": work,
		"tool_name": "Agent", "tool_input": map[string]interface{}{"prompt": "private"},
	})
	out, _, code := run(t, work, event, "event", "--harness", "claude", "--event", "PreToolUse")
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatal("the gate produced no decision for a session past its thresholds")
	}
	mustFollow(t, "decision.schema.json", []byte(out))
	if strings.Contains(out, "private") {
		t.Fatal("the decision carries content from the subagent prompt")
	}
}

func TestConfigurationFollowsItsSchema(t *testing.T) {
	for _, name := range []string{".zeroturn.json", ".zeroturn.example.json"} {
		b, err := ioutil.ReadFile(filepath.Join("..", name))
		if err != nil {
			t.Fatal(err)
		}
		mustFollow(t, "config.schema.json", b)
	}
}

// An adapter that follows the contract is accepted, and the same event
// with a different contract name is refused with a clear message.
func TestNormalizedEventContract(t *testing.T) {
	work := repo(t, config.ModeObserve)
	good, err := ioutil.ReadFile(filepath.Join("..", "fixtures", "normalized", "subagent-pre.json"))
	if err != nil {
		t.Fatal(err)
	}
	mustFollow(t, "normalized-event.schema.json", good)
	if _, errText, code := run(t, work, string(good), "event", "--harness", "normalized"); code != 0 || errText != "" {
		t.Fatalf("a valid normalized event was not accepted: exit %d %s", code, errText)
	}

	bad, err := ioutil.ReadFile(filepath.Join("..", "fixtures", "normalized", "bad-contract.json"))
	if err != nil {
		t.Fatal(err)
	}
	if problems := schema(t, "normalized-event.schema.json").Validate(bad); len(problems) == 0 {
		t.Error("the schema accepted an event with the wrong contract")
	}
	_, errText, code := run(t, work, string(bad), "event", "--harness", "normalized")
	if code != 0 {
		t.Errorf("a refused event must still exit 0, so the harness is not disturbed, got %d", code)
	}
	if !strings.Contains(errText, "zeroturn.event/1") {
		t.Errorf("the refusal does not name the contract this build speaks: %q", errText)
	}
}

// Every schema in schemas/ must be one this validator fully supports,
// otherwise a contract could appear to be checked when it is not.
func TestEverySchemaIsSupported(t *testing.T) {
	names, err := filepath.Glob(filepath.Join("..", "schemas", "*.schema.json"))
	if err != nil || len(names) == 0 {
		t.Fatalf("no schemas found: %v", err)
	}
	for _, name := range names {
		b, err := ioutil.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		s, err := jsonschema.Parse(b)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		for _, p := range s.Validate([]byte(`{}`)) {
			if strings.Contains(p, "does not support") {
				t.Errorf("%s: %s", filepath.Base(name), p)
			}
		}
	}
}

func encode(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func approve(t *testing.T, work string) {
	t.Helper()
	st, err := state.Open()
	if err != nil {
		t.Fatal(err)
	}
	c, err := config.Load(work)
	if err != nil {
		t.Fatal(err)
	}
	if err := trust.Approve(st, work, c); err != nil {
		t.Fatal(err)
	}
}
