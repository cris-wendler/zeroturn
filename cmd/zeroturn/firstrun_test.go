package main

import (
	"context"
	"strings"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/testutil"
)

// freshRepo is a repository nobody has run ZeroTurn in, which is what
// every reader of this tool sees once.
func freshRepo(t *testing.T) string {
	t.Helper()
	testutil.Isolate(t)
	work, _ := testutil.Remote(t)
	testutil.Write(t, work, "go.mod", "module example.com/app\n\ngo 1.17\n")
	return work
}

// A warning is something the reader can do something about. One they
// cannot act on teaches them to skip the rest, which is how a real
// warning goes unread. So every warning has to name the command.
func TestEveryWarningNamesWhatToDo(t *testing.T) {
	work := freshRepo(t)

	// --compat adds checks of its own, and the rule holds for those too.
	// It did not when this test was first written: one of them said "Add
	// --live" without naming the command that carries the flag.
	stages := []struct {
		name string
		args []string
	}{
		{"before init", []string{"doctor", "--json"}},
		{"after init", []string{"doctor", "--json"}},
		{"after init, with compat", []string{"doctor", "--json", "--compat"}},
	}
	for _, stage := range stages {
		if strings.HasPrefix(stage.name, "after init") && !config.Exists(work) {
			if err := inProcess(t, work, true, func() error {
				return cmdInit(context.Background(), []string{"--yes"})
			}); err != nil {
				t.Fatal(err)
			}
		}
		for _, ck := range doctorChecks(t, work, stage.args...) {
			if ck.Status != checkWarn {
				continue
			}
			if !strings.Contains(ck.Detail, "zeroturn ") {
				t.Errorf("%s: the warning %q names no command to run: %s",
					stage.name, ck.Name, ck.Detail)
			}
		}
	}

	// Every stage above stands inside a repository, so a warning that
	// only appears outside one was never covered by this rule, and one of
	// them named nothing to do for exactly as long as that was true.
	// Running doctor is the first thing somebody does when a command did
	// not behave, and doing it from the wrong directory is common.
	outside := t.TempDir()
	found := 0
	for _, ck := range doctorChecks(t, outside, "doctor", "--json") {
		if ck.Status != checkWarn {
			continue
		}
		found++
		if !strings.Contains(ck.Detail, "zeroturn ") {
			t.Errorf("outside a repository: the warning %q names no command to run: %s",
				ck.Name, ck.Detail)
		}
	}
	if found == 0 {
		t.Error("doctor warned about nothing outside a repository, so this half checks nothing")
	}
}

// The first thing a reader sees has to point at the one thing to do. Four
// warnings, three of which cannot be acted on, is the state this test was
// written to stop coming back.
func TestAFreshMachineHasOneThingToDo(t *testing.T) {
	work := freshRepo(t)

	var warned []string
	for _, ck := range doctorChecks(t, work, "doctor", "--json") {
		switch ck.Status {
		case checkWarn:
			warned = append(warned, ck.Name)
		case checkFail:
			t.Errorf("%s failed on a machine with nothing wrong with it: %s", ck.Name, ck.Detail)
		}
	}
	if len(warned) != 1 || warned[0] != "configuration" {
		t.Errorf("a fresh machine warns about %v, want only the missing configuration", warned)
	}
}

// A harness check is never a warning. A harness invokes ZeroTurn, never
// the other way round, so one that is absent changes nothing about
// whether the integration works.
//
// This asserted that both harnesses were notes, which passed only on a
// machine with neither installed. It failed for anyone with Claude Code
// on their PATH, which is most people who would run this at all. The
// rule does not depend on what is installed, so neither does the test:
// found means ok and says the version, absent means a note that says why
// it does not matter, and neither is ever a warning.
func TestAHarnessCheckIsNeverAWarning(t *testing.T) {
	work := freshRepo(t)

	found := 0
	for _, ck := range doctorChecks(t, work, "doctor", "--json") {
		if ck.Name != "claude" && ck.Name != "copilot" {
			continue
		}
		found++
		switch ck.Status {
		case checkOK:
			// Installed. The detail is the version, or where it is.
			if strings.TrimSpace(ck.Detail) == "" {
				t.Errorf("%s is installed and the check says nothing about it", ck.Name)
			}
		case checkNote:
			if !strings.Contains(ck.Detail, "does not need it") {
				t.Errorf("%s does not say why it does not matter: %s", ck.Name, ck.Detail)
			}
		default:
			t.Errorf("%s is reported as %q, and a harness is never worth warning about", ck.Name, ck.Status)
		}
	}
	if found != 2 {
		t.Errorf("%d harness checks were found, want 2", found)
	}
}

// --yes says do not ask me. Printing the whole file afterwards answers a
// question nobody asked, and it was fifty of the seventy lines a first
// run produced.
func TestInitPrintsTheFileOnlyWhenItIsAsking(t *testing.T) {
	asked := freshRepo(t)
	withQuestion, err := capture(t, asked, true, func() error {
		return cmdInit(context.Background(), nil)
	})
	if err != nil {
		t.Fatalf("init: %v\n%s", err, withQuestion)
	}
	if !strings.Contains(withQuestion, "proposed "+config.FileName) {
		t.Errorf("init did not show the file it was asking about:\n%s", withQuestion)
	}

	told := freshRepo(t)
	withoutQuestion, err := capture(t, told, true, func() error {
		return cmdInit(context.Background(), []string{"--yes"})
	})
	if err != nil {
		t.Fatalf("init --yes: %v\n%s", err, withoutQuestion)
	}
	if strings.Contains(withoutQuestion, "proposed "+config.FileName) {
		t.Errorf("init --yes printed the file nobody asked to see:\n%s", withoutQuestion)
	}

	// The file is still written, and it still says so.
	if !config.Exists(told) {
		t.Error("init --yes wrote no configuration")
	}
	if !strings.Contains(withoutQuestion, "Wrote ") {
		t.Errorf("init --yes did not say it wrote anything:\n%s", withoutQuestion)
	}
	if len(strings.Split(strings.TrimSpace(withoutQuestion), "\n")) > 20 {
		t.Errorf("init --yes still prints a wall of text:\n%s", withoutQuestion)
	}
}
