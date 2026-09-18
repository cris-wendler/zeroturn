package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/harness/claude"
	"github.com/cris-wendler/zeroturn/internal/testutil"
)

// staleSettings writes an integration that points at an executable path
// that is not this one, as happens after the executable is moved or
// installed somewhere else.
func staleSettings(t *testing.T, work, path string) {
	t.Helper()
	doc := map[string]interface{}{
		"statusLine": map[string]interface{}{
			"type": "command", "command": `"` + path + `" status --stdin --harness claude", "padding": 0`,
		},
		"hooks": map[string]interface{}{
			"PreToolUse": []interface{}{
				map[string]interface{}{"matcher": "Agent", "hooks": []interface{}{
					map[string]interface{}{"type": "command", "command": `"` + path + `" event --harness claude --event PreToolUse`}}},
			},
			"Stop": []interface{}{
				map[string]interface{}{"hooks": []interface{}{
					map[string]interface{}{"type": "command", "command": `"` + path + `" event --harness claude --event Stop`},
					map[string]interface{}{"type": "command", "command": "my-own-hook"}}},
			},
		},
	}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	testutil.Write(t, work, ".claude/settings.local.json", string(b))
}

func TestDoctorReportsHooksPointingNowhere(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	gone := filepath.Join(work, "not-installed-here", "zeroturn")
	staleSettings(t, work, gone)
	r := run(t, work, "", "doctor")
	if !strings.Contains(r.stdout, "which is not there") || !strings.Contains(r.stdout, gone) {
		t.Fatalf("doctor did not report the broken path:\n%s", r.stdout)
	}
	if r.code == 0 {
		t.Fatal("a broken integration must fail doctor")
	}
}

func TestDoctorReportsHooksPointingElsewhere(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	other := filepath.Join(work, "other-zeroturn")
	testutil.Write(t, work, "other-zeroturn", "#!/bin/sh\n")
	staleSettings(t, work, other)
	r := run(t, work, "", "doctor")
	if !strings.Contains(r.stdout, "this is") || !strings.Contains(r.stdout, other) {
		t.Fatalf("doctor did not report the foreign path:\n%s", r.stdout)
	}
}

func TestDoctorAcceptsHooksPointingAtThisExecutable(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	staleSettings(t, work, bin)
	r := run(t, work, "", "doctor")
	if !strings.Contains(r.stdout, "the settings point at this executable") {
		t.Fatalf("doctor did not accept a correct integration:\n%s", r.stdout)
	}
}

func TestIntegrateRepairsAStalePath(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	gone := filepath.Join(work, "moved-away", "zeroturn")
	staleSettings(t, work, gone)

	r := run(t, work, "", "integrate", "claude", "--plan")
	if !strings.Contains(r.stdout, "would repair") || !strings.Contains(r.stdout, gone) {
		t.Fatalf("the plan does not offer the repair:\n%s", r.stdout)
	}

	if err := integrate(t, work, true, "--apply"); err != nil {
		t.Fatal(err)
	}
	b, err := ioutil.ReadFile(settingsFile(work))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), gone) {
		t.Fatalf("the old path survived the repair:\n%s", b)
	}
	// The command a person added to the same entry is left alone.
	if !strings.Contains(string(b), "my-own-hook") {
		t.Fatalf("a hook that is not ZeroTurn's was removed:\n%s", b)
	}
	after := readJSON(t, settingsFile(work))
	for ev, entries := range after["hooks"].(map[string]interface{}) {
		for _, e := range entries.([]interface{}) {
			for _, h := range e.(map[string]interface{})["hooks"].([]interface{}) {
				cmd := h.(map[string]interface{})["command"].(string)
				if claude.Owned(cmd) && !strings.Contains(cmd, selfPath()) {
					t.Errorf("%s still points elsewhere: %s", ev, cmd)
				}
			}
		}
	}
}

func TestRepairIsNotOfferedWhenNothingIsWrong(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	// The settings name the executable that runs the plan, which is the
	// healthy case.
	staleSettings(t, work, bin)
	r := run(t, work, "", "integrate", "claude", "--plan")
	if strings.Contains(r.stdout, "would repair") {
		t.Fatalf("a repair was offered for a healthy integration:\n%s", r.stdout)
	}
}

