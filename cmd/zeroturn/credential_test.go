package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/output"
	"github.com/cris-wendler/zeroturn/internal/testutil"
)

const awsKey = "AKIA" + "QWERTYUIOPASDFGH"

func readEvent(t *testing.T, work, path string) string {
	t.Helper()
	return readEventFor(t, work, path, "s")
}

func readEventFor(t *testing.T, work, path, session string) string {
	t.Helper()
	b, err := json.Marshal(map[string]interface{}{
		"session_id": session, "hook_event_name": "PreToolUse", "cwd": work,
		"tool_name": "Read", "tool_input": map[string]interface{}{"file_path": path},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func readGate(t *testing.T, work, path string) result {
	t.Helper()
	return readGateFor(t, work, path, "s")
}

func readGateFor(t *testing.T, work, path, session string) result {
	t.Helper()
	return run(t, work, readEventFor(t, work, path, session), "event", "--harness", "claude", "--event", "PreToolUse")
}

func TestCredentialGateAsksBeforeAFileIsRead(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	testutil.Write(t, work, "deploy.env", "region=eu\nkey="+awsKey+"\n")
	r := readGate(t, work, filepath.Join(work, "deploy.env"))
	if r.code != 0 {
		t.Fatalf("%+v", r)
	}
	d, reason := decision(t, r)
	if d != "ask" {
		t.Fatalf("decision %q", d)
	}
	for _, want := range []string{"deploy.env", "line 2", "aws access key id"} {
		if !strings.Contains(reason, want) {
			t.Errorf("reason %q lacks %q", reason, want)
		}
	}
	if strings.Contains(r.all(), awsKey) {
		t.Fatal("the credential value was printed")
	}
}

func TestCredentialGateDenies(t *testing.T) {
	work, _ := repoWithConfig(t, func(c *config.Config) { c.Guard.Credentials.Mode = config.CredentialDeny })
	testutil.Write(t, work, "id_rsa", "-----BEGIN RSA PRIVATE KEY-----\nabc\n")
	d, reason := decision(t, readGate(t, work, filepath.Join(work, "id_rsa")))
	if d != "deny" || !strings.Contains(reason, "private key") {
		t.Fatalf("decision %q reason %q", d, reason)
	}
}

func TestCredentialGateStaysSilentWhenItShould(t *testing.T) {
	clean, _ := repoWithConfig(t, nil)
	testutil.Write(t, clean, "main.go", "package main\n\nfunc main() {}\n")
	cases := map[string]struct {
		work string
		path string
	}{
		"a file with no credential": {clean, filepath.Join(clean, "main.go")},
		"a file that is not there":  {clean, filepath.Join(clean, "missing.txt")},
		"a directory":               {clean, clean},
	}
	for name, c := range cases {
		if r := readGate(t, c.work, c.path); r.stdout != "" || r.code != 0 {
			t.Errorf("%s: %+v", name, r)
		}
	}

	off, _ := repoWithConfig(t, func(c *config.Config) { c.Guard.Credentials.Mode = config.CredentialOff })
	testutil.Write(t, off, "deploy.env", "key="+awsKey+"\n")
	if r := readGate(t, off, filepath.Join(off, "deploy.env")); r.stdout != "" {
		t.Errorf("mode off produced %q", r.stdout)
	}
}

// The guard reads the file, never the prompt, and stores neither.
func TestCredentialGateStoresNothingFromTheFile(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	testutil.Write(t, work, "secrets/deploy.env", "key="+awsKey+"\ncustomer=Acme Holdings\n")
	readGate(t, work, filepath.Join(work, "secrets", "deploy.env"))
	filepath.Walk(os.Getenv("ZEROTURN_STATE_DIR"), func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		b, _ := os.ReadFile(p)
		for _, forbidden := range []string{awsKey, "Acme Holdings", work} {
			if strings.Contains(string(b), forbidden) {
				t.Errorf("%s holds %q", filepath.Base(p), forbidden)
			}
		}
		return nil
	})
}

func TestCredentialWarningsAreCounted(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	testutil.Write(t, work, "deploy.env", "key="+awsKey+"\n")
	readGate(t, work, filepath.Join(work, "deploy.env"))
	readGate(t, work, filepath.Join(work, "deploy.env"))
	r := run(t, work, "", "report", "current", "--json")
	var rep reportJSON
	if err := json.Unmarshal([]byte(r.stdout), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.CredentialWarnings != 2 {
		t.Fatalf("credential warnings %d, want 2", rep.CredentialWarnings)
	}
}

// A large file is left alone rather than holding up the read.
func TestCredentialGateSkipsLargeFiles(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	big := filepath.Join(work, "big.log")
	if err := os.WriteFile(big, []byte(strings.Repeat("x", maxScan+1)+"\nkey="+awsKey+"\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if r := readGate(t, work, big); r.stdout != "" {
		t.Fatalf("%+v", r)
	}
}

func TestGateStillIgnoresOtherTools(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	for _, tool := range []string{"Bash", "Write", "Grep"} {
		event := `{"session_id":"s","hook_event_name":"PreToolUse","tool_name":"` + tool + `"}`
		if r := run(t, work, event, "event", "--harness", "claude", "--event", "PreToolUse"); r.stdout != "" || r.code != 0 {
			t.Errorf("%s: %+v", tool, r)
		}
	}
}

func promptEvent(t *testing.T, work, message string) string {
	t.Helper()
	b, err := json.Marshal(map[string]interface{}{
		"session_id": "s", "hook_event_name": "UserPromptSubmit", "cwd": work, "prompt": message,
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func promptGateRun(t *testing.T, work, message string) result {
	t.Helper()
	return run(t, work, promptEvent(t, work, message), "event", "--harness", "claude", "--event", "UserPromptSubmit")
}

func TestPromptGuardIsOffUntilSwitchedOn(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	r := promptGateRun(t, work, "deploy with key "+awsKey)
	if r.stdout != "" || r.code != 0 {
		t.Fatalf("the prompt guard acted while off: %+v", r)
	}
}

func TestPromptGuardBlocksACredential(t *testing.T) {
	work, _ := repoWithConfig(t, func(c *config.Config) { c.Guard.Credentials.Prompts = config.PromptsBlock })
	r := promptGateRun(t, work, "here is the key "+awsKey+" for staging")
	if r.code != 0 {
		t.Fatalf("%+v", r)
	}
	var out struct {
		Decision string `json:"decision"`
		Reason   string `json:"reason"`
	}
	if err := json.Unmarshal([]byte(r.stdout), &out); err != nil {
		t.Fatalf("output is not the harness shape: %q", r.stdout)
	}
	if out.Decision != "block" {
		t.Fatalf("decision %q", out.Decision)
	}
	for _, want := range []string{"was not sent", "aws access key id", "policy set guard.credentials.prompts off"} {
		if !strings.Contains(out.Reason, want) {
			t.Errorf("reason %q lacks %q", out.Reason, want)
		}
	}
	if strings.Contains(r.all(), awsKey) {
		t.Fatal("the credential value was printed")
	}
}

func TestPromptGuardLetsOrdinaryMessagesThrough(t *testing.T) {
	work, _ := repoWithConfig(t, func(c *config.Config) { c.Guard.Credentials.Prompts = config.PromptsBlock })
	for _, message := range []string{
		"please fix the failing test in internal/policy",
		"the password field should be validated, see auth.go",
		"",
	} {
		if r := promptGateRun(t, work, message); r.stdout != "" {
			t.Errorf("%q was blocked: %s", message, r.stdout)
		}
	}
}

// The message is scanned in memory. Nothing from it reaches the records.
func TestPromptGuardStoresNothing(t *testing.T) {
	work, _ := repoWithConfig(t, func(c *config.Config) { c.Guard.Credentials.Prompts = config.PromptsBlock })
	promptGateRun(t, work, "ship the Acme Holdings migration with key "+awsKey)
	filepath.Walk(os.Getenv("ZEROTURN_STATE_DIR"), func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		b, _ := os.ReadFile(p)
		for _, forbidden := range []string{awsKey, "Acme Holdings", "migration"} {
			if strings.Contains(string(b), forbidden) {
				t.Errorf("%s holds %q", filepath.Base(p), forbidden)
			}
		}
		return nil
	})
}

// The hook is installed only when the guard is switched on, so a
// developer who has not asked for it never has ZeroTurn reading messages.
func TestPromptHookInstalledOnlyWhenEnabled(t *testing.T) {
	off, _ := repoWithConfig(t, nil)
	r := run(t, off, "", "integrate", "claude", "--plan")
	if strings.Contains(r.stdout, "UserPromptSubmit") {
		t.Errorf("the prompt hook is in the plan while the guard is off:\n%s", r.stdout)
	}
	on, _ := repoWithConfig(t, func(c *config.Config) { c.Guard.Credentials.Prompts = config.PromptsBlock })
	r = run(t, on, "", "integrate", "claude", "--plan")
	if !strings.Contains(r.stdout, "UserPromptSubmit") {
		t.Errorf("the prompt hook is missing while the guard is on:\n%s", r.stdout)
	}
}

func TestSwitchingOnThePromptGuardNeedsConfirmation(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	if r := run(t, work, "", "policy", "set", "guard.credentials.prompts", "block"); r.code != output.ExitDeclined {
		t.Fatalf("without a terminal: %+v", r)
	}
	if c, _ := config.Load(work); c.Guard.Credentials.Prompts != config.PromptsOff {
		t.Fatal("the guard was switched on without confirmation")
	}
	if err := inProcess(t, work, false, func() error {
		return cmdPolicy(context.Background(), []string{"set", "guard.credentials.prompts", "block"})
	}); err == nil {
		t.Fatal("declining still switched it on")
	}
	if err := inProcess(t, work, true, func() error {
		return cmdPolicy(context.Background(), []string{"set", "guard.credentials.prompts", "block"})
	}); err != nil {
		t.Fatal(err)
	}
	if c, _ := config.Load(work); c.Guard.Credentials.Prompts != config.PromptsBlock {
		t.Fatal("confirming did not switch it on")
	}
}
