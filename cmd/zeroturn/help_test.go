package main

import (
	"strings"
	"testing"
)

// The top level usage tells a reader to run zeroturn <command> --help.
// Every command that says so has to answer it, print its own usage
// rather than the flag package's listing, and succeed. Exit 2 here was
// both wrong and dangerous: it is the code a harness reads as a
// blocking error.
func TestHelpIsAnsweredByEveryCommand(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	commands := [][]string{
		{"status"}, {"verify"}, {"doctor"}, {"init"}, {"capabilities"}, {"report"},
		{"ship"}, {"integrate"}, {"event"},
		{"policy"}, {"policy", "show"}, {"policy", "check"},
		{"policy", "set"}, {"policy", "reset"}, {"policy", "tune"},
	}
	for _, cmd := range commands {
		name := strings.Join(cmd, " ")
		t.Run(name, func(t *testing.T) {
			r := run(t, work, "", append(cmd, "--help")...)
			if r.code != 0 {
				t.Fatalf("zeroturn %s --help exited %d\n%s%s", name, r.code, r.stdout, r.stderr)
			}
			out := r.stdout + r.stderr
			if !strings.Contains(out, "zeroturn "+cmd[0]) {
				t.Errorf("help for %s does not name the command:\n%s", name, out)
			}
			// The flag package's own listing starts this way, and it is
			// not the format the rest of the output uses.
			if strings.Contains(out, "Usage of ") {
				t.Errorf("help for %s printed the default flag listing:\n%s", name, out)
			}
			for _, wrong := range []string{"could not read the flags", "help requested", "flag: help"} {
				if strings.Contains(out, wrong) {
					t.Errorf("help for %s reported an error: %q", name, wrong)
				}
			}
		})
	}
}

// -h is the short form and has to behave the same way.
func TestShortHelpFlagIsAnsweredToo(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	for _, cmd := range []string{"status", "verify", "doctor", "capabilities"} {
		if r := run(t, work, "", cmd, "-h"); r.code != 0 {
			t.Errorf("zeroturn %s -h exited %d", cmd, r.code)
		}
	}
}

// A flag that does not exist is still an error, with the project's own
// wording and the code that means invalid usage.
func TestAnUnknownFlagIsStillAnError(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	r := run(t, work, "", "status", "--nonsense")
	if r.code == 0 {
		t.Fatal("an unknown flag succeeded")
	}
	if !strings.Contains(r.stderr, "could not read the flags") {
		t.Errorf("stderr does not explain the failure:\n%s", r.stderr)
	}
	if !strings.Contains(r.stderr, "zeroturn status --help") {
		t.Errorf("stderr does not say where to look:\n%s", r.stderr)
	}
}
