package claude

import (
	"encoding/json"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/config"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
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

// A repair rewrites ZeroTurn's own entries and nothing else. The test
// above installs into a file that holds only ZeroTurn entries, so a
// repair that rewrote every command it could read would pass it.
func TestARepairLeavesSomebodyElsesCommandAlone(t *testing.T) {
	const foreign = "other-tool --watch"
	old := "/somewhere/else/zeroturn"
	hooks := map[string][]json.RawMessage{}
	for _, h := range Hooks {
		hooks[h.Event] = append(hooks[h.Event], entry(h.Matcher, Command(old, h.Event)))
	}
	// One more entry on an event ZeroTurn also uses, belonging to
	// somebody else, and a status line that is not ZeroTurn's either.
	event := Hooks[0].Event
	hooks[event] = append(hooks[event], entry("", foreign))
	mine, _ := json.Marshal(StatusLine{Type: "command", Command: foreign})
	top := map[string]json.RawMessage{"statusLine": json.RawMessage(mine)}

	p := Build(Input{Top: top, Hooks: hooks, Config: config.Default(), Exe: exe})
	Install(top, hooks, p, exe, false)

	// EntryCommand answers only for entries ZeroTurn owns, so the
	// foreign one is looked for in the raw JSON.
	var found bool
	for _, e := range hooks[event] {
		if strings.Contains(string(e), foreign) {
			found = true
		}
	}
	if !found {
		t.Errorf("a command ZeroTurn does not own was rewritten: %s", hooks[event])
	}
	var sl StatusLine
	if err := json.Unmarshal(top["statusLine"], &sl); err != nil {
		t.Fatal(err)
	}
	if sl.Command != foreign {
		t.Errorf("a status line ZeroTurn does not own became %q", sl.Command)
	}
}

// ZeroTurn installs two hooks on PreToolUse, one per matcher, so the
// entries on that event are examined twice. What the plan reports it
// would leave behind is the number of entries, not the number of times
// it looked at them.
func TestAForeignEntryIsCountedOnceHoweverOftenItIsExamined(t *testing.T) {
	var event string
	seen := map[string]bool{}
	for _, h := range Hooks {
		if seen[h.Event] {
			event = h.Event
		}
		seen[h.Event] = true
	}
	if event == "" {
		t.Skip("no event carries two ZeroTurn hooks any more")
	}
	hooks := map[string][]json.RawMessage{event: {entry("", "one-tool")}}
	p := Build(Input{Top: map[string]json.RawMessage{}, Hooks: hooks, Config: config.Default(), Exe: exe})
	if p.KeepHooks != 1 {
		t.Errorf("one entry on %s was counted as %d", event, p.KeepHooks)
	}
}

// What the plan reports as already installed has to be what a person
// would look for in the settings file, which for a hook with a matcher
// is both words: one event can hold several entries.
func TestThePlanNamesAnInstalledHookByItsEventAndMatcher(t *testing.T) {
	hooks := map[string][]json.RawMessage{}
	for _, h := range Hooks {
		hooks[h.Event] = append(hooks[h.Event], entry(h.Matcher, Command(exe, h.Event)))
	}
	p := Build(Input{Top: map[string]json.RawMessage{}, Hooks: hooks, Config: config.Default(), Exe: exe})
	named := strings.Join(p.AlreadyOwned, ",")

	var withMatcher, without string
	for _, h := range Hooks {
		if h.Matcher != "" && withMatcher == "" {
			withMatcher = h.Event + " " + h.Matcher
		}
		if h.Matcher == "" && without == "" {
			without = h.Event
		}
	}
	if withMatcher == "" || without == "" {
		t.Fatal("the hook list no longer holds both shapes, so this test checks nothing")
	}
	if !strings.Contains(named, withMatcher) {
		t.Errorf("a hook with a matcher is named without it: %v", p.AlreadyOwned)
	}
	if !strings.Contains(named, without) {
		t.Errorf("a hook with no matcher is not named at all: %v", p.AlreadyOwned)
	}
}

// Remove takes out ZeroTurn's status line and leaves anybody else's,
// which is the same rule the hooks follow and was only ever tested for
// the hooks.
func TestRemoveKeepsAStatusLineItDoesNotOwn(t *testing.T) {
	const foreign = "my-prompt --fancy"
	mine, _ := json.Marshal(StatusLine{Type: "command", Command: foreign})
	top := map[string]json.RawMessage{"statusLine": json.RawMessage(mine)}

	removed, removedStatus := Remove(top, map[string][]json.RawMessage{})
	if removed != 0 || removedStatus {
		t.Errorf("removed %d entries and reported the status line as %v", removed, removedStatus)
	}
	var sl StatusLine
	if err := json.Unmarshal(top["statusLine"], &sl); err != nil {
		t.Fatal(err)
	}
	if sl.Command != foreign {
		t.Errorf("the status line became %q", sl.Command)
	}
}

// Install repoints a ZeroTurn status line that names an executable that
// is no longer there, and leaves one already naming this executable
// exactly as it found it.
func TestInstallRepointsOnlyAStaleStatusLine(t *testing.T) {
	stale, _ := json.Marshal(StatusLine{Type: "command", Command: Command("/gone/zeroturn", "")})
	top := map[string]json.RawMessage{"statusLine": json.RawMessage(stale)}
	hooks := map[string][]json.RawMessage{}

	p := Build(Input{Top: top, Hooks: hooks, Config: config.Default(), Exe: exe})
	Install(top, hooks, p, exe, false)
	var sl StatusLine
	if err := json.Unmarshal(top["statusLine"], &sl); err != nil {
		t.Fatal(err)
	}
	if got := InstalledPath(sl.Command); got != exe {
		t.Errorf("a stale status line still names %q", got)
	}

	current := top["statusLine"]
	p = Build(Input{Top: top, Hooks: hooks, Config: config.Default(), Exe: exe})
	Install(top, hooks, p, exe, false)
	if string(top["statusLine"]) != string(current) {
		t.Errorf("a status line that was already right was rewritten to %s", top["statusLine"])
	}
}

// Two paths that lead to the same file are the same installation, and
// two that do not are not. A missing path is not the same as anything,
// including another missing path.
func TestSamePathFollowsLinksAndRefusesWhatIsNotThere(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "zeroturn")
	if err := ioutil.WriteFile(real, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symbolic links are not available here: %v", err)
	}
	other := filepath.Join(dir, "other")
	if err := ioutil.WriteFile(other, []byte("#!/bin/sh\n"), 0755); err != nil {
		t.Fatal(err)
	}

	if !SamePath(real, link) {
		t.Error("a link to the executable reads as a different installation")
	}
	if SamePath(real, other) {
		t.Error("two different files read as one installation")
	}
	missing := filepath.Join(dir, "gone")
	if SamePath(real, missing) || SamePath(missing, real) {
		t.Error("a path that is not there matched one that is")
	}
	if SamePath(missing, filepath.Join(dir, "also-gone")) {
		t.Error("two paths that are not there matched each other")
	}
}

