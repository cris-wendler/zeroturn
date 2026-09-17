package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/evidence"
	"github.com/cris-wendler/zeroturn/internal/output"
	"github.com/cris-wendler/zeroturn/internal/snapshot"
	"github.com/cris-wendler/zeroturn/internal/state"
	"github.com/cris-wendler/zeroturn/internal/trust"
	"github.com/cris-wendler/zeroturn/internal/verify"
)

// verifyJSON is the result with the evidence record beside it. The result
// is embedded rather than nested, so every field a reader already takes
// from this output is where it was.
type verifyJSON struct {
	verify.Result
	Evidence *evidence.Record `json:"evidence,omitempty"`
}

const verifyUsage = `zeroturn verify [--json] [--approve]

Run the validation steps this repository declares, directly, in order,
stopping at the first failure.

The commands are approved once per repository on this machine. Nothing
runs until they have been reviewed.

The result is recorded as evidence, together with the state of the
repository it ran against, so that zeroturn report can say later whether
it still covers the code that is there.
`

func cmdVerify(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("verify", flag.ContinueOnError)
	asJSON := fs.Bool("json", false, "print machine readable output")
	approve := fs.Bool("approve", false, "review and approve the configured commands for this repository")
	if err := parseFlags(fs, args, verifyUsage, "verify"); err != nil {
		return err
	}

	repo, err := openRepo(ctx)
	if err != nil {
		return err
	}
	c, err := loadConfig(repo.Root)
	if err != nil {
		return err
	}
	if len(c.Verify.Steps) == 0 {
		return output.Errorf(output.ExitInvalidUsage,
			"zeroturn verify ran nothing",
			"no validation steps are configured",
			"add steps to verify.steps in "+config.FileName+", or run zeroturn init again")
	}
	st, err := openStore()
	if err != nil {
		return err
	}

	status := trust.Check(st, repo.Root, c)
	if !status.Trusted {
		if !*approve {
			printCommands(c)
			return output.Errorf(output.ExitDeclined,
				"zeroturn verify ran nothing",
				status.Reason,
				"review the commands above, then run zeroturn verify --approve")
		}
		printCommands(c)
		ok, cerr := confirm("Execute these commands directly when zeroturn verify runs?")
		if cerr != nil {
			return cerr
		}
		if !ok {
			return output.Errorf(output.ExitDeclined,
				"zeroturn verify ran nothing",
				"approval was declined",
				"run zeroturn verify --approve again when you are ready")
		}
		if aerr := trust.Approve(st, repo.Root, c); aerr != nil {
			return output.Errorf(output.ExitInternal, "zeroturn could not record the approval",
				aerr.Error(), "check that your user data directory is writable")
		}
		fmt.Println("Approved. This approval is withdrawn automatically if the configured commands change.")
	}

	if missing := verify.MissingExecutables(c); len(missing) > 0 {
		return output.Errorf(output.ExitMissingExec,
			"zeroturn verify ran nothing",
			"these executables were not found on PATH: "+strings.Join(missing, ", "),
			"install them, or change the commands in "+config.FileName)
	}

	// The snapshot is taken before the steps run. Taking it afterwards
	// would fold whatever the steps wrote, a coverage file or a build
	// directory that is not ignored, into the state the result claims to
	// be evidence about.
	snap, serr := snapshot.Compute(ctx, repo, c.Hash())
	if serr != nil {
		return output.Errorf(output.ExitInternal, "zeroturn verify ran nothing",
			"the repository state could not be read: "+serr.Error(),
			"check that git is installed and that this repository is readable")
	}
	repoHash := state.RepoHash(repo.Root)
	rec, berr := evidence.Begin(st, repoHash, Version, snap, time.Now())
	if berr != nil {
		return output.Errorf(output.ExitInternal, "zeroturn verify ran nothing",
			"the evidence record could not be written: "+berr.Error(),
			"check that your user data directory is writable")
	}

	opts := verify.Options{RepoRoot: repo.Root}
	if !*asJSON {
		fmt.Println("ZEROTURN VERIFY")
		fmt.Println()
		opts.OnStep = printStep
	}
	res, rerr := verify.Run(ctx, c, opts)
	if rerr != nil {
		return output.Errorf(output.ExitInternal, "zeroturn verify could not start",
			rerr.Error(), "check that the repository .git directory is writable")
	}

	rec, cerr := evidence.Complete(st, rec, res, time.Now())
	if cerr != nil {
		// The run happened and its result is on the screen. Losing the
		// record is worth saying out loud, because the next command will
		// report that no validation was recorded, and that would otherwise
		// look like this run never took place.
		fmt.Fprintln(os.Stderr, "ZeroTurn could not record evidence for this run: "+cerr.Error())
	}

	if sess, found := recentSession(st, repo.Root); found {
		st.Update(sess.SessionID, "", func(s *state.Session) { s.DirectValidations++ })
	}

	if *asJSON {
		if jerr := output.JSON(os.Stdout, verifyJSON{Result: res, Evidence: &rec}); jerr != nil {
			return jerr
		}
	} else {
		printVerifySummary(res)
		fmt.Printf("Evidence %s recorded for repository state %s\n", rec.EvidenceID, snap.Short())
	}

	if res.Cancelled {
		return output.Errorf(output.ExitDeclined, "zeroturn verify stopped",
			"the run was cancelled", "run zeroturn verify again when ready")
	}
	if res.Failed > 0 {
		return output.Errorf(output.ExitPolicyFailure, "zeroturn verify failed",
			output.CountedPair(res.Failed, len(res.Steps), "%d of %d %s did not pass", "step", "steps"),
			"fix the failure above, then run zeroturn verify again")
	}
	return nil
}

