package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/security"

	"github.com/cris-wendler/zeroturn/internal/events"
	"github.com/cris-wendler/zeroturn/internal/git"
	"github.com/cris-wendler/zeroturn/internal/policy"
	"github.com/cris-wendler/zeroturn/internal/session"
	"github.com/cris-wendler/zeroturn/internal/state"
)

// maxScan bounds the work done while a developer waits. Credentials live
// in small files: an environment file, a key, a configuration file. A
// file above this size is almost always data or a log, and scanning one
// would hold up the read for close to a second in the worst case. The
// limit is stated in the documentation rather than hidden, because it is
// a limit of the check.
const maxScan = 1 << 20

// scanTarget reads the file the credential guard is about to scan, and
// reports whether it is one worth scanning at all.
//
// The check is made against the open file rather than against the path.
// A path can name a different file by the time it is opened, and what
// gets scanned then is not what was checked, so the size and the kind
// are read back from the handle the content comes from. The read is
// bounded as well, whatever the size claimed, because a size is a
// statement about the past.
//
// The cheap check by path stays in front of it. Only an ordinary file is
// opened, because a device such as /dev/zero reports a size of zero and
// then never reaches the end, and a pipe blocks on being opened at all,
// either of which would hang the read a developer is waiting for.
func scanTarget(path string) ([]byte, bool) {
	if info, err := os.Stat(path); err != nil || !info.Mode().IsRegular() || info.Size() > maxScan {
		return nil, false
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, false
	}
	defer f.Close()
	opened, err := f.Stat()
	if err != nil || !opened.Mode().IsRegular() || opened.Size() > maxScan {
		return nil, false
	}
	content, err := ioutil.ReadAll(io.LimitReader(f, maxScan))
	if err != nil {
		return nil, false
	}
	return content, true
}

// credentialGate scans the file a read tool is about to open. It reads
// the file, never the prompt, and reports the file, the line, and the
// category, never the value.
func credentialGate(st *state.Store, e events.Event) error {
	if e.FilePath == "" {
		return nil
	}
	cfg := sessionGuard(st, e.CWD).Config()
	if cfg.Guard.Credentials.Mode == config.CredentialOff {
		return nil
	}
	content, ok := scanTarget(e.FilePath)
	if !ok {
		return nil
	}
	name := filepath.Base(e.FilePath)
	if root, ok := git.FindRoot(filepath.Dir(e.FilePath)); ok {
		if rel, rerr := filepath.Rel(root, e.FilePath); rerr == nil && !strings.HasPrefix(rel, "..") {
			name = filepath.ToSlash(rel)
		}
	}
	findings, serr := security.ScanBytes(name, content)
	if serr != nil || len(findings) == 0 {
		return nil
	}

	decision, reason := policy.CredentialDecision(cfg.Guard.Credentials.Mode, name, findings[0].Line, security.Categories(findings))
	if decision == policy.DecisionAllow {
		return nil
	}
	st.Update(e.SessionID, session.RepoHashFor(e.CWD), func(s *state.Session) { s.CredentialWarnings++ })
	return printDecision(decision, reason)
}

// promptGate scans the message a developer is about to send. The text
// is held in memory for the length of the scan and is never stored,
// logged, or included in the reason. The guard is off unless the
// developer switched it on.
func promptGate(st *state.Store, e events.Event) error {
	if e.Prompt == "" {
		return nil
	}
	cfg := sessionGuard(st, e.CWD).Config()
	findings, err := security.ScanBytes("message", []byte(e.Prompt))
	if err != nil || len(findings) == 0 {
		return nil
	}
	block, reason := policy.PromptCredentialDecision(cfg.Guard.Credentials.Prompts, security.Categories(findings))
	if !block {
		return nil
	}
	st.Update(e.SessionID, session.RepoHashFor(e.CWD), func(s *state.Session) { s.CredentialWarnings++ })

	b, merr := json.Marshal(promptDecision{Decision: "block", Reason: reason})
	if merr != nil {
		return nil
	}
	fmt.Println(string(b))
	return nil
}

