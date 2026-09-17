package main

import (
	"strings"
	"testing"
)

// Standing in the wrong directory is the same mistake whichever command
// you typed, so every command that needs a repository has to answer it
// the same way. integrate did not: it collapsed the repository lookup
// and the home directory lookup into one failure, and the advice it
// printed belonged to the other one. A reader was told to check that
// their home directory was readable when the only thing wrong was where
// they were standing, and the exit code said the tool had broken
// internally rather than that the command had been used wrongly.
func TestEveryCommandThatNeedsARepositorySaysTheSameThingWithoutOne(t *testing.T) {
	outside := t.TempDir()

	// Each of these opens the repository before it does anything else.
	// report is deliberately absent: without one it reports the machine,
	// which is a described behaviour rather than a refusal.
	commands := [][]string{
		{"verify"},
		{"status"},
		{"integrate", "claude", "--plan"},
	}

	type answer struct {
		code int
		next string
	}
	got := map[string]answer{}
	for _, args := range commands {
		r := run(t, outside, "", args...)
		name := strings.Join(args, " ")
		out := r.stdout + r.stderr
		if !strings.Contains(out, "not inside a Git repository") {
			t.Errorf("%s did not say the directory is not a repository:\n%s", name, out)
			continue
		}
		var next string
		for _, line := range strings.Split(out, "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "next:") {
				next = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line), "next:"))
			}
		}
		got[name] = answer{r.code, next}
	}
	if len(got) == 0 {
		t.Fatal("no command answered, so this test checks nothing")
	}

	// verify is the reference, because it is the plainest case.
	want, ok := got["verify"]
	if !ok {
		t.Fatal("verify did not report a directory that is not a repository")
	}
	if !strings.Contains(want.next, "git init") {
		t.Fatalf("the reference advice does not name what to do: %q", want.next)
	}
	for name, a := range got {
		if a.code != want.code {
			t.Errorf("%s exits %d outside a repository and verify exits %d", name, a.code, want.code)
		}
		if a.next != want.next {
			t.Errorf("%s advises %q and verify advises %q, for the same mistake", name, a.next, want.next)
		}
	}
}

// A message that wraps another one states what stopped twice, and the
// two halves were written for different readers.
func TestTheRefusalIsNotWrappedInASecondRefusal(t *testing.T) {
	outside := t.TempDir()
	r := run(t, outside, "", "integrate", "claude", "--plan")
	out := r.stdout + r.stderr
	if strings.Contains(out, "changed nothing: zeroturn") ||
		strings.Contains(out, "zeroturn stopped before doing anything: this directory") &&
			strings.Contains(out, "integrate changed nothing") {
		t.Errorf("the reason carries a second refusal inside it:\n%s", out)
	}
}
