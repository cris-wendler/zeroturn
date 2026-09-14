package claude

import (
	"encoding/json"

	"github.com/cris-wendler/zeroturn/internal/config"
)

// Repair is an entry ZeroTurn owns whose executable path no longer
// matches this one.
type Repair struct {
	Where string
	From  string
	// Gone reports that the path it points at is not there at all, which
	// means the entry does nothing.
	Gone bool
}

// Plan is what an install would change, worked out before anything is
// written. Nothing here touches the file: the same value is what --plan
// prints and what --apply acts on, so the two cannot disagree.
type Plan struct {
	AddStatusLine bool
	// KeepStatusLine holds a status line belonging to someone else, which
	// is left in place unless replacing it was asked for.
	KeepStatusLine string
	AddHooks       []Hook
	RepairHooks    []Repair
	// KeepHooks counts hook entries written by the user or another tool.
	KeepHooks    int
	AlreadyOwned []string
}

// Input is what the plan is worked out from: the settings as they stand,
// the configuration that decides which hooks belong, and the executable
// the entries should point at.
type Input struct {
	Top           map[string]json.RawMessage
	Hooks         map[string][]json.RawMessage
	Config        config.Config
	Exe           string
	ReplaceStatus bool
}

// Build works out what an install would change.
func Build(in Input) Plan {
	var p Plan

	counted := map[string]bool{}
	for _, h := range HooksFor(in.Config) {
		found := false
		for _, entry := range in.Hooks[h.Event] {
			if EntryOwned(entry, h.Matcher) {
				found = true
				if path, stale := StalePath(EntryCommand(entry), in.Exe); stale {
					p.RepairHooks = append(p.RepairHooks, Repair{
						Where: "hooks." + h.Event + " " + h.Matcher, From: path, Gone: !pathExists(path)})
				}
				continue
			}
			// Entries not owned by ZeroTurn are counted once per event,
			// however many ZeroTurn entries that event holds.
			if !counted[h.Event] && !EntryOwned(entry, "") {
				p.KeepHooks++
			}
		}
		counted[h.Event] = true
		if found {
			name := h.Event
			if h.Matcher != "" {
				name += " " + h.Matcher
			}
			p.AlreadyOwned = append(p.AlreadyOwned, name)
		} else {
			p.AddHooks = append(p.AddHooks, h)
		}
	}

	v, ok := in.Top["statusLine"]
	if !ok {
		p.AddStatusLine = true
		return p
	}
	var sl StatusLine
	if json.Unmarshal(v, &sl) == nil && Owned(sl.Command) {
		p.AlreadyOwned = append(p.AlreadyOwned, "statusLine")
		if path, stale := StalePath(sl.Command, in.Exe); stale {
			p.RepairHooks = append(p.RepairHooks, Repair{Where: "statusLine", From: path, Gone: !pathExists(path)})
		}
		return p
	}
	// A status line someone else configured is left alone unless
	// replacing it was asked for.
	p.KeepStatusLine = sl.Command
	p.AddStatusLine = in.ReplaceStatus
	return p
}

// NothingToDo reports a plan that would change nothing.
func (p Plan) NothingToDo() bool {
	return !p.AddStatusLine && len(p.AddHooks) == 0 && len(p.RepairHooks) == 0
}

// Install applies the plan to the settings in memory. The caller writes
// the file.
func Install(top map[string]json.RawMessage, hooks map[string][]json.RawMessage, p Plan, exe string, replaceStatus bool) {
	// An entry ZeroTurn owns that points at another executable is
	// rewritten to point here. Entries it does not own are left alone.
	if len(p.RepairHooks) > 0 {
		for event, entries := range hooks {
			for i, entry := range entries {
				if !EntryOwned(entry, "") {
					continue
				}
				if _, stale := StalePath(EntryCommand(entry), exe); !stale {
					continue
				}
				entries[i] = repointEntry(entry, Command(exe, event))
			}
		}
		if v, ok := top["statusLine"]; ok {
			var sl StatusLine
			if json.Unmarshal(v, &sl) == nil && Owned(sl.Command) {
				if _, stale := StalePath(sl.Command, exe); stale {
					sl.Command = Command(exe, "")
					b, _ := json.Marshal(sl)
					top["statusLine"] = json.RawMessage(b)
				}
			}
		}
	}

	for _, h := range p.AddHooks {
		entry := Entry{Matcher: h.Matcher,
			Hooks: []Inner{{Type: "command", Command: Command(exe, h.Event), Timeout: 10}}}
		b, _ := json.Marshal(entry)
		hooks[h.Event] = append(hooks[h.Event], json.RawMessage(b))
	}
}

// StatusLineValue is the status line entry to store for this executable.
func StatusLineValue(exe string) json.RawMessage {
	b, _ := json.Marshal(StatusLine{Type: "command", Command: Command(exe, ""), Padding: 0})
	return json.RawMessage(b)
}

// Remove takes ZeroTurn's entries out of the settings in memory and
// reports how many it took out. An entry holding a command the user
// added keeps that command; an event left with no entries is dropped.
func Remove(top map[string]json.RawMessage, hooks map[string][]json.RawMessage) (removed int, removedStatusLine bool) {
	for ev, entries := range hooks {
		var kept []json.RawMessage
		for _, e := range entries {
			rest, n, keep := withoutZeroTurnHooks(e)
			removed += n
			if keep {
				kept = append(kept, rest)
			}
		}
		if len(kept) == 0 {
			delete(hooks, ev)
			continue
		}
		hooks[ev] = kept
	}

	if v, ok := top["statusLine"]; ok {
		var sl StatusLine
		if json.Unmarshal(v, &sl) == nil && Owned(sl.Command) {
			removedStatusLine = true
			removed++
		}
	}
	return removed, removedStatusLine
}
