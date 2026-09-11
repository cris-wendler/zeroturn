package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cris-wendler/zeroturn/internal/git"
	"github.com/cris-wendler/zeroturn/internal/output"
	"github.com/cris-wendler/zeroturn/internal/state"
)

const integrateUsage = `zeroturn integrate <harness> --plan | --apply | --remove

  claude      Claude Code status line and subagent events
  copilot     GitHub Copilot CLI, experimental and partial

  --plan      Show what would change, changing nothing
  --apply     Write the change after confirmation
  --remove    Remove only the entries ZeroTurn owns
  --user      Act on the user wide settings file instead of this repository

Without --user the change goes to .claude/settings.local.json, the per user
settings file for this repository, because the entries contain the absolute
path of this machine's zeroturn executable.
`

// hookEntry mirrors the harness settings shape. Unknown fields in the
// user's own entries are preserved because untouched entries are never
// decoded into this type.
type hookEntry struct {
	Matcher string      `json:"matcher,omitempty"`
	Hooks   []hookInner `json:"hooks"`
}

type hookInner struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Timeout int    `json:"timeout,omitempty"`
}

type statusLineEntry struct {
	Type    string `json:"type"`
	Command string `json:"command"`
	Padding int    `json:"padding"`
}

type claudePlan struct {
	SettingsPath   string
	Backup         string
	Exists         bool
	AddStatusLine  bool
	KeepStatusLine string
	AddHooks       []string
	KeepHooks      int
	AlreadyOwned   []string
	Notes          []string
}

func cmdIntegrate(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, integrateUsage)
		return output.Errorf(output.ExitInvalidUsage, "zeroturn integrate changed nothing",
			"no harness was named", "run zeroturn integrate claude --plan")
	}
	harness := args[0]
	rest := args[1:]

	mode := ""
	userWide := false
	replaceStatus := false
	for _, a := range rest {
		switch a {
		case "--plan":
			mode = "plan"
		case "--apply":
			mode = "apply"
		case "--remove":
			mode = "remove"
		case "--user":
			userWide = true
		case "--replace-status-line":
			replaceStatus = true
		default:
			return output.Errorf(output.ExitInvalidUsage, "zeroturn integrate changed nothing",
				"flag "+a+" is not one zeroturn integrate accepts", "run zeroturn integrate --help")
		}
	}
	if mode == "" {
		fmt.Fprint(os.Stderr, integrateUsage)
		return output.Errorf(output.ExitInvalidUsage, "zeroturn integrate changed nothing",
			"none of --plan, --apply, or --remove was given",
			"start with zeroturn integrate "+harness+" --plan")
	}

	switch harness {
	case "claude":
		return integrateClaude(ctx, mode, userWide, replaceStatus)
	case "copilot":
		return output.Errorf(output.ExitNoIntegration,
			"zeroturn integrate changed nothing",
			"the Copilot integration is experimental and is not installed by this command",
			"follow docs/integrations/copilot.md, which explains what Copilot exposes today")
	default:
		return output.Errorf(output.ExitNoIntegration, "zeroturn integrate changed nothing",
			"ZeroTurn has no integration named "+harness, "run zeroturn integrate --help")
	}
}

func selfPath() string {
	p, err := os.Executable()
	if err != nil {
		return "zeroturn"
	}
	if resolved, rerr := filepath.EvalSymlinks(p); rerr == nil {
		return resolved
	}
	return p
}

func claudeSettingsPath(ctx context.Context, userWide bool) (string, error) {
	if userWide {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(home, ".claude", "settings.json"), nil
	}
	repo, err := openRepo(ctx)
	if err != nil {
		return "", err
	}
	return filepath.Join(repo.Root, ".claude", "settings.local.json"), nil
}

var claudeHookEvents = []string{"PreToolUse", "SubagentStart", "SubagentStop", "Stop", "SessionEnd"}

func zeroturnCommand(event string) string {
	if event == "" {
		return fmt.Sprintf("%q status --stdin --harness claude", selfPath())
	}
	return fmt.Sprintf("%q event --harness claude --event %s", selfPath(), event)
}

// owned reports whether a settings entry was written by ZeroTurn. The test
// looks for the executable name together with a ZeroTurn argument, so a
// user's own command that merely mentions the word is left alone.
func owned(command string) bool {
	if !strings.Contains(command, "zeroturn") {
		return false
	}
	return strings.Contains(command, "event --harness") || strings.Contains(command, "status --stdin")
}

