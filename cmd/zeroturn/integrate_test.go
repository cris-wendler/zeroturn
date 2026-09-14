package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/harness/claude"
	"github.com/cris-wendler/zeroturn/internal/output"
	"github.com/cris-wendler/zeroturn/internal/testutil"
)

// userSettings holds entries written by the user or another tool. Every
// one of them must survive install, reinstall, and removal unchanged.
const userSettings = `{
  "permissions": {"allow": ["Bash(npm test)"], "deny": ["Read(./.env)"]},
  "env": {"FOO": "bar"},
  "hooks": {
    "PreToolUse": [
      {"matcher": "Bash", "hooks": [{"type": "command", "command": "~/bin/audit-bash", "timeout": 5}]}
    ],
    "Stop": [
      {"hooks": [{"type": "command", "command": "say done"}], "note": "unknown field kept"}
    ]
  },
  "model": "sonnet"
}
`

func settingsFile(work string) string {
	return filepath.Join(work, ".claude", "settings.local.json")
}

func readJSON(t *testing.T, p string) map[string]interface{} {
	t.Helper()
	b, err := ioutil.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("%s is not valid JSON after the change: %v", p, err)
	}
	return m
}

func integrate(t *testing.T, work string, answer bool, args ...string) error {
	t.Helper()
	return inProcess(t, work, answer, func() error {
		return cmdIntegrate(context.Background(), append([]string{"claude"}, args...))
	})
}

func zeroturnHooks(m map[string]interface{}) map[string]int {
	out := map[string]int{}
	hooks, _ := m["hooks"].(map[string]interface{})
	for ev, list := range hooks {
		for _, e := range list.([]interface{}) {
			for _, h := range e.(map[string]interface{})["hooks"].([]interface{}) {
				if claude.Owned(h.(map[string]interface{})["command"].(string)) {
					out[ev]++
				}
			}
		}
	}
	return out
}

func TestIntegratePlanChangesNothing(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	r := run(t, work, "", "integrate", "claude", "--plan")
	if r.code != 0 || !strings.Contains(r.stdout, "matcher Agent, exact") || !strings.Contains(r.stdout, "Nothing was changed") {
		t.Fatalf("%+v", r)
	}
	if _, err := os.Stat(settingsFile(work)); !os.IsNotExist(err) {
		t.Fatal("plan created the settings file")
	}
	if !strings.Contains(r.stdout, "not ignored by Git") {
		t.Fatalf("plan did not warn that the file is not ignored: %s", r.stdout)
	}
}

func TestIntegrateApplyWithoutTerminalRefuses(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	r := run(t, work, "", "integrate", "claude", "--apply")
	if r.code != output.ExitDeclined {
		t.Fatalf("%+v", r)
	}
	if _, err := os.Stat(settingsFile(work)); !os.IsNotExist(err) {
		t.Fatal("apply wrote without confirmation")
	}
}

func TestIntegrateInstallReinstallRemove(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	testutil.Write(t, work, ".claude/settings.local.json", userSettings)
	original := readJSON(t, settingsFile(work))

	if err := integrate(t, work, true, "--apply"); err != nil {
		t.Fatal(err)
	}
	after := readJSON(t, settingsFile(work))
	got := zeroturnHooks(after)
	want := map[string]int{}
	for _, h := range claude.Hooks {
		want[h.Event]++
	}
	for ev, n := range want {
		if got[ev] != n {
			t.Errorf("%s: %d ZeroTurn hooks, want %d", ev, got[ev], n)
		}
	}
	matchers := map[string]bool{}
	for _, e := range after["hooks"].(map[string]interface{})["PreToolUse"].([]interface{}) {
		m := e.(map[string]interface{})
		if claude.Owned(m["hooks"].([]interface{})[0].(map[string]interface{})["command"].(string)) {
			matcher, _ := m["matcher"].(string)
			matchers[matcher] = true
		}
	}
	// Exact tool names only. A pattern would fire for tools ZeroTurn does
	// not gate.
	if !matchers["Agent"] || !matchers["Read"] || len(matchers) != 2 {
		t.Errorf("PreToolUse matchers are %v, want exactly Agent and Read", matchers)
	}
	for _, k := range []string{"permissions", "env", "model"} {
		if !reflect.DeepEqual(after[k], original[k]) {
			t.Errorf("%s changed", k)
		}
	}
	if _, ok := after["statusLine"]; !ok {
		t.Error("status line not installed")
	}

	backups, _ := filepath.Glob(filepath.Join(os.Getenv("ZEROTURN_STATE_DIR"), "backups", "*.json"))
	if len(backups) != 1 {
		t.Fatalf("backups %v", backups)
	}
	if b, _ := ioutil.ReadFile(backups[0]); string(b) != userSettings {
		t.Fatal("backup does not hold the original content")
	}

	if err := integrate(t, work, true, "--apply"); err != nil {
		t.Fatal(err)
	}
	if again := zeroturnHooks(readJSON(t, settingsFile(work))); !reflect.DeepEqual(again, got) {
		t.Fatalf("reinstall duplicated entries: %v", again)
	}

	if err := integrate(t, work, true, "--remove"); err != nil {
		t.Fatal(err)
	}
	removed := readJSON(t, settingsFile(work))
	if !reflect.DeepEqual(removed, original) {
		a, _ := json.MarshalIndent(removed, "", " ")
		t.Fatalf("after removal the file differs from the original:\n%s", a)
	}
	if err := integrate(t, work, true, "--remove"); err != nil {
		t.Fatalf("second removal: %v", err)
	}
}