// The path is read out of a command a settings file holds, which may be
// anything at all by the time somebody has edited it by hand.
func TestInstalledPathOnCommandsNobodyMeantToWrite(t *testing.T) {
	cases := map[string]string{
		` zeroturn event`: " zeroturn event",
		`""`:              "",
		`" "`:             " ",
		``:                "",
	}
	for command, want := range cases {
		if got := InstalledPath(command); got != want {
			t.Errorf("InstalledPath(%q) = %q, want %q", command, got, want)
		}
	}
}

// One entry can hold two commands, ZeroTurn's and somebody else's, and
// a repair has to rewrite only the first. Separate entries prove less:
// an entry ZeroTurn does not own is never opened at all.
func TestARepairInsideASharedEntryTouchesOnlyItsOwnCommand(t *testing.T) {
	const foreign = "my-own-hook --check"
	shared, _ := json.Marshal(Entry{Matcher: "Agent", Hooks: []Inner{
		{Type: "command", Command: Command("/gone/zeroturn", "PreToolUse")},
		{Type: "command", Command: foreign},
	}})
	hooks := map[string][]json.RawMessage{"PreToolUse": {json.RawMessage(shared)}}
	top := map[string]json.RawMessage{}

	p := Build(Input{Top: top, Hooks: hooks, Config: config.Default(), Exe: exe})
	if len(p.RepairHooks) == 0 {
		t.Fatal("a stale command in a shared entry was not reported as needing repair")
	}
	Install(top, hooks, p, exe, false)

	var e Entry
	if err := json.Unmarshal(hooks["PreToolUse"][0], &e); err != nil {
		t.Fatal(err)
	}
	if len(e.Hooks) != 2 {
		t.Fatalf("the entry now holds %d commands: %+v", len(e.Hooks), e.Hooks)
	}
	var mine, theirs int
	for _, h := range e.Hooks {
		switch {
		case h.Command == foreign:
			theirs++
		case InstalledPath(h.Command) == exe:
			mine++
		}
	}
	if theirs != 1 {
		t.Errorf("the command ZeroTurn does not own was rewritten: %+v", e.Hooks)
	}
	if mine != 1 {
		t.Errorf("the stale ZeroTurn command was not repointed: %+v", e.Hooks)
	}
}
