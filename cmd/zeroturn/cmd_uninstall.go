package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/git"
	"github.com/cris-wendler/zeroturn/internal/output"
	"github.com/cris-wendler/zeroturn/internal/state"
	"github.com/cris-wendler/zeroturn/internal/uninstall"
)

const uninstallUsage = `zeroturn uninstall [--apply]

Lists everything ZeroTurn has put on this machine and, with --apply,
takes it away.

  --apply   Remove it, after showing the list and asking

Without --apply nothing changes. The zeroturn executable is never
deleted, because removing a program you installed is yours to do; the
list says where it is.
`

func cmdUninstall(args []string) error {
	apply := false
	for _, a := range args {
		switch a {
		case "--apply":
			apply = true
		case "--help", "-h", "help":
			fmt.Print(uninstallUsage)
			return errHelp
		default:
			return output.Errorf(output.ExitInvalidUsage, "zeroturn uninstall changed nothing",
				"flag "+a+" is not one zeroturn uninstall accepts", "run zeroturn uninstall --help")
		}
	}

	sources, root := uninstallSources()
	plan, err := uninstall.Survey(sources)
	if err != nil {
		return output.Errorf(output.ExitInvalidUsage, "zeroturn uninstall changed nothing",
			err.Error(), "correct the file, then run zeroturn uninstall again")
	}

	if len(plan.Items) == 0 {
		fmt.Println("ZeroTurn has nothing on this machine to remove.")
		printExecutable()
		return nil
	}

	printUninstallPlan(plan, root)
	if !apply {
		fmt.Println()
		fmt.Println("Nothing has changed. Run zeroturn uninstall --apply to remove it.")
		return nil
	}

	fmt.Println()
	ok, err := confirm("Remove all of it?")
	if err != nil {
		return err
	}
	if !ok {
		return output.Errorf(output.ExitDeclined, "zeroturn uninstall changed nothing",
			"the removal was declined", "run the command again when you are ready")
	}

	// The backup goes to a temporary directory rather than into ZeroTurn's
	// own state, which this command is about to delete.
	backupDir, berr := ioutil.TempDir("", "zeroturn-uninstall-")
	if berr != nil {
		return output.Errorf(output.ExitInternal, "zeroturn uninstall changed nothing",
			"a directory for the settings backup could not be created", berr.Error())
	}

	result := uninstall.Apply(plan, backupDir)
	fmt.Println()
	for _, it := range result.Done {
		if it.Action == uninstall.ActionEdit {
			fmt.Printf("Removed %s from %s\n", entryCount(it.Entries), it.Path)
			continue
		}
		fmt.Printf("Deleted %s\n", it.Path)
	}
	if plan.Entries() > 0 {
		fmt.Printf("Backup %s\n", backupDir)
	}
	if len(result.Failed) > 0 {
		for _, f := range result.Failed {
			fmt.Fprintf(os.Stderr, "Left in place: %s (%s)\n", f.Item.Path, f.Reason)
		}
		return output.Errorf(output.ExitInternal, "zeroturn uninstall did not remove everything",
			fmt.Sprintf("%d of %d items could not be removed", len(result.Failed), len(plan.Items)),
			"check the permissions on the paths listed above, then run zeroturn uninstall --apply again")
	}
	printExecutable()
	fmt.Println("Start a new coding session for the change to take effect.")
	return nil
}

// uninstallSources asks each package where it writes. Outside a
// repository there is no policy file and no per repository settings file,
// and the user wide settings file is still worth looking at.
func uninstallSources() (uninstall.Sources, string) {
	var s uninstall.Sources

	if dir, err := state.DataDir(); err == nil {
		s.StateDir = dir
	}
	if home, err := os.UserHomeDir(); err == nil {
		s.SettingsFiles = append(s.SettingsFiles, filepath.Join(home, ".claude", "settings.json"))
	}

	wd, err := os.Getwd()
	if err != nil {
		return s, ""
	}
	// FindRoot walks the directories itself, so uninstall works on a
	// machine where git is not installed or is already gone.
	root, ok := git.FindRoot(wd)
	if !ok {
		return s, ""
	}
	s.SettingsFiles = append(s.SettingsFiles,
		filepath.Join(root, ".claude", "settings.local.json"))
	s.ConfigFiles = append(s.ConfigFiles, config.Path(root))
	return s, root
}

func printUninstallPlan(p uninstall.Plan, root string) {
	fmt.Println("ZEROTURN UNINSTALL")
	fmt.Println()
	for _, it := range p.Items {
		fmt.Printf("  %-7s %s\n", it.Action, it.Path)
		fmt.Printf("          %s\n", itemDetail(it))
	}
	fmt.Println()
	printExecutable()
	if root != "" {
		fmt.Println()
		fmt.Println("Policy files in other repositories are not found by this command.")
		fmt.Println("Each one is a " + config.FileName + " you can delete.")
	}
}

// itemDetail says what will happen to one item in one line. A settings
// file says what stays as well as what goes, because the promise that the
// rest of the file survives is the reason a person can answer yes.
func itemDetail(it uninstall.Item) string {
	if it.Action != uninstall.ActionEdit {
		if it.Detail != "" {
			return it.Detail
		}
		return it.What
	}
	if it.Kept == 0 {
		return fmt.Sprintf("%s, and no entries written by you or another tool", entryCount(it.Entries))
	}
	kept := "entries"
	if it.Kept == 1 {
		kept = "entry"
	}
	return fmt.Sprintf("%s, keeping %d %s written by you or another tool",
		entryCount(it.Entries), it.Kept, kept)
}

func entryCount(n int) string {
	if n == 1 {
		return "1 ZeroTurn entry"
	}
	return fmt.Sprintf("%d ZeroTurn entries", n)
}

// printExecutable says where the program is and how to remove it. ZeroTurn
// does not delete its own binary: a running executable cannot be deleted on
// Windows, and a binary placed by a package manager belongs to that manager.
func printExecutable() {
	exe := selfPath()
	fmt.Printf("The zeroturn executable stays at %s\n", exe)
	if runtime.GOOS == "windows" {
		fmt.Printf("Delete it yourself once this command has finished.\n")
		return
	}
	fmt.Printf("Remove it with: rm %s\n", exe)
}
