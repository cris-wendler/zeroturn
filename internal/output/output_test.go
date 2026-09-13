package output

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"testing"
)

// The exit codes are a published contract. Every assertion elsewhere in
// this repository uses the constant, so renumbering one would leave the
// whole suite green while breaking every adapter and every script that
// reads them. These are the numbers, written out.
//
// Changing one of these is a major contract version, not a test to edit.
func TestExitCodesAreTheirPublishedNumbers(t *testing.T) {
	for _, c := range []struct {
		name string
		got  int
		want int
	}{
		{"ExitOK", ExitOK, 0},
		{"ExitPolicyFailure", ExitPolicyFailure, 1},
		{"ExitInvalidUsage", ExitInvalidUsage, 2},
		{"ExitUnsafeGit", ExitUnsafeGit, 3},
		{"ExitMissingExec", ExitMissingExec, 4},
		{"ExitInternal", ExitInternal, 5},
		{"ExitDeclined", ExitDeclined, 6},
		{"ExitContractVersion", ExitContractVersion, 7},
		{"ExitNoIntegration", ExitNoIntegration, 8},
	} {
		if c.got != c.want {
			t.Errorf("%s is %d, and the published contract says %d", c.name, c.got, c.want)
		}
	}
}

// Every failure message states what stopped, why, and the smallest safe
// next action. A message missing one of those leaves a reader stuck.
func TestErrorStatesAllThreeParts(t *testing.T) {
	e := Errorf(ExitUnsafeGit, "zeroturn ship stopped", "the branch is protected", "switch to a branch you can push")
	var buf bytes.Buffer
	e.Print(&buf)
	out := buf.String()
	for _, want := range []string{"zeroturn ship stopped", "the branch is protected", "switch to a branch you can push"} {
		if !strings.Contains(out, want) {
			t.Errorf("printed message is missing %q:\n%s", want, out)
		}
	}
	if e.Code != ExitUnsafeGit {
		t.Errorf("code %d", e.Code)
	}
	if !strings.Contains(e.Error(), "the branch is protected") {
		t.Errorf("Error() does not say why: %q", e.Error())
	}
}

// The entry point decides the exit code by inspecting the error, so a
// wrapped error has to stay recognisable. It did not before: a bare type
// assertion turned a wrapped code into the internal failure code.
func TestAWrappedErrorKeepsItsCode(t *testing.T) {
	wrapped := fmt.Errorf("while shipping: %w", Errorf(ExitDeclined, "stopped", "declined", "answer yes"))
	var ze *Error
	if !errors.As(wrapped, &ze) {
		t.Fatal("a wrapped ZeroTurn error is no longer recognisable")
	}
	if ze.Code != ExitDeclined {
		t.Fatalf("code %d, want %d", ze.Code, ExitDeclined)
	}
}
