package settings

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.json")
	if err := ioutil.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

// A settings file that is not there is not a problem: writing it is how
// it gets created.
func TestAMissingFileReadsAsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "settings.json")
	f, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if f.Exists || len(f.Top) != 0 {
		t.Fatalf("exists %v with %d keys", f.Exists, len(f.Top))
	}
	f.Set("statusLine", json.RawMessage(`{"type":"command"}`))
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("the directory was not created: %v", err)
	}
}

func TestInvalidJSONIsReportedAsSuch(t *testing.T) {
	_, err := Read(write(t, "{ not json"))
	if !errors.Is(err, ErrInvalidJSON) {
		t.Fatalf("got %v, want ErrInvalidJSON", err)
	}
}

// The promise this package exists for. A rewrite must show only the
// change that was asked for, so keys keep their order and values this
// package does not understand keep their content.
func TestARewriteMovesAndDropsNothing(t *testing.T) {
	const original = `{
  "model": "opus",
  "permissions": {
    "allow": ["Bash(ls:*)"],
    "somethingZeroTurnHasNeverHeardOf": {"deep": [1, 2, {"x": null}]}
  },
  "env": {"FOO": "bar"},
  "statusLine": {"type": "command", "command": "mine"}
}`
	path := write(t, original)
	f, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.Keys(), ","); got != "model,permissions,env,statusLine" {
		t.Fatalf("key order read as %q", got)
	}
	f.Set("statusLine", json.RawMessage(`{"type":"command","command":"zeroturn"}`))
	f.Set("hooks", json.RawMessage(`{"Stop":[]}`))
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}

	b, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	out := string(b)
	// The unrelated keys are still there, in the order they were in.
	for _, pair := range [][2]string{{"model", "permissions"}, {"permissions", "env"}, {"env", "statusLine"}} {
		if strings.Index(out, `"`+pair[0]+`"`) > strings.Index(out, `"`+pair[1]+`"`) {
			t.Errorf("%s moved after %s:\n%s", pair[0], pair[1], out)
		}
	}
	// A new key goes at the end rather than in the middle of the file.
	if strings.Index(out, `"hooks"`) < strings.Index(out, `"statusLine"`) {
		t.Errorf("a new key was written before an existing one:\n%s", out)
	}
	var back map[string]interface{}
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatalf("the rewritten file is not valid JSON: %v\n%s", err, out)
	}
	perms := back["permissions"].(map[string]interface{})
	if _, ok := perms["somethingZeroTurnHasNeverHeardOf"]; !ok {
		t.Errorf("a value this package does not understand was dropped:\n%s", out)
	}
	if back["statusLine"].(map[string]interface{})["command"] != "zeroturn" {
		t.Errorf("the change that was asked for was not written:\n%s", out)
	}
}

func TestDeleteRemovesTheKeyAndItsPlace(t *testing.T) {
	f, err := Read(write(t, `{"a": 1, "statusLine": {"x": 1}, "b": 2}`))
	if err != nil {
		t.Fatal(err)
	}
	f.Delete("statusLine")
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}
	b, _ := ioutil.ReadFile(f.Path)
	if strings.Contains(string(b), "statusLine") {
		t.Fatalf("the key survived:\n%s", b)
	}
	if got := strings.Join(f.Keys(), ","); got != "a,b" {
		t.Errorf("order after delete: %q", got)
	}
}

// Removing the last entry leaves a file a harness can still read.
func TestAnEmptiedFileIsStillValidJSON(t *testing.T) {
	f, err := Read(write(t, `{"statusLine": {"x": 1}}`))
	if err != nil {
		t.Fatal(err)
	}
	f.Delete("statusLine")
	if err := f.Write(); err != nil {
		t.Fatal(err)
	}
	b, _ := ioutil.ReadFile(f.Path)
	if string(b) != "{}\n" {
		t.Fatalf("emptied file reads %q", b)
	}
}

func TestSectionReportsAShapeItCannotRead(t *testing.T) {
	f, err := Read(write(t, `{"hooks": "not an object"}`))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Section("hooks"); !errors.Is(err, ErrInvalidJSON) {
		t.Fatalf("got %v, want ErrInvalidJSON", err)
	}
	if s, err := f.Section("absent"); err != nil || len(s) != 0 {
		t.Fatalf("an absent section read as %v, %v", s, err)
	}
}

func TestBackupCopiesWhatIsOnDisk(t *testing.T) {
	path := write(t, `{"a": 1}`)
	f, err := Read(path)
	if err != nil {
		t.Fatal(err)
	}
	// The copy is of the file, not of what is about to be written.
	f.Set("a", json.RawMessage(`2`))
	dst := filepath.Join(t.TempDir(), "backups", "settings.json")
	if err := f.Backup(dst); err != nil {
		t.Fatal(err)
	}
	b, err := ioutil.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != `{"a": 1}` {
		t.Fatalf("backup holds %q", b)
	}
}