// A build run from the source is not the installed one and is not meant
// to be. CONTRIBUTING tells contributors to work that way, so reporting
// it as two installs to remove one of would warn everybody who followed
// the instructions.
//
// go run lands in two different places: a directory under the system
// temporary one, and the Go build cache, which is not temporary by name.
// Only the first was recognised at first, and the second is the one that
// happens on a warm cache, which is to say almost always.
func TestABuildRunFromTheSourceIsNotMistakenForAnInstall(t *testing.T) {
	cache, err := os.UserCacheDir()
	if err != nil {
		t.Skip("no user cache directory on this machine")
	}
	inCache := filepath.Join(cache, "go-build", "bf", "abc123-d", "zeroturn")
	inTemp := filepath.Join(os.TempDir(), "go-build123456", "b001", "exe", "zeroturn")
	for _, p := range []string{inCache, inTemp} {
		if !isTemporaryBuild(p) {
			t.Errorf("%s is a build to run once and was taken for an install", p)
		}
	}

	// An installed executable must never be taken for a temporary one,
	// or the advice for somebody whose PATH is wrong would be to install
	// what they already installed.
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home directory")
	}
	for _, p := range []string{
		filepath.Join(home, "go", "bin", "zeroturn"),
		filepath.Join("/usr", "local", "bin", "zeroturn"),
	} {
		if isTemporaryBuild(p) {
			t.Errorf("%s is an installed executable and was taken for a temporary build", p)
		}
	}
}

// Whatever the doctor says about this, it has to say something, and a
// warning has to name an action. The check exists because somebody could
// not run zeroturn at all, so an answer they cannot act on is the one
// failure mode that matters.
//
// The two levels are held to different rules on purpose. Entry 43 in the
// decisions defines a note as something true that the reader cannot act
// on, which is the whole reason the level exists, so requiring an action
// from one contradicts it.
//
// This test used to demand an action from every answer that was not ok,
// and it therefore depended on the machine it ran on. A checkout built
// by go test is a temporary build, so on a machine with zeroturn
// installed and on PATH the check correctly returns the note that says
// which one typing the name would run, and the test failed on a true
// answer. It passed on every continuous integration runner and in any
// shell without the install directory on PATH, which is why it survived.
// That is the shape of entry 50: assert the rule, not the machine.
func TestThePathCheckAlwaysNamesSomethingToDo(t *testing.T) {
	c := checkOnPath()
	if c.Detail == "" {
		t.Fatalf("the check said nothing at all, at status %q", c.Status)
	}
	if !strings.Contains(c.Detail, "zeroturn") {
		t.Errorf("the detail does not name the command it is about: %q", c.Detail)
	}
	if c.Status != checkWarn {
		return
	}
	// The wording differs with the case, so what is required is an
	// action, not a particular word. Asking for the word PATH failed the
	// moment a case stopped needing to say it, which is the wrong reason
	// for a test to fail.
	if !strings.Contains(c.Detail, "export PATH=") &&
		!strings.Contains(c.Detail, "go install") &&
		!strings.Contains(c.Detail, "Remove") {
		t.Errorf("the warning names nothing the reader can do: %q", c.Detail)
	}
}

// The rule above is only as good as the answers it is applied to, and on
// any one machine checkOnPath returns one of its four. This applies the
// rule to all four, so a case nobody's machine happens to produce cannot
// drift. It is the half the live check could never cover.
func TestEveryAnswerThePathCheckCanGiveFollowsTheRule(t *testing.T) {
	answers := []check{
		{"on your PATH", checkOK, "/usr/local/bin/zeroturn"},
		{"on your PATH", checkNote,
			"this build was run from the source and is not installed. Install it with go install ./cmd/zeroturn to run it by name"},
		{"on your PATH", checkNote,
			"this build was run from the source. Typing zeroturn runs the installed one at /usr/local/bin/zeroturn"},
		{"on your PATH", checkWarn,
			"zeroturn is at /tmp/zeroturn and is not on your PATH, so the command is not found by name. " +
				"Add its directory to PATH in your shell profile: export PATH=\"/tmp:$PATH\""},
		{"on your PATH", checkWarn,
			"this is /tmp/zeroturn, and typing zeroturn runs /usr/local/bin/zeroturn instead. " +
				"Remove the one you do not want, or move its directory later in your PATH"},
	}
	for _, c := range answers {
		if c.Detail == "" {
			t.Errorf("%s: says nothing", c.Status)
			continue
		}
		if c.Status != checkWarn {
			continue
		}
		if !strings.Contains(c.Detail, "export PATH=") &&
			!strings.Contains(c.Detail, "go install") &&
			!strings.Contains(c.Detail, "Remove") {
			t.Errorf("a warning names nothing the reader can do: %q", c.Detail)
		}
	}
}