func integrateClaude(ctx context.Context, mode string, userWide, replaceStatus bool) error {
	path, err := claudeSettingsPath(ctx, userWide)
	if err != nil {
		return output.Errorf(output.ExitInternal, "zeroturn integrate changed nothing", err.Error(),
			"check that your home directory is readable")
	}
	raw, readErr := ioutil.ReadFile(path)
	exists := readErr == nil
	if readErr != nil && !os.IsNotExist(readErr) {
		return output.Errorf(output.ExitInternal, "zeroturn integrate changed nothing",
			"the settings file could not be read", "check permissions on "+path)
	}
	if !exists {
		raw = []byte("{}")
	}

	top := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &top); err != nil {
		return output.Errorf(output.ExitInvalidUsage, "zeroturn integrate changed nothing",
			path+" is not valid JSON", "correct the file, then run zeroturn integrate again")
	}
	order := topLevelOrder(raw)

	st, serr := openStore()
	if serr != nil {
		return serr
	}
	backupDir := filepath.Join(st.Dir(), "backups")

	plan := claudePlan{SettingsPath: path, Exists: exists}
	plan.Backup = filepath.Join(backupDir, fmt.Sprintf("claude-settings-%s.json", time.Now().UTC().Format("20060102-150405")))

	hooks := map[string][]json.RawMessage{}
	if v, ok := top["hooks"]; ok {
		if err := json.Unmarshal(v, &hooks); err != nil {
			return output.Errorf(output.ExitInvalidUsage, "zeroturn integrate changed nothing",
				"the hooks section of "+path+" has a shape ZeroTurn does not recognise",
				"correct the file, then run zeroturn integrate again")
		}
	}

	for _, ev := range claudeHookEvents {
		found := false
		for _, entry := range hooks[ev] {
			if entryOwnedByZeroTurn(entry) {
				found = true
			} else {
				plan.KeepHooks++
			}
		}
		if found {
			plan.AlreadyOwned = append(plan.AlreadyOwned, ev)
		} else {
			plan.AddHooks = append(plan.AddHooks, ev)
		}
	}
	if !userWide && !gitIgnored(ctx, path) {
		plan.Notes = append(plan.Notes,
			path+" is not ignored by Git. The entries hold this machine's executable path, so add it to .gitignore before committing.")
	}

	if v, ok := top["statusLine"]; ok {
		var sl statusLineEntry
		if json.Unmarshal(v, &sl) == nil && owned(sl.Command) {
			plan.AlreadyOwned = append(plan.AlreadyOwned, "statusLine")
		} else {
			plan.KeepStatusLine = sl.Command
			plan.AddStatusLine = replaceStatus
			if !replaceStatus {
				plan.Notes = append(plan.Notes,
					"A status line is already configured and will be left in place. Pass --replace-status-line to replace it.")
			}
		}
	} else {
		plan.AddStatusLine = true
	}

	switch mode {
	case "plan":
		printClaudePlan(plan, top, order)
		return nil
	case "remove":
		return applyClaudeRemove(path, top, order, hooks, backupDir, plan)
	}
	return applyClaudeInstall(path, top, order, hooks, backupDir, plan, replaceStatus)
}

func entryOwnedByZeroTurn(raw json.RawMessage) bool {
	var e hookEntry
	if json.Unmarshal(raw, &e) != nil {
		return false
	}
	for _, h := range e.Hooks {
		if owned(h.Command) {
			return true
		}
	}
	return false
}

