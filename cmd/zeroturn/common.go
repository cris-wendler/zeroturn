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

// sessionConfig loads the policy for a harness event from its working
// directory. It starts no git process, because it runs on every status
// line repaint, and it falls back to the defaults on any problem so that a
// broken file cannot break the harness.
func sessionConfig(st *state.Store, dir string) config.Config {
	root, ok := git.FindRoot(dir)
	if !ok {
		return config.Default()
	}
	cfg := config.Default()
	if config.Exists(root) {
		if loaded, err := config.Load(root); err == nil {
			cfg = loaded
		}
	}
	return effectiveGuard(st, root, cfg)
}

// effectiveGuard applies the local Strict approval. A repository file can
// ask for Strict, but only the person on this machine can enable it, so
// until they do the gate asks instead of denying.
func effectiveGuard(st *state.Store, root string, c config.Config) config.Config {
	if c.Guard.Mode == config.ModeStrict && !trust.StrictApproved(st, root) {
		c.Guard.Mode = config.ModeConfirm
	}
	return c
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

// confirm asks a yes or no question. It refuses rather than assumes when
// no terminal is attached, so an unattended run can never self approve.
// It is a variable only so in process tests can answer it; no flag or
// environment value can replace it.
var confirm = func(question string) (bool, error) {
	if !isTTY(os.Stdin) {
		return false, output.Errorf(output.ExitDeclined,
			"zeroturn stopped before making any change",
			"confirmation is required and no terminal is attached",
			"run the command from a terminal, or use --dry-run to inspect it")
	}
	fmt.Printf("%s [y/N]: ", question)
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil {
		return false, nil
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
