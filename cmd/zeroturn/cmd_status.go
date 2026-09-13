package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/events"
	"github.com/cris-wendler/zeroturn/internal/output"
	"github.com/cris-wendler/zeroturn/internal/policy"
	"github.com/cris-wendler/zeroturn/internal/state"
	"github.com/cris-wendler/zeroturn/internal/status"
)

type statusJSON struct {
	Available bool           `json:"sessionDataAvailable"`
	Session   *state.Session `json:"session,omitempty"`
	Policy    policy.Result  `json:"policy"`
	Branch    string         `json:"branch,omitempty"`
	Note      string         `json:"note,omitempty"`
}

const statusUsage = `zeroturn status [--json] [--stdin --harness <name>]

Show the condition of the current session: context use, the usage
windows, how long it has run, and how many subagents are active.

With --stdin it reads one harness status payload and prints a status
line, which is how a harness draws it on every repaint.
`

func cmdStatus(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	stdin := fs.Bool("stdin", false, "read a harness status payload from standard input")
	harness := fs.String("harness", "claude", "harness sending the payload when --stdin is used")
	asJSON := fs.Bool("json", false, "print machine readable output")
	if err := parseFlags(fs, args, statusUsage, "status"); err != nil {
		return err
	}

	if *stdin {
		return statusFromStdin(ctx, *harness)
	}
	return statusFromRepo(ctx, *asJSON)
}

// statusFromStdin is the status line path. It always exits successfully so
// that a ZeroTurn problem cannot break the harness display.
func statusFromStdin(ctx context.Context, harness string) error {
	var e events.Event
	var err error
	if harness == "claude" {
		e, err = events.ParseClaude(os.Stdin, "statusline")
	} else {
		e, err = events.ParseNormalized(os.Stdin)
	}
	if err != nil {
		return nil
	}
	st, serr := state.Open()
	if serr != nil {
		return nil
	}
	sess, uerr := applyEvent(st, e)
	if uerr != nil {
		return nil
	}

	res := policy.Evaluate(sessionConfig(st, e.CWD), sess)
	fmt.Println(status.Render(status.Line{
		Session: sess,
		Result:  res,
		Color:   output.NewColor(os.Stdout, harness),
	}))
	return nil
}

func statusFromRepo(ctx context.Context, asJSON bool) error {
	repo, err := openRepo(ctx)
	if err != nil {
		return err
	}
	cfg := config.Default()
	if config.Exists(repo.Root) {
		if loaded, lerr := config.Load(repo.Root); lerr == nil {
			cfg = loaded
		}
	}
	st, err := openStore()
	if err != nil {
		return err
	}

	branch, _ := repo.Branch(ctx)
	sess, found := recentSession(st, repo.Root)
	res := policy.Evaluate(effectiveGuard(st, repo.Root, cfg), sess)

	if asJSON {
		out := statusJSON{Available: found, Policy: res, Branch: branch}
		if found {
			s := sess
			out.Session = &s
		} else {
			out.Note = "No recent ZeroTurn session record for this repository. Live session information is only available while a supported coding session is running."
		}
		return output.JSON(os.Stdout, out)
	}

	c := output.NewColor(os.Stdout, "")
	fmt.Println(status.Render(status.Line{
		Session: sess, Result: res, Branch: branch, Color: c, ShowRepo: true,
	}))
	if !found {
		fmt.Println(c.Dim("Live session information is unavailable outside a supported coding session."))
	}
	return nil
}

// recentSession returns the newest record for this repository, within a
// window short enough that a stale record is not shown as current.
func recentSession(st *state.Store, repoRoot string) (state.Session, bool) {
	sessions, err := st.Since(12 * time.Hour)
	if err != nil {
		return state.Session{}, false
	}
	h := state.RepoHash(repoRoot)
	for _, s := range sessions {
		if s.RepoHash == h {
			return s, true
		}
	}
	return state.Session{}, false
}
