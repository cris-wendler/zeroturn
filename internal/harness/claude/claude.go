// Package claude knows which settings entries ZeroTurn owns in a Claude
// Code settings file, and what has to change to install, repair, or
// remove them.
//
// Ownership is the rule everything here rests on. ZeroTurn writes and
// removes its own entries and never touches anything else, including a
// command a person added to the same entry as one of ZeroTurn's.
package claude

import (
	"encoding/json"
	"os"
	"strings"

	"github.com/cris-wendler/zeroturn/internal/config"
)

// Hook is one settings entry ZeroTurn owns. Matchers are exact tool
// names, never patterns, so a hook fires for the tool it names and for
// nothing else.
type Hook struct {
	Event   string
	Matcher string
	Purpose string
}

// Hooks are the entries every installation gets.
var Hooks = []Hook{
	{"PreToolUse", "Agent", "the subagent gate"},
	{"PreToolUse", "Read", "the credential guard, before a file is read"},
	{"SubagentStart", "", "counts a subagent as started"},
	{"SubagentStop", "", "counts it as stopped"},
	{"Stop", "", "reads how many background tasks are running"},
	{"SessionEnd", "", "marks the end of the session"},
}

// HooksFor returns the entries to install for this configuration. The
// prompt hook is included only when the prompt guard is switched on, so
// a developer who has not asked for it never has ZeroTurn in the path of
// their messages.
func HooksFor(c config.Config) []Hook {
	if c.Guard.Credentials.Prompts != config.PromptsBlock {
		return Hooks
	}
	return append(append([]Hook{}, Hooks...),
		Hook{"UserPromptSubmit", "", "the prompt guard, before a message is sent"})
}

// Entry mirrors the harness settings shape. Unknown fields in the user's
// own entries are preserved because untouched entries are never decoded
// into this type.
type Entry struct {
	Matcher string  `json:"matcher,omitempty"`
	Hooks   []Inner `json:"hooks"`
}

type Inner struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

type StatusLine struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Padding int    `json:"padding"`
}

// Command builds the command for one event, or for the status line when
// the event is empty. The path is quoted with plain quotation marks
// rather than Go quoting: the settings file is JSON, and its encoder
// already escapes the backslashes in a Windows path. Quoting them a
// second time stored every separator doubled.
func Command(exe, event string) string {
	return `"` + exe + `"` + commandArgs(event)
}

func commandArgs(event string) string {
	if event == "" {
		return " status --stdin --harness claude"
	}
	return " event --harness claude --event " + event
}

// PluginExecutable is how a plugin names ZeroTurn. A plugin does not
// carry the binary, so the name is resolved on PATH.
const PluginExecutable = "zeroturn"

// PluginCommand is Command without a path, and needs no quoting.
func PluginCommand(event string) string {
	return PluginExecutable + commandArgs(event)
}