// promptDecision is the UserPromptSubmit response shape. The harness
// offers no way to ask about a message, so the only answers are block
// and silence.
type promptDecision struct {
	Decision string `json:"decision"`
	Reason   string `json:"reason"`
}

func printDecision(decision, reason string) error {
	var out decisionOutput
	out.HookSpecificOutput.HookEventName = "PreToolUse"
	out.HookSpecificOutput.PermissionDecision = decision
	out.HookSpecificOutput.PermissionDecisionReason = reason
	b, err := json.Marshal(out)
	if err != nil {
		return nil
	}
	fmt.Println(string(b))
	return nil
}

// decisionOutput is the Claude Code permission response shape.
type decisionOutput struct {
	HookSpecificOutput struct {
		HookEventName            string `json:"hookEventName"`
		PermissionDecision       string `json:"permissionDecision"`
		PermissionDecisionReason string `json:"permissionDecisionReason"`
	} `json:"hookSpecificOutput"`
}

// cmdEvent is the adapter entry point. It fails open: any problem leaves
// the harness to its normal behaviour rather than blocking a developer
// because ZeroTurn had trouble.
const eventUsage = `zeroturn event --harness <name> --event <name>

The entry point a harness hook calls. It reads one event on standard
input and writes a decision on standard output when there is one.

It exits 0 whatever happens, because a guard that fails must never stop
a session. A malformed event, an unreadable state directory, or a broken
configuration all end in silence.
`

func cmdEvent(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("event", flag.ContinueOnError)
	fs.SetOutput(ioutil.Discard)
	fs.Usage = func() {}
	fs.SetOutput(os.Stderr)
	harness := fs.String("harness", "claude", "harness sending the event: claude, or normalized for other adapters")
	name := fs.String("event", "", "harness event name, for example PreToolUse")
	if err := fs.Parse(args); err != nil {
		// A person running this by hand deserves an answer. Every other
		// failure is silent and exits 0, because a gate that fails must
		// not stop a session.
		if errors.Is(err, flag.ErrHelp) {
			fmt.Print(eventUsage)
			fs.SetOutput(os.Stdout)
			fmt.Println("Flags:")
			fs.PrintDefaults()
		}
		return nil
	}

	var e events.Event
	var err error
	if *harness == "claude" {
		e, err = events.ParseClaude(os.Stdin, *name)
	} else {
		e, err = events.ParseNormalized(os.Stdin)
	}
	if err != nil {
		if _, bad := err.(events.ContractError); bad {
			fmt.Fprintf(os.Stderr, "zeroturn ignored an event\n  reason: %v\n", err)
		}
		return nil
	}

	st, serr := state.Open()
	if serr != nil {
		return nil
	}

	if e.Type == events.TypeFileRead {
		return credentialGate(st, e)
	}
	if e.Type == events.TypePromptSubmit {
		return promptGate(st, e)
	}

	if e.Type != events.TypeSubagentPre {
		if _, uerr := session.Record(st, e); uerr != nil {
			return nil
		}
		return nil
	}

	// The gate folds the event and the decision into one state write,
	// because it runs while the developer waits for the subagent.
	guard := sessionGuard(st, e.CWD)
	var res policy.Result
	if _, uerr := st.Update(e.SessionID, session.RepoHashFor(e.CWD), func(s *state.Session) {
		session.Apply(s, e)
		res = policy.Evaluate(guard, *s)
		s.LastDecision = res.Decision
		if res.Decision != policy.DecisionAllow {
			names := make([]string, 0, len(res.Triggers))
			for _, t := range res.Triggers {
				names = append(names, t.Name)
			}
			s.RecordGate(res.Decision, names)
		}
		switch res.Decision {
		case policy.DecisionAsk:
			s.ConfirmRequests++
		case policy.DecisionDeny:
			s.DeniedStarts++
		default:
			s.AllowedStarts++
		}
	}); uerr != nil {
		return nil
	}

	// An allow decision prints nothing so that ZeroTurn does not override
	// the permission rules the user already configured in the harness.
	if res.Decision == policy.DecisionAllow {
		return nil
	}

	return printDecision(res.Decision, res.Reason)
}
