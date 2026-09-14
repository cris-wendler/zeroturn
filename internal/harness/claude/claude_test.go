package claude

import (
	"encoding/json"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/config"
)

const exe = "/usr/local/bin/zeroturn"

func TestInstalledPath(t *testing.T) {
	cases := map[string]string{
		`"/usr/local/bin/zeroturn" event --harness claude`: "/usr/local/bin/zeroturn",
		`/usr/local/bin/zeroturn event`:                    "/usr/local/bin/zeroturn",
		`"C:\\Program Files\\zeroturn.exe" status`:         `C:\\Program Files\\zeroturn.exe`,
		`zeroturn`: "zeroturn",
	}
	for command, want := range cases {
		if got := InstalledPath(command); got != want {
			t.Errorf("InstalledPath(%q) = %q, want %q", command, got, want)
		}
	}
}

// The settings file is JSON, whose encoder escapes backslashes. Quoting
// the path a second time stored every separator doubled, which is what
// happened before the command was built with plain quotation marks.
func TestCommandQuotingSurvivesAWindowsPath(t *testing.T) {
	command := Command(`C:\Users\dev\zeroturn.exe`, "Stop")
	b, err := json.Marshal(map[string]string{"command": command})
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]string
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	if got := InstalledPath(back["command"]); got != `C:\Users\dev\zeroturn.exe` {
		t.Fatalf("path after a round trip: %q", got)
	}
}

// A command that merely mentions the word belongs to whoever wrote it.
func TestOwnedNeedsAZeroTurnArgument(t *testing.T) {
	cases := map[string]bool{
		`"/bin/zeroturn" event --harness claude --event Stop`: true,
		`"/bin/zeroturn" status --stdin --harness claude`:     true,
		`echo zeroturn is installed`:                          false,
		`my-linter --config zeroturn.json`:                    false,
		`"/bin/other" event --harness claude`:                 false,
		``:                                                    false,
	}
	for command, want := range cases {
		if got := Owned(command); got != want {
			t.Errorf("Owned(%q) = %v, want %v", command, got, want)
		}
	}
}

// The prompt guard puts ZeroTurn in the path of what a developer types,
// so its hook is installed only when they have switched it on.
func TestThePromptHookFollowsTheSetting(t *testing.T) {
	has := func(hooks []Hook) bool {
		for _, h := range hooks {
			if h.Event == "UserPromptSubmit" {
				return true
			}
		}
		return false
	}
	if has(HooksFor(config.Default())) {
		t.Error("the prompt hook is installed by default")
	}
	on := config.Default()
	on.Guard.Credentials.Prompts = config.PromptsBlock
	if !has(HooksFor(on)) {
		t.Error("the prompt hook is missing when the prompt guard is on")
	}
	if has(Hooks) {
		t.Error("HooksFor appended to the shared table")
	}
}

func entry(matcher, command string) json.RawMessage {
	b, _ := json.Marshal(Entry{Matcher: matcher, Hooks: []Inner{{Type: "command", Command: command}}})
	return json.RawMessage(b)
}

func TestPlanOnAnEmptyFileInstallsEverything(t *testing.T) {
	p := Build(Input{Top: map[string]json.RawMessage{}, Hooks: map[string][]json.RawMessage{},
		Config: config.Default(), Exe: exe})
	if !p.AddStatusLine {
		t.Error("the status line would not be installed")
	}
	if len(p.AddHooks) != len(Hooks) {
		t.Errorf("would add %d hooks, want %d", len(p.AddHooks), len(Hooks))
	}
	if p.NothingToDo() {
		t.Error("a plan that installs everything reports nothing to do")
	}
}

func TestPlanOnAnInstalledFileAddsNothing(t *testing.T) {
	hooks := map[string][]json.RawMessage{}
	for _, h := range Hooks {
		hooks[h.Event] = append(hooks[h.Event], entry(h.Matcher, Command(exe, h.Event)))
	}
	top := map[string]json.RawMessage{"statusLine": StatusLineValue(exe)}

	p := Build(Input{Top: top, Hooks: hooks, Config: config.Default(), Exe: exe})
	if !p.NothingToDo() {
		t.Fatalf("a second plan would change something: add %v, repair %v, status %v",
			p.AddHooks, p.RepairHooks, p.AddStatusLine)
	}
	if len(p.AlreadyOwned) != len(Hooks)+1 {
		t.Errorf("already installed: %v", p.AlreadyOwned)
	}
}