// PluginHooks is the hooks.json a plugin ships, built from the same list
// integrate writes. A plugin cannot carry a status line, so it installs
// the hooks and nothing else.
//
// The prompt guard is absent deliberately: it is off by default, and a
// plugin must not put ZeroTurn in the path of a person's messages
// without them asking.
// PluginHooksJSON is the file contents, so the generator and the test
// compare the same bytes.
func PluginHooksJSON() ([]byte, error) {
	doc := struct {
		Hooks map[string][]Entry `json:"hooks"`
	}{PluginHooks()}
	b, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func PluginHooks() map[string][]Entry {
	out := map[string][]Entry{}
	for _, h := range Hooks {
		out[h.Event] = append(out[h.Event], Entry{
			Matcher: h.Matcher,
			Hooks:   []Inner{{Type: "command", Command: PluginCommand(h.Event), Timeout: 10}},
		})
	}
	return out
}

// Owned reports whether a settings command was written by ZeroTurn. The
// test looks for the executable name together with a ZeroTurn argument,
// so a user's own command that merely mentions the word is left alone.
func Owned(command string) bool {
	if !strings.Contains(command, "zeroturn") {
		return false
	}
	return strings.Contains(command, "event --harness") || strings.Contains(command, "status --stdin")
}

// InstalledPath reads the executable out of a settings entry ZeroTurn
// wrote. The command is quoted, so the path is what lies between the
// first pair of quotation marks.
func InstalledPath(command string) string {
	if !strings.HasPrefix(command, `"`) {
		if i := strings.Index(command, " "); i > 0 {
			return command[:i]
		}
		return command
	}
	if end := strings.Index(command[1:], `"`); end > 0 {
		return command[1 : end+1]
	}
	return ""
}

// StalePath reports whether a settings entry points somewhere other than
// exe. A hook pointing at a path that is gone fails in silence, which
// for a guard is the worst way to fail.
func StalePath(command, exe string) (path string, stale bool) {
	path = InstalledPath(command)
	if path == "" || SamePath(path, exe) {
		return path, false
	}
	return path, true
}

// SamePath compares two paths as files rather than as text, so a path
// reached through a symbolic link, which is how a package manager
// usually installs an executable, is not reported as a different one.
func SamePath(a, b string) bool {
	if a == b {
		return true
	}
	ai, aerr := os.Stat(a)
	bi, berr := os.Stat(b)
	if aerr != nil || berr != nil {
		return false
	}
	return os.SameFile(ai, bi)
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// EntryCommand returns the first ZeroTurn command in an entry.
func EntryCommand(raw json.RawMessage) string {
	var e Entry
	if json.Unmarshal(raw, &e) != nil {
		return ""
	}
	for _, h := range e.Hooks {
		if Owned(h.Command) {
			return h.Command
		}
	}
	return ""
}

// EntryOwned reports whether this entry is one ZeroTurn wrote. A matcher
// narrows the test to the entry for one tool; an empty matcher matches
// any ZeroTurn entry.
func EntryOwned(raw json.RawMessage, matcher string) bool {
	var e Entry
	if json.Unmarshal(raw, &e) != nil {
		return false
	}
	if matcher != "" && e.Matcher != matcher {
		return false
	}
	for _, h := range e.Hooks {
		if Owned(h.Command) {
			return true
		}
	}
	return false
}

// repointEntry rewrites only the ZeroTurn commands inside an entry,
// keeping every other field, including ones ZeroTurn does not know.
func repointEntry(raw json.RawMessage, command string) json.RawMessage {
	var entry map[string]json.RawMessage
	if json.Unmarshal(raw, &entry) != nil {
		return raw
	}
	var inner []map[string]json.RawMessage
	if json.Unmarshal(entry["hooks"], &inner) != nil {
		return raw
	}
	for i, h := range inner {
		var cmd string
		if json.Unmarshal(h["command"], &cmd) == nil && Owned(cmd) {
			b, _ := json.Marshal(command)
			inner[i]["command"] = b
		}
	}
	b, _ := json.Marshal(inner)
	entry["hooks"] = b
	out, err := json.Marshal(entry)
	if err != nil {
		return raw
	}
	return out
}

// withoutZeroTurnHooks removes ZeroTurn's own commands from one hook
// entry and keeps everything else in it, so a command a user added to
// the same entry survives removal. keep is false when nothing remains.
func withoutZeroTurnHooks(raw json.RawMessage) (rest json.RawMessage, removed int, keep bool) {
	var entry map[string]json.RawMessage
	if json.Unmarshal(raw, &entry) != nil {
		return raw, 0, true
	}
	var inner []json.RawMessage
	if json.Unmarshal(entry["hooks"], &inner) != nil {
		return raw, 0, true
	}
	var kept []json.RawMessage
	for _, h := range inner {
		var hi Inner
		if json.Unmarshal(h, &hi) == nil && Owned(hi.Command) {
			removed++
			continue
		}
		kept = append(kept, h)
	}
	if removed == 0 {
		return raw, 0, true
	}
	if len(kept) == 0 {
		return nil, removed, false
	}
	b, _ := json.Marshal(kept)
	entry["hooks"] = b
	out, _ := json.Marshal(entry)
	return out, removed, true
}
