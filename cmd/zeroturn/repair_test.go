package main

import (
	"encoding/json"
	"io/ioutil"
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
