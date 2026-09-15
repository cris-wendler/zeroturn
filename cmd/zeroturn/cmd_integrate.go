package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/git"
	"github.com/cris-wendler/zeroturn/internal/harness/claude"
	"github.com/cris-wendler/zeroturn/internal/output"
	"github.com/cris-wendler/zeroturn/internal/settings"
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

func cmdIntegrate(ctx context.Context, args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, integrateUsage)
		return output.Errorf(output.ExitInvalidUsage, "zeroturn integrate changed nothing",
			"no harness was named", "run zeroturn integrate claude --plan")
	}
	if args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		fmt.Print(integrateUsage)
		return errHelp
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
		case "--help", "-h":
			fmt.Print(integrateUsage)
			return errHelp
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
			"run zeroturn capabilities --json, which reports what each harness supports")
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

// claudePlan is the plan together with what the command needs to report
// it: where the file is, where the backup goes, and anything the person
// running it should know before they answer.
type claudePlan struct {
	claude.Plan
	SettingsPath string
	Backup       string
	Exists       bool
	Notes        []string
}

func integrateClaude(ctx context.Context, mode string, userWide, replaceStatus bool) error {
	path, err := claudeSettingsPath(ctx, userWide)
	if err != nil {
		return output.Errorf(output.ExitInternal, "zeroturn integrate changed nothing", err.Error(),
			"check that your home directory is readable")
	}
	file, err := settings.Read(path)
	if err != nil {
		if errors.Is(err, settings.ErrInvalidJSON) {
			return output.Errorf(output.ExitInvalidUsage, "zeroturn integrate changed nothing",
				path+" is not valid JSON", "correct the file, then run zeroturn integrate again")
		}
		return output.Errorf(output.ExitInternal, "zeroturn integrate changed nothing",
			"the settings file could not be read", "check permissions on "+path)
	}
	hooks, err := file.Section("hooks")
	if err != nil {
		return output.Errorf(output.ExitInvalidUsage, "zeroturn integrate changed nothing",
			"the hooks section of "+path+" has a shape ZeroTurn does not recognise",
			"correct the file, then run zeroturn integrate again")
	}

	st, serr := openStore()
	if serr != nil {
		return serr
	}
	backupDir := filepath.Join(st.Dir(), "backups")

	// The repository configuration decides whether the prompt hook is
	// part of the plan. Outside a repository the defaults apply, which
	// leave it out.
	cfg := config.Default()
	if repo, rerr := openRepo(ctx); rerr == nil {
		if loaded, lerr := config.Load(repo.Root); lerr == nil {
			cfg = loaded
		}
	}

	p := claudePlan{
		Plan: claude.Build(claude.Input{
			Top: file.Top, Hooks: hooks, Config: cfg, Exe: selfPath(), ReplaceStatus: replaceStatus,
		}),
		SettingsPath: path,
		Exists:       file.Exists,
		Backup:       filepath.Join(backupDir, fmt.Sprintf("claude-settings-%s.json", time.Now().UTC().Format("20060102-150405"))),
	}
	if p.KeepStatusLine != "" && !replaceStatus {
		p.Notes = append(p.Notes,
			"A status line is already configured and will be left in place. Pass --replace-status-line to replace it.")
	}
	if !userWide && !gitIgnored(ctx, path) {
		p.Notes = append(p.Notes,
			path+" is not ignored by Git. The entries hold this machine's executable path, so add it to .gitignore before committing.")
	}

	switch mode {
	case "plan":
		printClaudePlan(p, file.Keys())
		return nil
	case "remove":
		return applyClaudeRemove(file, hooks, backupDir, p)
	}
	return applyClaudeInstall(file, hooks, backupDir, p, replaceStatus)
}

func gitIgnored(ctx context.Context, path string) bool {
	repo, err := git.Open(ctx, filepath.Dir(filepath.Dir(path)))
	if err != nil {
		return false
	}
	return repo.IsIgnored(ctx, path)
}

