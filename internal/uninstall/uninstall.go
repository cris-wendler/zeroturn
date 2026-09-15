// Package uninstall finds what ZeroTurn has put on this machine and
// takes it away.
//
// Every location is supplied by the caller, computed by the package that
// writes there, so a removal cannot go looking in a directory that moved.
// A survey reports what it found without changing anything, because a
// person deserves to read the list before a file is deleted.
package uninstall

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"

	"github.com/cris-wendler/zeroturn/internal/harness/claude"
	"github.com/cris-wendler/zeroturn/internal/settings"
)

// What happens to one item. A settings file belongs to whoever wrote it,
// so ZeroTurn takes its own entries out and leaves the rest alone. A file
// that only ZeroTurn ever writes is deleted whole.
const (
	ActionEdit   = "edit"
	ActionDelete = "delete"
)

// Item is one thing a removal would touch.
type Item struct {
	Path   string
	Action string
	// What names the item in the words the plan prints.
	What string
	// Detail says how much is there, so an empty directory can be told
	// apart from a year of records before anyone answers a prompt.
	Detail string
	// Entries counts the ZeroTurn entries in a settings file, and Kept
	// counts the entries that belong to somebody else.
	Entries int
	Kept    int
}

// Sources are the places ZeroTurn writes.
type Sources struct {
	// StateDir holds session records, trust approvals, and backups.
	StateDir string
	// ConfigFiles are the per repository policy files that are known.
	// Only the current repository can be known, which is why the plan
	// says so rather than claiming to have found them all.
	ConfigFiles []string
	// SettingsFiles are harness settings files to take entries out of.
	SettingsFiles []string
}

// Plan is what a removal would do. Items is empty when this machine has
// nothing of ZeroTurn's left on it.
type Plan struct {
	Items []Item
}

// Entries reports how many harness entries the plan would remove, which
// is the number that decides whether ZeroTurn still runs afterwards.
func (p Plan) Entries() int {
	n := 0
	for _, it := range p.Items {
		n += it.Entries
	}
	return n
}

// Survey reports what is there. It opens files to count what they hold
// and writes nothing.
func Survey(s Sources) (Plan, error) {
	var p Plan
	// Settings come first, in the plan and in the removal, because taking
	// the entries out is what stops ZeroTurn running. Deleting its data
	// while a harness still calls it would only make the data again.
	for _, path := range s.SettingsFiles {
		it, found, err := surveySettings(path)
		if err != nil {
			return Plan{}, err
		}
		if found {
			p.Items = append(p.Items, it)
		}
	}
	for _, path := range s.ConfigFiles {
		if !exists(path) {
			continue
		}
		p.Items = append(p.Items, Item{
			Path:   path,
			Action: ActionDelete,
			What:   "repository policy",
		})
	}
	if s.StateDir != "" && exists(s.StateDir) {
		detail, err := stateDetail(s.StateDir)
		if err != nil {
			return Plan{}, err
		}
		p.Items = append(p.Items, Item{
			Path:   s.StateDir,
			Action: ActionDelete,
			What:   "session records, trust approvals, and backups",
			Detail: detail,
		})
	}
	return p, nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// stateDetail counts what the state directory holds by name, so the plan
// can say what is about to be lost rather than how many files there are.
func stateDetail(dir string) (string, error) {
	counts := map[string]int{}
	for _, sub := range []string{"sessions", "trust"} {
		names, err := ioutil.ReadDir(filepath.Join(dir, sub))
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", err
		}
		for _, n := range names {
			if !n.IsDir() {
				counts[sub]++
			}
		}
	}
	return fmt.Sprintf("%s, %s",
		plural(counts["sessions"], "session record"),
		plural(counts["trust"], "trust approval")), nil
}

func plural(n int, noun string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// surveySettings counts what ZeroTurn owns in one settings file. It works
// on the copy it parsed, so the file on disk is not changed.
func surveySettings(path string) (Item, bool, error) {
	f, err := settings.Read(path)
	if err != nil {
		if errors.Is(err, settings.ErrInvalidJSON) {
			return Item{}, false, fmt.Errorf("%s is not valid JSON", path)
		}
		return Item{}, false, err
	}
	if !f.Exists {
		return Item{}, false, nil
	}
	hooks, err := f.Section("hooks")
	if err != nil {
		return Item{}, false, fmt.Errorf(
			"the hooks section of %s has a shape ZeroTurn does not recognise", path)
	}
	removed, _ := claude.Remove(f.Top, hooks)
	if removed == 0 {
		return Item{}, false, nil
	}
	return Item{
		Path:    path,
		Action:  ActionEdit,
		What:    "harness entries",
		Entries: removed,
		Kept:    countKept(f.Top, hooks),
	}, true, nil
}

// countKept counts what is left after ZeroTurn's entries are gone, which
// is what the plan promises not to touch.
func countKept(top map[string]json.RawMessage, hooks map[string][]json.RawMessage) int {
	kept := 0
	for _, entries := range hooks {
		kept += len(entries)
	}
	if v, ok := top["statusLine"]; ok {
		var sl claude.StatusLine
		if json.Unmarshal(v, &sl) == nil && !claude.Owned(sl.Command) {
			kept++
		}
	}
	return kept
}

// Result reports what a removal actually did.
type Result struct {
	// Done lists the items that were removed or edited.
	Done []Item
	// Failed lists what could not be removed, with the reason, so the
	// command can say what is left rather than claiming a clean machine.
	Failed []Failure
}

// Failure is one item that stayed.
type Failure struct {
	Item   Item
	Reason string
}

// Apply carries out the plan. Every settings file is copied into backupDir
// before it is edited, so a mistake can be undone. It keeps going after a
// failure, because stopping halfway would leave a harness calling an
// executable whose data is already gone.
func Apply(p Plan, backupDir string) Result {
	var r Result
	for _, it := range p.Items {
		var err error
		switch it.Action {
		case ActionEdit:
			err = editSettings(it.Path, backupDir)
		default:
			err = os.RemoveAll(it.Path)
		}
		if err != nil {
			r.Failed = append(r.Failed, Failure{Item: it, Reason: err.Error()})
			continue
		}
		r.Done = append(r.Done, it)
	}
	return r
}

func editSettings(path, backupDir string) error {
	f, err := settings.Read(path)
	if err != nil {
		return err
	}
	if !f.Exists {
		return nil
	}
	hooks, err := f.Section("hooks")
	if err != nil {
		return err
	}
	removed, removedStatusLine := claude.Remove(f.Top, hooks)
	if removed == 0 {
		return nil
	}
	if backupDir != "" {
		if err := os.MkdirAll(backupDir, 0700); err != nil {
			return err
		}
		if err := f.Backup(filepath.Join(backupDir, backupName(path))); err != nil {
			return err
		}
	}
	if removedStatusLine {
		f.Delete("statusLine")
	}
	if len(hooks) == 0 {
		f.Delete("hooks")
	} else {
		b, err := json.Marshal(hooks)
		if err != nil {
			return err
		}
		f.Set("hooks", json.RawMessage(b))
	}
	return f.Write()
}

// backupName keeps the two settings files apart in one backup directory.
// They share a base name often enough that the repository one would
// otherwise overwrite the user wide one.
func backupName(path string) string {
	parent := filepath.Base(filepath.Dir(filepath.Dir(path)))
	return parent + "-" + filepath.Base(path)
}