func TestIntegrateKeepsExistingStatusLine(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	testutil.Write(t, work, ".claude/settings.local.json", `{"statusLine": {"type": "command", "command": "my-line"}}`)
	if err := integrate(t, work, true, "--apply"); err != nil {
		t.Fatal(err)
	}
	sl := readJSON(t, settingsFile(work))["statusLine"].(map[string]interface{})
	if sl["command"] != "my-line" {
		t.Fatalf("status line replaced without --replace-status-line: %v", sl)
	}
}

// A command the user added to the same entry as ZeroTurn's must survive.
func TestRemoveKeepsUserHookInSharedEntry(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	shared := `{"hooks": {"Stop": [{"hooks": [
  {"type": "command", "command": "\"/opt/zeroturn\" event --harness claude --event Stop"},
  {"type": "command", "command": "say done"}
]}]}}`
	testutil.Write(t, work, ".claude/settings.local.json", shared)
	if err := integrate(t, work, true, "--remove"); err != nil {
		t.Fatal(err)
	}
	m := readJSON(t, settingsFile(work))
	stop := m["hooks"].(map[string]interface{})["Stop"].([]interface{})
	inner := stop[0].(map[string]interface{})["hooks"].([]interface{})
	if len(inner) != 1 || inner[0].(map[string]interface{})["command"] != "say done" {
		t.Fatalf("remaining hooks %v", inner)
	}
}

// Removing the last entry leaves a tidy empty object, not a blank line.
func TestRemoveLeavesTidyFile(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	if err := integrate(t, work, true, "--apply"); err != nil {
		t.Fatal(err)
	}
	if err := integrate(t, work, true, "--remove"); err != nil {
		t.Fatal(err)
	}
	b, err := ioutil.ReadFile(settingsFile(work))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "{}\n" {
		t.Fatalf("settings file after removal: %q", b)
	}
}

func TestIntegrateRefusesInvalidJSON(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	testutil.Write(t, work, ".claude/settings.local.json", "{broken")
	err := integrate(t, work, true, "--apply")
	if ze, ok := err.(*output.Error); !ok || ze.Code != output.ExitInvalidUsage {
		t.Fatalf("err %v", err)
	}
	if b, _ := ioutil.ReadFile(settingsFile(work)); string(b) != "{broken" {
		t.Fatal("an invalid file was rewritten")
	}
}

func TestIntegrateDeclinedChangesNothing(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	testutil.Write(t, work, ".claude/settings.local.json", userSettings)
	err := integrate(t, work, false, "--apply")
	if ze, ok := err.(*output.Error); !ok || ze.Code != output.ExitDeclined {
		t.Fatalf("err %v", err)
	}
	if b, _ := ioutil.ReadFile(settingsFile(work)); string(b) != userSettings {
		t.Fatal("a declined change was written")
	}
}

func TestIntegrateNeverTouchesUserSettingsWithoutFlag(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	if err := integrate(t, work, true, "--apply"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(os.Getenv("HOME"), ".claude", "settings.json")); !os.IsNotExist(err) {
		t.Fatal("the user wide settings file was written")
	}
}

func TestIntegrateCopilotIsUnavailable(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	r := run(t, work, "", "integrate", "copilot", "--plan")
	if r.code != output.ExitNoIntegration {
		t.Fatalf("%+v", r)
	}
}