// withoutZeroTurnHooks removes ZeroTurn's own commands from one hook entry
// and keeps everything else in it, so a command a user added to the same
// entry survives removal. keep is false when nothing remains.
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
		var hi hookInner
		if json.Unmarshal(h, &hi) == nil && owned(hi.Command) {
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

func gitIgnored(ctx context.Context, path string) bool {
	repo, err := git.Open(ctx, filepath.Dir(filepath.Dir(path)))
	if err != nil {
		return false
	}
	return repo.IsIgnored(ctx, path)
}

func printClaudePlan(p claudePlan, top map[string]json.RawMessage, order []string) {
	fmt.Println("ZEROTURN INTEGRATE CLAUDE (plan)")
	fmt.Printf("settings file   %s\n", p.SettingsPath)
	if !p.Exists {
		fmt.Println("                the file does not exist yet and would be created")
	}
	fmt.Printf("backup          %s\n", p.Backup)
	fmt.Println()

	fmt.Println("would add:")
	if p.AddStatusLine {
		fmt.Printf("  statusLine    %s\n", zeroturnCommand(""))
	}
	for _, ev := range p.AddHooks {
		matcher := ""
		if ev == "PreToolUse" {
			matcher = " (matcher Agent, exact)"
		}
		fmt.Printf("  hooks.%-14s %s%s\n", ev, zeroturnCommand(ev), matcher)
	}
	if !p.AddStatusLine && len(p.AddHooks) == 0 {
		fmt.Println("  nothing, the integration is already installed")
	}
	fmt.Println()

	fmt.Println("would keep unchanged:")
	kept := 0
	for _, k := range order {
		if k == "hooks" || k == "statusLine" {
			continue
		}
		fmt.Printf("  %s\n", k)
		kept++
	}
	if p.KeepStatusLine != "" {
		fmt.Printf("  statusLine (yours)\n")
		kept++
	}
	if p.KeepHooks > 0 {
		fmt.Printf("  %d hook entries written by you or another tool\n", p.KeepHooks)
		kept++
	}
	if kept == 0 {
		fmt.Println("  nothing else is in this file")
	}
	if len(p.AlreadyOwned) > 0 {
		fmt.Printf("\nalready installed by ZeroTurn: %s\n", strings.Join(p.AlreadyOwned, ", "))
	}
	fmt.Println("\ncompatibility:")
	fmt.Println("  The subagent gate uses PreToolUse with the exact matcher Agent.")
	fmt.Println("  Subagent counts come from ZeroTurn's own events, not from the harness.")
	for _, n := range p.Notes {
		fmt.Printf("  %s\n", n)
	}
	fmt.Println("\nNothing was changed. Run the same command with --apply to write it.")
}

func applyClaudeInstall(path string, top map[string]json.RawMessage, order []string,
	hooks map[string][]json.RawMessage, backupDir string, p claudePlan, replaceStatus bool) error {

	if !p.AddStatusLine && len(p.AddHooks) == 0 {
		fmt.Println("The Claude integration is already installed. Nothing was changed.")
		return nil
	}
	printClaudePlan(p, top, order)
	fmt.Println()
	ok, err := confirm("Apply this change?")
	if err != nil {
		return err
	}
	if !ok {
		return output.Errorf(output.ExitDeclined, "zeroturn integrate changed nothing",
			"the change was declined", "run the command again when you are ready")
	}

	if p.Exists {
		if err := backupFile(path, p.Backup); err != nil {
			return output.Errorf(output.ExitInternal, "zeroturn integrate changed nothing",
				"the backup could not be written", "check that "+backupDir+" is writable")
		}
	}

	for _, ev := range p.AddHooks {
		entry := hookEntry{Hooks: []hookInner{{Type: "command", Command: zeroturnCommand(ev), Timeout: 10}}}
		if ev == "PreToolUse" {
			entry.Matcher = "Agent"
		}
		b, _ := json.Marshal(entry)
		hooks[ev] = append(hooks[ev], json.RawMessage(b))
	}
	if len(hooks) > 0 {
		b, _ := json.Marshal(hooks)
		top["hooks"] = json.RawMessage(b)
		order = ensureKey(order, "hooks")
	}
	if p.AddStatusLine || replaceStatus {
		b, _ := json.Marshal(statusLineEntry{Type: "command", Command: zeroturnCommand(""), Padding: 0})
		top["statusLine"] = json.RawMessage(b)
		order = ensureKey(order, "statusLine")
	}

	if err := writeOrdered(path, top, order); err != nil {
		return output.Errorf(output.ExitInternal, "zeroturn integrate could not write the settings file",
			err.Error(), "the backup at "+p.Backup+" holds the previous content")
	}
	fmt.Printf("Wrote %s\n", path)
	if p.Exists {
		fmt.Printf("Backup %s\n", p.Backup)
	}
	fmt.Println("Start a new coding session for the change to take effect.")
	return nil
}

func applyClaudeRemove(path string, top map[string]json.RawMessage, order []string,
	hooks map[string][]json.RawMessage, backupDir string, p claudePlan) error {

	if !p.Exists {
		fmt.Printf("No settings file at %s. Nothing to remove.\n", path)
		return nil
	}

	removed := 0
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

	removedStatus := false
	if v, ok := top["statusLine"]; ok {
		var sl statusLineEntry
		if json.Unmarshal(v, &sl) == nil && owned(sl.Command) {
			delete(top, "statusLine")
			order = removeKey(order, "statusLine")
			removedStatus = true
			removed++
		}
	}

	if removed == 0 {
		fmt.Println("No ZeroTurn entries were found. Nothing was changed.")
		return nil
	}

	fmt.Println("ZEROTURN INTEGRATE CLAUDE (remove)")
	fmt.Printf("settings file   %s\n", path)
	fmt.Printf("backup          %s\n", p.Backup)
	fmt.Printf("would remove    %d ZeroTurn entries\n", removed)
	if removedStatus {
		fmt.Println("                including the ZeroTurn status line")
	}
	fmt.Printf("would keep      %d entries written by you or another tool\n", p.KeepHooks)
	fmt.Println()
	ok, err := confirm("Remove the ZeroTurn entries?")
	if err != nil {
		return err
	}
	if !ok {
		return output.Errorf(output.ExitDeclined, "zeroturn integrate changed nothing",
			"the removal was declined", "run the command again when you are ready")
	}

	if err := backupFile(path, p.Backup); err != nil {
		return output.Errorf(output.ExitInternal, "zeroturn integrate changed nothing",
			"the backup could not be written", "check that "+backupDir+" is writable")
	}
	if len(hooks) == 0 {
		delete(top, "hooks")
		order = removeKey(order, "hooks")
	} else {
		b, _ := json.Marshal(hooks)
		top["hooks"] = json.RawMessage(b)
	}
	if err := writeOrdered(path, top, order); err != nil {
		return output.Errorf(output.ExitInternal, "zeroturn integrate could not write the settings file",
			err.Error(), "the backup at "+p.Backup+" holds the previous content")
	}
	fmt.Printf("Removed %d ZeroTurn entries from %s\n", removed, path)
	fmt.Printf("Backup %s\n", p.Backup)
	return nil
}

func backupFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0700); err != nil {
		return err
	}
	b, err := ioutil.ReadFile(src)
	if err != nil {
		return err
	}
	return state.AtomicWrite(dst, b, 0600)
}

