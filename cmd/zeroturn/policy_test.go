package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/capabilities"
	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/output"
	"github.com/cris-wendler/zeroturn/internal/state"
	"github.com/cris-wendler/zeroturn/internal/trust"
)

func policyIn(t *testing.T, work string, answer bool, args ...string) error {
	t.Helper()
	return inProcess(t, work, answer, func() error { return cmdPolicy(context.Background(), args) })
}

func withHarness(t *testing.T, version string, err error) {
	orig := harnessVersion
	harnessVersion = func() (string, error) { return version, err }
	t.Cleanup(func() { harnessVersion = orig })
}

func strictApproved(t *testing.T, work string) bool {
	st, err := state.Open()
	if err != nil {
		t.Fatal(err)
	}
	return trust.StrictApproved(st, work)
}

func TestPolicySetValidates(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	for _, args := range [][]string{
		{"set", "guard.context.warn", "0"},
		{"set", "guard.context.critical", "101"},
		{"set", "guard.context.warn", "85"},
		{"set", "guard.context.warn", "abc"},
		{"set", "guard.mode", "block"},
		{"set", "guard.unknown", "1"},
		{"set", "guard.mode"},
	} {
		r := run(t, work, "", append([]string{"policy"}, args...)...)
		if r.code != output.ExitInvalidUsage || !strings.Contains(r.stderr, "next:") {
			t.Errorf("%v: %+v", args, r)
		}
	}
	c, _ := config.Load(work)
	if c.Guard != config.Default().Guard {
		t.Fatal("a rejected value was written")
	}
}

func TestPolicySetConfirmNeedsNoApproval(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	r := run(t, work, "", "policy", "set", "guard.mode", "confirm")
	if r.code != 0 {
		t.Fatalf("%+v", r)
	}
	if c, _ := config.Load(work); c.Guard.Mode != config.ModeConfirm {
		t.Fatal("mode not written")
	}
}

func TestStrictRequiresConfirmation(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	withHarness(t, capabilities.ClaudeTestedVersions[0], nil)
	// The built binary uses the real harness lookup, so on a machine
	// without the harness it stops earlier, with exit 8, which is also safe.
	if r := run(t, work, "", "policy", "set", "guard.mode", "strict"); r.code != output.ExitDeclined && r.code != output.ExitNoIntegration {
		t.Fatalf("without a terminal: %+v", r)
	}
	err := policyIn(t, work, false, "set", "guard.mode", "strict")
	if ze, ok := err.(*output.Error); !ok || ze.Code != output.ExitDeclined {
		t.Fatalf("declined: %v", err)
	}
	if c, _ := config.Load(work); c.Guard.Mode != config.ModeObserve || strictApproved(t, work) {
		t.Fatal("strict was enabled without confirmation")
	}
	if err := policyIn(t, work, true, "set", "guard.mode", "strict"); err != nil {
		t.Fatal(err)
	}
	if c, _ := config.Load(work); c.Guard.Mode != config.ModeStrict || !strictApproved(t, work) {
		t.Fatal("confirmed strict was not enabled")
	}
	if err := policyIn(t, work, true, "set", "guard.mode", "confirm"); err != nil {
		t.Fatal(err)
	}
	if strictApproved(t, work) {
		t.Fatal("leaving strict did not withdraw the approval")
	}
}

func TestStrictRefusedWithoutHarness(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	withHarness(t, "", errors.New("not found"))
	err := policyIn(t, work, true, "set", "guard.mode", "strict")
	if ze, ok := err.(*output.Error); !ok || ze.Code != output.ExitNoIntegration {
		t.Fatalf("err %v", err)
	}
}

func TestPolicyResetWithdrawsStrict(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	withHarness(t, capabilities.ClaudeTestedVersions[0], nil)
	policyIn(t, work, true, "set", "guard.mode", "strict")
	if err := policyIn(t, work, true, "reset"); err != nil {
		t.Fatal(err)
	}
	if c, _ := config.Load(work); c.Guard.Mode != config.ModeObserve || strictApproved(t, work) {
		t.Fatal("reset left strict in place")
	}
}

func TestPolicyShowNotesUnapprovedStrict(t *testing.T) {
	work, _ := repoWithConfig(t, func(c *config.Config) { c.Guard.Mode = config.ModeStrict })
	r := run(t, work, "", "policy", "show")
	if r.code != 0 || !strings.Contains(r.stdout, "has not been approved on this machine") {
		t.Fatalf("%+v", r)
	}
}
