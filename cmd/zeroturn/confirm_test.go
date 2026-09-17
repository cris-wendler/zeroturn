package main

import (
	"os"
	"strings"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/output"
)

// withStdin points os.Stdin at the named file for one test. The real
// confirm reads os.Stdin directly, and every other test in this package
// replaces the confirm variable instead, so nothing here had ever run
// the function itself against a real file.
func withStdin(t *testing.T, name string) {
	t.Helper()
	f, err := os.Open(name)
	if err != nil {
		t.Skipf("%s cannot be opened on this machine: %v", name, err)
	}
	orig := os.Stdin
	os.Stdin = f
	t.Cleanup(func() {
		os.Stdin = orig
		f.Close()
	})
}

// A confirmation nobody answered is not a refusal. /dev/null is the
// stdin of a scheduled job, a container started without an interactive
// flag, and a continuous integration step. It is a character device, so
// the terminal check accepts it, and reading it ends at once. Calling
// that a decline states a decision nobody made, and the advice to run
// the command again cannot ever work, because the next run reads the
// same empty input.
func TestInputThatAnswersNothingIsNotADecline(t *testing.T) {
	withStdin(t, os.DevNull)

	ok, err := confirm("Do the thing?")
	if ok {
		t.Fatal("an unanswered confirmation was taken for a yes")
	}
	if err == nil {
		t.Fatal("an unanswered confirmation was reported as a refusal, with no error")
	}
	oe, isOutput := err.(*output.Error)
	if !isOutput {
		t.Fatalf("got %v, want a ZeroTurn error", err)
	}
	if strings.Contains(strings.ToLower(oe.Because), "declin") ||
		strings.Contains(strings.ToLower(oe.Because), "refus") {
		t.Errorf("the reason says somebody refused, and nobody answered: %q", oe.Because)
	}
	if oe.Next == "" {
		t.Error("the refusal names nothing to do instead")
	}
}

// The two shapes of input that cannot answer are a pipe and a character
// device that is not a terminal. They took different paths and gave
// different answers, and only one of them was true.
func TestEveryInputThatCannotAnswerGivesTheSameReason(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	w.Close()
	orig := os.Stdin
	os.Stdin = r
	piped, perr := confirm("Do the thing?")
	os.Stdin = orig
	r.Close()

	if piped {
		t.Fatal("a closed pipe was taken for a yes")
	}
	if perr == nil {
		t.Fatal("a closed pipe was reported as a refusal")
	}

	withStdin(t, os.DevNull)
	null, nerr := confirm("Do the thing?")
	if null {
		t.Fatal("an empty device was taken for a yes")
	}
	if nerr == nil {
		t.Fatal("an empty device was reported as a refusal")
	}

	po, ok1 := perr.(*output.Error)
	no, ok2 := nerr.(*output.Error)
	if !ok1 || !ok2 {
		t.Fatalf("want ZeroTurn errors, got %v and %v", perr, nerr)
	}
	if po.Because != no.Because {
		t.Errorf("a pipe says %q and a device says %q, and neither could answer", po.Because, no.Because)
	}
	if po.Code != no.Code {
		t.Errorf("a pipe exits %d and a device exits %d", po.Code, no.Code)
	}
}

// Whatever it says, the reader has to be told what to do, and the
// advice has to be true for the command they ran. Naming a flag that
// only one command has sends everybody else looking for it.
func TestTheRefusalNamesSomethingEveryCommandCanDo(t *testing.T) {
	withStdin(t, os.DevNull)
	_, err := confirm("Do the thing?")
	oe, ok := err.(*output.Error)
	if !ok {
		t.Fatalf("got %v, want a ZeroTurn error", err)
	}
	if !strings.Contains(oe.Next, "terminal") {
		t.Errorf("the next action does not name the one thing that always works: %q", oe.Next)
	}
	// --dry-run belongs to ship alone. verify, init, uninstall, report
	// purge, integrate and the two policy confirmations do not have it.
	if strings.Contains(oe.Next, "--dry-run") {
		t.Errorf("the next action names a flag most of the commands that ask do not have: %q", oe.Next)
	}
}