// topLevelOrder records the order of keys in the original file so that an
// unrelated setting does not move when ZeroTurn rewrites the file.
func topLevelOrder(raw []byte) []string {
	dec := json.NewDecoder(bytes.NewReader(raw))
	tok, err := dec.Token()
	if err != nil {
		return nil
	}
	if d, ok := tok.(json.Delim); !ok || d != '{' {
		return nil
	}
	var keys []string
	depth := 0
	for {
		t, err := dec.Token()
		if err == io.EOF || err != nil {
			return keys
		}
		if d, ok := t.(json.Delim); ok {
			switch d {
			case '{', '[':
				depth++
			case '}', ']':
				if depth == 0 {
					return keys
				}
				depth--
			}
			continue
		}
		if depth == 0 {
			if s, ok := t.(string); ok {
				keys = append(keys, s)
				var skip json.RawMessage
				if dec.Decode(&skip) != nil {
					return keys
				}
			}
		}
	}
}

func ensureKey(order []string, key string) []string {
	for _, k := range order {
		if k == key {
			return order
		}
	}
	return append(order, key)
}

func removeKey(order []string, key string) []string {
	var out []string
	for _, k := range order {
		if k != key {
			out = append(out, k)
		}
	}
	return out
}

// writeOrdered rebuilds the file preserving the original key order and
// writes it atomically so an interruption cannot leave a partial file.
func writeOrdered(path string, top map[string]json.RawMessage, order []string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	if len(top) == 0 {
		// Removing the last entry leaves an empty settings file rather
		// than a file with a stray blank line in it.
		return state.AtomicWrite(path, []byte("{}\n"), 0644)
	}
	seen := map[string]bool{}
	var buf bytes.Buffer
	buf.WriteString("{\n")
	first := true
	emit := func(k string) error {
		v, ok := top[k]
		if !ok || seen[k] {
			return nil
		}
		seen[k] = true
		if !first {
			buf.WriteString(",\n")
		}
		first = false
		kb, _ := json.Marshal(k)
		buf.WriteString("  ")
		buf.Write(kb)
		buf.WriteString(": ")
		var indented bytes.Buffer
		if err := json.Indent(&indented, v, "  ", "  "); err != nil {
			return err
		}
		buf.Write(indented.Bytes())
		return nil
	}
	for _, k := range order {
		if err := emit(k); err != nil {
			return err
		}
	}
	rest := make([]string, 0, len(top))
	for k := range top {
		if !seen[k] {
			rest = append(rest, k)
		}
	}
	sortStrings(rest)
	for _, k := range rest {
		if err := emit(k); err != nil {
			return err
		}
	}
	buf.WriteString("\n}\n")
	return state.AtomicWrite(path, buf.Bytes(), 0644)
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}
