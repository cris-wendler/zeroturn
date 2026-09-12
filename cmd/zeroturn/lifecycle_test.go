package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/state"
	"github.com/cris-wendler/zeroturn/internal/testutil"
	"github.com/cris-wendler/zeroturn/internal/trust"
)

// TestLifecycle walks one repository from an empty checkout to a pushed
// commit, in the order a developer meets the commands. Each step checks
// what the step before it left behind, so a change that breaks the flow
// between commands fails here even when every command passes on its own.
func TestLifecycle(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the validation steps in this scenario use a POSIX shell")
	}
	testutil.Isolate(t)
	work, bare := testutil.Remote(t)
	testutil.Write(t, work, "go.mod", "module example.com/app\n\ngo 1.17\n")
	testutil.Write(t, work, "app.go", "package app\n")
	testutil.Git(t, work, "add", "-A")
	testutil.Git(t, work, "commit", "--quiet", "--message", "project")
	testutil.Git(t, work, "push", "--quiet")
	testutil.Git(t, work, "checkout", "--quiet", "-b", "feature")
	testutil.Git(t, work, "push", "--quiet", "--set-upstream", "origin", "feature")

	// 1. init detects the project and proposes a configuration.
	r := run(t, work, "", "init", "--yes")
	if r.code != 0 || !strings.Contains(r.stdout, "language") {
		t.Fatalf("init: %+v", r)
	}
	c, err := config.Load(work)
	if err != nil {
		t.Fatalf("init wrote a file that does not load: %v", err)
	}
	if c.Guard.Mode != config.ModeObserve {
		t.Fatalf("init chose mode %q, want the safe default", c.Guard.Mode)
	}

	// 2. the integration is planned, then installed.
	if r := run(t, work, "", "integrate", "claude", "--plan"); r.code != 0 || strings.Contains(r.stdout, "would repair") {
		t.Fatalf("plan: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(work, ".claude", "settings.local.json")); !os.IsNotExist(err) {
		t.Fatal("the plan wrote the settings file")
	}
	if err := integrate(t, work, true, "--apply"); err != nil {
		t.Fatalf("apply: %v", err)
	}

	// 3. a session reports its condition, and the gate stays quiet in
	// observe mode however loaded the session is.
	statusline(t, work, "life", contextAt(95))
	if r := gate(t, work, "life"); r.stdout != "" {
		t.Fatalf("observe mode answered the gate: %q", r.stdout)
	}

	// 4. confirm mode asks, and the reason names the measurement.
	if r := run(t, work, "", "policy", "set", "guard.mode", "confirm"); r.code != 0 {
		t.Fatalf("policy set: %+v", r)
	}
	d, reason := decision(t, gate(t, work, "life"))
	if d != "ask" || !strings.Contains(reason, "95%") {
		t.Fatalf("decision %q reason %q", d, reason)
	}

	// 5. subagent counts come from the harness events and reach the
	// status line and the report.
	for _, id := range []string{"a1", "a2"} {
		agent := id
		run(t, work, claudeEvent(t, "claude/subagent-start.json", work, "life",
			func(m map[string]interface{}) { m["agent_id"] = agent }),
			"event", "--harness", "claude", "--event", "SubagentStart")
	}
	if line := statusline(t, work, "life", contextAt(95)); !strings.Contains(line.stdout, "agents 2") {
		t.Fatalf("status line: %q", line.stdout)
	}

	// 6. the credential guard stops a file from being read.
	testutil.Write(t, work, "deploy.env", "key="+awsKey+"\n")
	if d, _ := decision(t, readGateFor(t, work, filepath.Join(work, "deploy.env"), "life")); d != "ask" {
		t.Fatalf("credential guard decision %q", d)
	}
	os.Remove(filepath.Join(work, "deploy.env"))

	// 7. validation refuses to run until the commands are approved.
	if r := run(t, work, "", "verify"); r.code == 0 {
		t.Fatal("unapproved commands ran")
	}
	approveLifecycle(t, work)
	if r := run(t, work, "", "verify"); r.code != 0 || !strings.Contains(r.stdout, "checks passed") {
		t.Fatalf("verify: %+v", r)
	}

	// 8. ship refuses a credential, then carries the real change through.
	testutil.Write(t, work, "notes.md", "# Notes\n")
	testutil.Write(t, work, "leak.env", "key="+awsKey+"\n")
	if r := ship(t, work, "--message", "notes", "--files", "leak.env", "--dry-run"); r.code == 0 {
		t.Fatal("ship accepted a credential")
	}
	os.Remove(filepath.Join(work, "leak.env"))
	if r := ship(t, work, "--message", "docs: notes", "--files", "notes.md", "--dry-run"); r.code != 0 || !strings.Contains(r.stdout, "READY TO SHIP") {
		t.Fatalf("dry run: %+v", r)
	}
	orig := confirm
	confirm = func(string) (bool, error) { return true, nil }
	defer func() { confirm = orig }()
	wd, _ := os.Getwd()
	os.Chdir(work)
	stdout := os.Stdout
	devnull, _ := os.Open(os.DevNull)
	os.Stdout = devnull
	err = cmdShip(context.Background(), []string{"--message", "docs: notes", "--files", "notes.md"})
	os.Stdout = stdout
	os.Chdir(wd)
	if err != nil {
		t.Fatalf("ship: %v", err)
	}
	if testutil.Git(t, bare, "rev-parse", "feature") != testutil.Git(t, work, "rev-parse", "HEAD") {
		t.Fatal("the commit did not reach the remote")
	}

	// 9. the report counts what happened, and only that.
	r = run(t, work, "", "report", "current", "--json")
	var rep reportJSON
	if err := json.Unmarshal([]byte(r.stdout), &rep); err != nil {
		t.Fatalf("report: %v", err)
	}
	if rep.SubagentStarts != 2 || rep.ConfirmRequests < 1 || rep.CredentialWarnings < 1 || rep.DirectValidations < 1 {
		t.Fatalf("report does not reflect the session: %+v", rep)
	}

	// 10. removing the integration leaves the repository as it was.
	if err := integrate(t, work, true, "--remove"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	b, err := ioutil.ReadFile(filepath.Join(work, ".claude", "settings.local.json"))
	if err != nil || strings.Contains(string(b), "zeroturn") {
		t.Fatalf("settings after removal: %s", b)
	}
	if r := run(t, work, "", "doctor"); !strings.Contains(r.stdout, "not installed for this repository") {
		t.Fatalf("doctor after removal:\n%s", r.stdout)
	}
}

func approveLifecycle(t *testing.T, work string) {
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
