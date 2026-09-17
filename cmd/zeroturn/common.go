package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/git"
	"github.com/cris-wendler/zeroturn/internal/output"
	"github.com/cris-wendler/zeroturn/internal/policy"
	"github.com/cris-wendler/zeroturn/internal/state"
	"github.com/cris-wendler/zeroturn/internal/trust"
)

func openRepo(ctx context.Context) (git.Repo, error) {
	wd, err := os.Getwd()
	if err != nil {
		return git.Repo{}, output.Errorf(output.ExitInternal,
			"zeroturn could not read the working directory", err.Error(),
			"run the command again from a directory you can read")
	}
	r, err := git.Open(ctx, wd)
	if err == git.ErrNotRepo {
		return git.Repo{}, output.Errorf(output.ExitInvalidUsage,
			"zeroturn stopped before doing anything",
			"this directory is not inside a Git repository",
			"change to a repository, or run git init first")
	}
	if err != nil {
		return git.Repo{}, output.Errorf(output.ExitMissingExec,
			"zeroturn stopped before doing anything", err.Error(),
			"install git and make sure it is on PATH")
	}
	return r, nil
}

func loadConfig(root string) (config.Config, error) {
	if !config.Exists(root) {
		return config.Config{}, output.Errorf(output.ExitInvalidUsage,
			"zeroturn stopped before doing anything",
			"this repository has no "+config.FileName,
			"run zeroturn init to write one")
	}
	c, err := config.Load(root)
	if err != nil {
		// A file from a newer ZeroTurn is not a broken file. Telling
		// somebody to run init here would answer it by overwriting the
		// policy they wrote, which is the one thing they cannot undo.
		var newer config.ErrNewer
		if errors.As(err, &newer) {
			return config.Config{}, output.Errorf(output.ExitContractVersion,
				"zeroturn stopped before doing anything", newer.Error(),
				"upgrade ZeroTurn, which can read the newer file")
		}
		if ve, ok := err.(config.ValidationError); ok {
			return config.Config{}, output.Errorf(output.ExitInvalidUsage,
				"zeroturn stopped before doing anything",
				ve.Field+" "+ve.Detail, ve.Fix)
		}
		return config.Config{}, output.Errorf(output.ExitInvalidUsage,
			"zeroturn stopped before doing anything", err.Error(),
			"correct "+config.FileName+" or run zeroturn init to rewrite it")
	}
	return c, nil
}

// sessionGuard loads the policy for a harness event from its working
// directory. It starts no git process, because it runs on every status
// line repaint, and it falls back to the defaults on any problem so that a
// broken file cannot break the harness.
func sessionGuard(st *state.Store, dir string) policy.Guard {
	root, ok := git.FindRoot(dir)
	if !ok {
		return policy.NewGuard(config.Default(), false)
	}
	cfg := config.Default()
	if config.Exists(root) {
		if loaded, err := config.Load(root); err == nil {
			cfg = loaded
		}
	}
	return guardFor(st, root, cfg)
}

// guardFor answers the one question policy.NewGuard asks: has the person
// at this machine approved Strict mode for this repository.
func guardFor(st *state.Store, root string, c config.Config) policy.Guard {
	return policy.NewGuard(c, trust.StrictApproved(st, root))
}

func openStore() (*state.Store, error) {
	st, err := state.Open()
	if err != nil {
		return nil, output.Errorf(output.ExitInternal,
			"zeroturn could not open its local state", err.Error(),
			"check that your user data directory is writable")
	}
	return st, nil
}

func isTTY(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}

// unanswered reports that a confirmation was required and nothing
// answered it. It is not a refusal: saying that a person declined states
// a decision nobody made, and the advice to run the command again would
// send the reader back to the same silent input.
func unanswered() error {
	return output.Errorf(output.ExitDeclined,
		"zeroturn stopped before making any change",
		"confirmation is required and nothing answered it",
		"run the command from a terminal")
}

// confirm asks a yes or no question. It refuses rather than assumes when
// nothing can answer, so an unattended run can never self approve.
// It is a variable only so in process tests can answer it; no flag or
// environment value can replace it.
var confirm = func(question string) (bool, error) {
	if !isTTY(os.Stdin) {
		return false, unanswered()
	}
	fmt.Printf("%s [y/N]: ", question)
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	// Input can pass the check above and still answer nothing. /dev/null
	// is a character device, and it is the standard input of a scheduled
	// job, a container started without an interactive flag, and a
	// continuous integration step. Reading it ends at once, and reading a
	// terminal ends the same way when somebody presses ctrl-D. A read
	// that ended after delivering something is a real answer, however it
	// ended, so only an empty one is unanswered.
	if err != nil && line == "" {
		return false, unanswered()
	}
	line = strings.ToLower(strings.TrimSpace(line))
	return line == "y" || line == "yes", nil
}

// errHelp reports that help was asked for, which is not a failure. The
// flag package returns flag.ErrHelp from Parse for -h and --help, and
// treating that as a parse error made the top level instruction to run
// zeroturn <command> --help print an error and exit with the code a
// harness reads as a blocking one.
var errHelp = errors.New("help requested")

// parseFlags reads the flags for one command. Asking for help prints the
// command's own usage and the flags it accepts, and stops.
func parseFlags(fs *flag.FlagSet, args []string, usage, command string) error {
	// The flag package prints its own usage before returning the error.
	// This prints the project's instead.
	fs.SetOutput(ioutil.Discard)
	fs.Usage = func() {}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			fmt.Print(usage)
			fs.SetOutput(os.Stdout)
			fmt.Println("Flags:")
			fs.PrintDefaults()
			return errHelp
		}
		return output.Errorf(output.ExitInvalidUsage, "zeroturn could not read the flags",
			err.Error(), "run zeroturn "+command+" --help")
	}
	return nil
}
