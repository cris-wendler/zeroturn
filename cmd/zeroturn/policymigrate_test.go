package main

import (
	"context"
	"errors"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/output"
	"github.com/cris-wendler/zeroturn/internal/testutil"
)

// oldFile writes the kind of file migration exists for: no version, a
// value worth keeping, several settings that did not exist when it was
// written, and one that a later version removed.
func oldFile(t *testing.T) string {
	t.Helper()
	testutil.Isolate(t)
	work, _ := testutil.Remote(t)
	body := `{
  "guard": {
    "mode": "confirm",
    "context": {"warn": 55, "confirm": 65, "critical": 75},
    "limits": {"tenDayWarn": 60}
  },
  "git": {"remote": "origin", "protectedBranches": ["main"]}
}`
	if err := ioutil.WriteFile(config.Path(work), []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return work
}

// A file like this loads without anyone running a command. Migration in
// memory is what keeps an older file working; the command only writes
// down what is already being read.
func TestAnOlderFileLoadsWithoutBeingMigrated(t *testing.T) {
	work := oldFile(t)

	c, err := config.Load(work)
	if err != nil {
		t.Fatalf("an older file does not load: %v", err)
	}
	if c.Guard.Mode != config.ModeConfirm {
		t.Errorf("mode is %q, want the value the file carried", c.Guard.Mode)
	}
	if c.Guard.Context.Warn != 55 {
		t.Errorf("context warn is %d, want the value the file carried", c.Guard.Context.Warn)
	}
	if c.Guard.Credentials.Mode != config.CredentialAsk {
		t.Errorf("credential mode is %q, want the default for a setting the file predates", c.Guard.Credentials.Mode)
	}
}

func TestPolicyMigratePlanChangesNothing(t *testing.T) {
	work := oldFile(t)
	before, err := ioutil.ReadFile(config.Path(work))
	if err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, work, true, func() error {
		return policyMigrate(context.Background(), []string{"--plan"})
	})
	if err != nil {
		t.Fatalf("policy migrate --plan: %v\n%s", err, out)
	}

	after, err := ioutil.ReadFile(config.Path(work))
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Errorf("the plan rewrote the file:\n%s", after)
	}
	for _, want := range []string{
		"version         0 to 1",
		"guard.credentials.mode",
		"guard.limits.tenDayWarn",
		"Nothing has changed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("the plan does not mention %q:\n%s", want, out)
		}
	}
}

func TestPolicyMigrateWritesWhatItPromised(t *testing.T) {
	work := oldFile(t)

	if _, err := capture(t, work, true, func() error {
		return policyMigrate(context.Background(), nil)
	}); err != nil {
		t.Fatalf("policy migrate: %v", err)
	}

	raw, err := ioutil.ReadFile(config.Path(work))
	if err != nil {
		t.Fatal(err)
	}
	c, report, err := config.Migrate(raw)
	if err != nil {
		t.Fatalf("the migrated file does not load: %v", err)
	}
	if report.Changed() {
		t.Errorf("the migrated file still needs migrating: %+v", report)
	}
	// What the file carried has to survive being rewritten.
	if c.Guard.Mode != config.ModeConfirm || c.Guard.Context.Warn != 55 {
		t.Errorf("migration lost a value: mode %q, context warn %d", c.Guard.Mode, c.Guard.Context.Warn)
	}
	// The setting this build does not have is gone from the file, having
	// been named before it went.
	if strings.Contains(string(raw), "tenDayWarn") {
		t.Errorf("the removed setting is still in the file:\n%s", raw)
	}
}

// Running it again has to be quiet, or it would look as though something
// were still wrong.
func TestPolicyMigrateOnAFileThatNeedsNothing(t *testing.T) {
	testutil.Isolate(t)
	work, _ := testutil.Remote(t)
	if err := config.Save(work, config.Default()); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, work, true, func() error {
		return policyMigrate(context.Background(), nil)
	})
	if err != nil {
		t.Fatalf("policy migrate: %v\n%s", err, out)
	}
	if !strings.Contains(out, "already in the format") {
		t.Errorf("it did not say there was nothing to do:\n%s", out)
	}
}

// A file from a newer build is the case where the old advice was actively
// harmful: it said to run init, which overwrites the policy.
func TestAFileFromANewerBuildIsNotAnsweredByOverwritingIt(t *testing.T) {
	testutil.Isolate(t)
	work, _ := testutil.Remote(t)
	if err := ioutil.WriteFile(config.Path(work), []byte(`{"version": 99}`), 0600); err != nil {
		t.Fatal(err)
	}

	out, err := capture(t, work, true, func() error {
		return policyMigrate(context.Background(), nil)
	})
	if err == nil {
		t.Fatal("a file from a newer build was migrated")
	}
	// The smallest safe next action is the part that was wrong, and it is
	// carried on the error rather than in its message.
	assertNextAction(t, err, "upgrade ZeroTurn")
	// The command returns before printing anything on this path, so
	// checking stdout for the old advice checked an empty string. What a
	// person reads is the error itself.
	if out != "" {
		t.Errorf("this path is expected to print nothing, and printed:\n%s", out)
	}
	if strings.Contains(err.Error(), "init") {
		t.Errorf("the message still suggests overwriting the file: %v", err)
	}

	// Every other command has to say the same thing rather than call it a
	// broken file.
	_, lerr := loadConfig(work)
	if lerr == nil {
		t.Fatal("loading a newer file succeeded")
	}
	assertNextAction(t, lerr, "upgrade ZeroTurn")
}

// assertNextAction checks the advice a failure gives, which is the half of
// an error message a person acts on.
func assertNextAction(t *testing.T, err error, want string) {
	t.Helper()
	var oe *output.Error
	if !errors.As(err, &oe) {
		t.Fatalf("got %v, want a ZeroTurn error carrying a next action", err)
	}
	if !strings.Contains(oe.Next, want) {
		t.Errorf("the next action is %q, want it to say %q", oe.Next, want)
	}
	if strings.Contains(oe.Next, "init") {
		t.Errorf("the next action still suggests overwriting the file: %q", oe.Next)
	}
}
