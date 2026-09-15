package uninstall

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}
}

// The commands are the ones integrate writes. Ownership is decided by the
// ZeroTurn argument, not by the word zeroturn appearing in a path, so a
// fixture that only looks right is not owned and would prove nothing.
const zeroTurnSettings = `{
  "statusLine": {"type": "command", "command": "\"/usr/local/bin/zeroturn\" status --stdin --harness claude"},
  "hooks": {
    "PreToolUse": [
      {"matcher": "Agent", "hooks": [{"type": "command", "command": "\"/usr/local/bin/zeroturn\" event --harness claude --event PreToolUse"}]}
    ]
  }
}`

// A settings file that holds nothing of ZeroTurn's must not appear in the
// plan. Listing it would make a person answer for a file that will not be
// touched, and the count beside it would be zero.
func TestSurveySkipsASettingsFileWithNothingOfOurs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	write(t, path, `{"hooks": {"PreToolUse": [{"matcher": "Bash", "hooks": [{"type": "command", "command": "audit"}]}]}}`)

	p, err := Survey(Sources{SettingsFiles: []string{path}})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Items) != 0 {
		t.Fatalf("a file with no ZeroTurn entries was listed: %+v", p.Items)
	}
}

// A file ZeroTurn cannot read is the one case where saying nothing is
// worse than failing: the entries would be left behind and the plan would
// report a clean machine.
func TestSurveyRefusesAFileItCannotRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	write(t, path, "{not json")

	_, err := Survey(Sources{SettingsFiles: []string{path}})
	if err == nil {
		t.Fatal("a settings file that is not JSON was surveyed as if it were empty")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("the error does not say which file: %v", err)
	}
}

// The backup is the reason a person can answer yes, so it has to hold
// what the file said before the edit.
func TestTheBackupHoldsTheFileAsItWas(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "repo", ".claude", "settings.json")
	write(t, path, zeroTurnSettings)
	backups := filepath.Join(dir, "backups")

	p, err := Survey(Sources{SettingsFiles: []string{path}})
	if err != nil {
		t.Fatal(err)
	}
	if r := Apply(p, backups); len(r.Failed) != 0 {
		t.Fatalf("apply failed: %+v", r.Failed)
	}

	after, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(after), "zeroturn") {
		t.Errorf("the entries are still there:\n%s", after)
	}

	names, err := ioutil.ReadDir(backups)
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 1 {
		t.Fatalf("%d backups were written, want 1", len(names))
	}
	saved, err := ioutil.ReadFile(filepath.Join(backups, names[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	if string(saved) != zeroTurnSettings {
		t.Errorf("the backup is not the file as it was:\n%s", saved)
	}
}

// Stopping at the first failure would leave a harness calling ZeroTurn
// while its data is already gone, which is the one state worse than not
// having started.
func TestApplyKeepsGoingAfterAFailure(t *testing.T) {
	dir := t.TempDir()
	// A directory where a settings file is expected cannot be edited.
	bad := filepath.Join(dir, "settings.json")
	if err := os.MkdirAll(bad, 0700); err != nil {
		t.Fatal(err)
	}
	doomed := filepath.Join(dir, "state")
	write(t, filepath.Join(doomed, "sessions", "one.json"), "{}")

	p := Plan{Items: []Item{
		{Path: bad, Action: ActionEdit, Entries: 1},
		{Path: doomed, Action: ActionDelete},
	}}
	r := Apply(p, filepath.Join(dir, "backups"))

	if len(r.Failed) != 1 || r.Failed[0].Item.Path != bad {
		t.Fatalf("failures %+v, want the one settings path", r.Failed)
	}
	if len(r.Done) != 1 || r.Done[0].Path != doomed {
		t.Fatalf("done %+v, want the state directory", r.Done)
	}
	if _, err := os.Stat(doomed); !os.IsNotExist(err) {
		t.Errorf("the state directory survived a failure elsewhere: %v", err)
	}
}

// The plan says how much is about to be lost, and an empty state
// directory has to read differently from a full one.
func TestTheStateDirectoryIsCountedByWhatItHolds(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "state")
	write(t, filepath.Join(state, "sessions", "one.json"), "{}")
	write(t, filepath.Join(state, "sessions", "two.json"), "{}")
	write(t, filepath.Join(state, "trust", "repo.json"), "{}")

	p, err := Survey(Sources{StateDir: state})
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Items) != 1 {
		t.Fatalf("items %+v", p.Items)
	}
	if got, want := p.Items[0].Detail, "2 session records, 1 trust approval"; got != want {
		t.Errorf("detail %q, want %q", got, want)
	}
}

// Entries decides whether anything was taken out of a harness, which is
// what tells a person ZeroTurn has stopped running.
func TestEntriesCountsWhatWouldStopZeroTurnRunning(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	write(t, path, zeroTurnSettings)

	p, err := Survey(Sources{SettingsFiles: []string{path}, StateDir: filepath.Join(dir, "state")})
	if err != nil {
		t.Fatal(err)
	}
	// One hook and one status line, and the state directory is not there.
	if p.Entries() != 2 {
		t.Fatalf("Entries() is %d, want 2", p.Entries())
	}
}
