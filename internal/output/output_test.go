package output

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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

// Colour is decided once, here, and every command asks this. The rules
// are ordered: the environment's refusal wins over everything, a harness
// that renders the output itself wins over the destination, and a
// destination that cannot show an escape sequence gets none.
func TestWhatDecidesColour(t *testing.T) {
	plain, err := os.Create(filepath.Join(t.TempDir(), "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer plain.Close()
	terminal, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer terminal.Close()

	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm")

	// A destination that is not a character device gets none, and one
	// that is gets colour.
	if NewColor(plain, "").Escapes() {
		t.Error("a redirected stream was given colour")
	}
	if !NewColor(terminal, "").Escapes() {
		t.Error("a character device was not given colour")
	}
	// A harness renders the output itself, so colour is kept even
	// though the destination is a pipe or a file.
	if !NewColor(plain, "claude").Escapes() {
		t.Error("a harness rendering the output was given no colour")
	}

	// Either refusal in the environment wins over both of those.
	t.Setenv("NO_COLOR", "1")
	if NewColor(terminal, "claude").Escapes() {
		t.Error("NO_COLOR was set and colour was used anyway")
	}
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "dumb")
	if NewColor(terminal, "claude").Escapes() {
		t.Error("TERM=dumb was set and colour was used anyway")
	}

	// A destination that cannot be asked about is treated as one that
	// cannot show colour.
	t.Setenv("TERM", "xterm")
	closed, err := os.Create(filepath.Join(t.TempDir(), "closed"))
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	if NewColor(closed, "").Escapes() {
		t.Error("a destination that could not be read was given colour")
	}
}