// An entry pointing at an executable that has moved is repaired, not
// duplicated. A hook pointing at a path that is gone fails in silence.
func TestPlanRepairsAnEntryPointingElsewhere(t *testing.T) {
	old := "/somewhere/else/zeroturn"
	hooks := map[string][]json.RawMessage{}
	for _, h := range Hooks {
		hooks[h.Event] = append(hooks[h.Event], entry(h.Matcher, Command(old, h.Event)))
	}
	p := Build(Input{Top: map[string]json.RawMessage{}, Hooks: hooks, Config: config.Default(), Exe: exe})
	if len(p.AddHooks) != 0 {
		t.Fatalf("a stale entry was treated as missing: %v", p.AddHooks)
	}
	if len(p.RepairHooks) != len(Hooks) {
		t.Fatalf("would repair %d of %d", len(p.RepairHooks), len(Hooks))
	}
	for _, r := range p.RepairHooks {
		if r.From != old {
			t.Errorf("repair reports %q, want %q", r.From, old)
		}
		if !r.Gone {
			t.Errorf("a path that is not there was not reported as gone: %+v", r)
		}
	}

	Install(map[string]json.RawMessage{}, hooks, p, exe, false)
	for event, entries := range hooks {
		for _, e := range entries {
			if got := EntryCommand(e); got != Command(exe, event) {
				t.Errorf("%s still reads %q", event, got)
			}
		}
	}
}

// A status line belonging to someone else is left alone unless replacing
// it was asked for.
func TestPlanKeepsAStatusLineItDoesNotOwn(t *testing.T) {
	mine, _ := json.Marshal(StatusLine{Type: "command", Command: "my-prompt --fancy"})
	top := map[string]json.RawMessage{"statusLine": json.RawMessage(mine)}

	p := Build(Input{Top: top, Hooks: map[string][]json.RawMessage{}, Config: config.Default(), Exe: exe})
	if p.AddStatusLine {
		t.Error("someone else's status line would be replaced without being asked")
	}
	if p.KeepStatusLine != "my-prompt --fancy" {
		t.Errorf("kept %q", p.KeepStatusLine)
	}

	p = Build(Input{Top: top, Hooks: map[string][]json.RawMessage{}, Config: config.Default(),
		Exe: exe, ReplaceStatus: true})
	if !p.AddStatusLine {
		t.Error("the status line was not replaced when replacing it was asked for")
	}
}

// The rule the whole package rests on: a command someone else put in the
// same entry survives removal.
func TestRemoveKeepsWhatItDoesNotOwn(t *testing.T) {
	shared, _ := json.Marshal(Entry{Matcher: "Agent", Hooks: []Inner{
		{Type: "command", Command: Command(exe, "PreToolUse")},
		{Type: "command", Command: "my-own-hook --check"},
	}})
	hooks := map[string][]json.RawMessage{
		"PreToolUse": {json.RawMessage(shared)},
		"Stop":       {entry("", Command(exe, "Stop"))},
	}
	top := map[string]json.RawMessage{"statusLine": StatusLineValue(exe)}

	removed, removedStatus := Remove(top, hooks)
	if removed != 3 {
		t.Errorf("removed %d, want 3", removed)
	}
	if !removedStatus {
		t.Error("the ZeroTurn status line was not reported")
	}
	if _, ok := hooks["Stop"]; ok {
		t.Error("an event left with no entries was kept")
	}
	rest := hooks["PreToolUse"]
	if len(rest) != 1 {
		t.Fatalf("the shared entry was dropped: %v", rest)
	}
	var e Entry
	if err := json.Unmarshal(rest[0], &e); err != nil {
		t.Fatal(err)
	}
	if len(e.Hooks) != 1 || e.Hooks[0].Command != "my-own-hook --check" {
		t.Fatalf("the user's own command did not survive: %+v", e.Hooks)
	}
	if e.Matcher != "Agent" {
		t.Errorf("the entry lost its matcher: %q", e.Matcher)
	}
}

func TestRemoveOnAFileWithNoZeroTurnEntries(t *testing.T) {
	hooks := map[string][]json.RawMessage{"Stop": {entry("", "my-own-hook")}}
	top := map[string]json.RawMessage{}
	removed, removedStatus := Remove(top, hooks)
	if removed != 0 || removedStatus {
		t.Fatalf("removed %d entries from a file ZeroTurn does not appear in", removed)
	}
	if len(hooks["Stop"]) != 1 {
		t.Error("a hook that is not ZeroTurn's was dropped")
	}
}
