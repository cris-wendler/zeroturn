package main

import (
	"strings"
	"testing"
)

// doctor asks each harness for its version and printed back whatever the
// first line of standard output happened to be, as the version, with an
// ok beside it. On a real machine that produced:
//
//	OK    copilot    Install GitHub Copilot CLI? ['y/N']
//
// which is an installer prompt written to standard output by a program
// that was asked for its version. Reported as a version it is wrong
// twice: it says a harness is installed when what is installed is
// something offering to install it, and it prints a question to somebody
// who cannot answer it, in a list of facts.
//
// A version names a number. That is the whole rule, and it is enough:
// every real answer here carries one and the prompt carries none.
func TestAVersionLineHasToNameANumber(t *testing.T) {
	versions := []string{
		"git version 2.37.2",
		"2.1.270 (Claude Code)",
		"1.0.83",
		"v0.3.0",
		"GitHub CLI 2.40.1 (2024-01-08)",
	}
	for _, v := range versions {
		if !looksLikeVersion(v) {
			t.Errorf("%q is a version and was refused", v)
		}
	}

	notVersions := []string{
		"Install GitHub Copilot CLI? ['y/N']",
		"command not found",
		"Please log in to continue",
		"",
		"   ",
	}
	for _, v := range notVersions {
		if looksLikeVersion(v) {
			t.Errorf("%q is not a version and was accepted", v)
		}
	}
}

// A program can write a paragraph to standard output. One line of a
// table is not the place for it, and a long answer is a sign that what
// came back was not a version anyway.
func TestAVersionLineIsNotAParagraph(t *testing.T) {
	long := "1.0.0 " + strings.Repeat("and more text ", 40)
	if looksLikeVersion(long) {
		t.Error("a paragraph beginning with a number was accepted as a version")
	}
}

// Whatever it decides, it must not put a question to the reader. doctor
// prints a list of findings, and a question in that list reads as
// something they failed to answer.
func TestTheHarnessCheckNeverPrintsAQuestion(t *testing.T) {
	for _, c := range []check{
		checkExecutable("git", "git", "--version"),
		checkExecutable("claude", "claude", "--version"),
		checkExecutable("copilot", "copilot", "--version"),
	} {
		if strings.Contains(c.Detail, "?") {
			t.Errorf("the %s check asks the reader a question: %q", c.Name, c.Detail)
		}
		if strings.Contains(strings.ToLower(c.Detail), "y/n") {
			t.Errorf("the %s check prints a prompt: %q", c.Name, c.Detail)
		}
	}
}
