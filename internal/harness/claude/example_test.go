package claude

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"testing"
)

// integrations/claude/settings.example.json is an install path, not an
// illustration: the integration document tells a reader to copy the
// entries out of it and replace the executable path. Nothing in Go had
// ever opened it, and it was missing the credential guard, so anyone who
// installed by hand lost that guard with nothing to tell them.
func TestTheExampleSettingsInstallEveryHook(t *testing.T) {
	path := filepath.Join("..", "..", "..", "integrations", "claude", "settings.example.json")
	b, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var file struct {
		Hooks      map[string][]json.RawMessage `json:"hooks"`
		StatusLine json.RawMessage              `json:"statusLine"`
	}
	if err := json.Unmarshal(b, &file); err != nil {
		t.Fatalf("%s is not valid JSON: %v", path, err)
	}

	// Every hook the installer writes has to be in the file somebody
	// copies by hand.
	for _, h := range Hooks {
		found := false
		for _, entry := range file.Hooks[h.Event] {
			if EntryOwned(entry, h.Matcher) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("the example settings have no %s entry with matcher %q, which is %s",
				h.Event, h.Matcher, h.Purpose)
		}
	}

	// And nothing in the file that the installer would not write, or a
	// hand install gets an entry ZeroTurn no longer answers.
	installs := map[string]map[string]bool{}
	for _, h := range Hooks {
		if installs[h.Event] == nil {
			installs[h.Event] = map[string]bool{}
		}
		installs[h.Event][h.Matcher] = true
	}
	for event, entries := range file.Hooks {
		for _, entry := range entries {
			if EntryCommand(entry) == "" {
				continue
			}
			var e Entry
			if json.Unmarshal(entry, &e) != nil {
				continue
			}
			if !installs[event][e.Matcher] {
				t.Errorf("the example settings install %s with matcher %q, which ZeroTurn does not write",
					event, e.Matcher)
			}
		}
	}

	if file.StatusLine == nil {
		t.Error("the example settings have no status line, which is where every measurement comes from")
	}
}