func printClaudePlan(p claudePlan, order []string) {
	exe := selfPath()
	fmt.Println("ZEROTURN INTEGRATE CLAUDE (plan)")
	fmt.Printf("settings file   %s\n", p.SettingsPath)
	if !p.Exists {
		fmt.Println("                the file does not exist yet and would be created")
	}
	fmt.Printf("backup          %s\n", p.Backup)
	fmt.Println()

	fmt.Println("would add:")
	if p.AddStatusLine {
		fmt.Printf("  statusLine    %s\n", claude.Command(exe, ""))
	}
	for _, h := range p.AddHooks {
		suffix := ""
		if h.Matcher != "" {
			suffix = fmt.Sprintf("  (matcher %s, exact, %s)", h.Matcher, h.Purpose)
		}
		fmt.Printf("  hooks.%-14s %s%s\n", h.Event, claude.Command(exe, h.Event), suffix)
	}
	if !p.AddStatusLine && len(p.AddHooks) == 0 {
		fmt.Println("  nothing, the integration is already installed")
	}
	fmt.Println()

	if len(p.RepairHooks) > 0 {
		fmt.Println("would repair:")
		for _, r := range p.RepairHooks {
			state := "which is not this executable"
			if r.Gone {
				state = "which is no longer there, so the entry does nothing"
			}
			fmt.Printf("  %-22s points at %s, %s\n", r.Where, r.From, state)
		}
		fmt.Printf("  all of them would point at %s\n", exe)
		fmt.Println()
	}

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
	fmt.Println("  The credential guard uses PreToolUse with the exact matcher Read, and scans only that file.")
	fmt.Println("  Subagent counts come from ZeroTurn's own events, not from the harness.")
	for _, n := range p.Notes {
		fmt.Printf("  %s\n", n)
	}
	fmt.Println("\nNothing was changed. Run the same command with --apply to write it.")
}

func applyClaudeInstall(file *settings.File, hooks map[string][]json.RawMessage,
	backupDir string, p claudePlan, replaceStatus bool) error {

	if p.NothingToDo() {
		fmt.Println("The Claude integration is already installed. Nothing was changed.")
		return nil
	}
	printClaudePlan(p, file.Keys())
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
		if err := file.Backup(p.Backup); err != nil {
			return output.Errorf(output.ExitInternal, "zeroturn integrate changed nothing",
				"the backup could not be written", "check that "+backupDir+" is writable")
		}
	}

	exe := selfPath()
	claude.Install(file.Top, hooks, p.Plan, exe, replaceStatus)
	if len(hooks) > 0 {
		b, _ := json.Marshal(hooks)
		file.Set("hooks", json.RawMessage(b))
	}
	if p.AddStatusLine || replaceStatus {
		file.Set("statusLine", claude.StatusLineValue(exe))
	}

	if err := file.Write(); err != nil {
		return output.Errorf(output.ExitInternal, "zeroturn integrate could not write the settings file",
			err.Error(), "the backup at "+p.Backup+" holds the previous content")
	}
	if len(p.RepairHooks) > 0 {
		fmt.Printf("Repointed %d entries at %s\n", len(p.RepairHooks), exe)
	}
	fmt.Printf("Wrote %s\n", file.Path)
	if p.Exists {
		fmt.Printf("Backup %s\n", p.Backup)
	}
	fmt.Println("Start a new coding session for the change to take effect.")
	return nil
}

func applyClaudeRemove(file *settings.File, hooks map[string][]json.RawMessage,
	backupDir string, p claudePlan) error {

	if !p.Exists {
		fmt.Printf("No settings file at %s. Nothing to remove.\n", file.Path)
		return nil
	}

	removed, removedStatus := claude.Remove(file.Top, hooks)
	if removed == 0 {
		fmt.Println("No ZeroTurn entries were found. Nothing was changed.")
		return nil
	}

	fmt.Println("ZEROTURN INTEGRATE CLAUDE (remove)")
	fmt.Printf("settings file   %s\n", file.Path)
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

	if err := file.Backup(p.Backup); err != nil {
		return output.Errorf(output.ExitInternal, "zeroturn integrate changed nothing",
			"the backup could not be written", "check that "+backupDir+" is writable")
	}
	if removedStatus {
		file.Delete("statusLine")
	}
	if len(hooks) == 0 {
		file.Delete("hooks")
	} else {
		b, _ := json.Marshal(hooks)
		file.Set("hooks", json.RawMessage(b))
	}
	if err := file.Write(); err != nil {
		return output.Errorf(output.ExitInternal, "zeroturn integrate could not write the settings file",
			err.Error(), "the backup at "+p.Backup+" holds the previous content")
	}
	fmt.Printf("Removed %d ZeroTurn entries from %s\n", removed, file.Path)
	fmt.Printf("Backup %s\n", p.Backup)
	return nil
}