func printCommands(c config.Config) {
	fmt.Println("These commands are configured by this repository and have not been approved on this machine:")
	for _, s := range c.Verify.Steps {
		fmt.Printf("  %-10s", s.Name)
		for i, a := range s.Command {
			if i > 0 {
				fmt.Print(" ")
			}
			fmt.Printf("%q", a)
		}
		fmt.Println()
	}
	fmt.Println("They run directly, without a shell.")
}

// printStep writes one result line. It is passed to verify.Run so each
// line appears when its step finishes.
func printStep(s verify.StepResult) {
	c := output.NewColor(os.Stdout, "")
	switch s.Status {
	case verify.StatusPass:
		fmt.Printf("%s  %-10s %.1fs\n", c.Green("PASS"), s.Name, s.Seconds)
	case verify.StatusFail:
		fmt.Printf("%s  %-10s %.1fs  exit %d\n", c.Red("FAIL"), s.Name, s.Seconds, s.ExitCode)
	case verify.StatusMissing:
		fmt.Printf("%s  %-10s %s\n", c.Red("MISS"), s.Name, s.Excerpt)
	case verify.StatusCancelled:
		fmt.Printf("%s  %-10s\n", c.Yellow("STOP"), s.Name)
	default:
		fmt.Printf("%s  %-10s\n", c.Dim("SKIP"), s.Name)
	}
}

// printVerifySummary follows the streamed step lines with the failure
// excerpt, the log location, and the result line.
func printVerifySummary(res verify.Result) {
	for _, s := range res.Steps {
		if s.Status == verify.StatusFail && s.Excerpt != "" {
			fmt.Printf("\nLast output from %s:\n", s.Name)
			for _, line := range strings.Split(s.Excerpt, "\n") {
				fmt.Printf("  %s\n", line)
			}
			if s.LogFile != "" {
				fmt.Printf("Full output: %s\n", s.LogFile)
			}
		}
	}
	fmt.Println()
	if res.Failed == 0 && res.Skipped == 0 {
		fmt.Printf("Result: %s in %.1fs\n",
			output.Counted(res.Passed, "1 check passed", "%d checks passed"), res.Seconds)
		return
	}
	fmt.Printf("Result: %d passed, %d failed, %d skipped in %.1fs\n",
		res.Passed, res.Failed, res.Skipped, res.Seconds)
}
