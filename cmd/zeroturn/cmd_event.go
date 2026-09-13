package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/security"

	"github.com/cris-wendler/zeroturn/internal/events"
	"github.com/cris-wendler/zeroturn/internal/git"
	"github.com/cris-wendler/zeroturn/internal/policy"
	"github.com/cris-wendler/zeroturn/internal/state"
)

func repoHashFor(cwd string) string {
	if root, ok := git.FindRoot(cwd); ok {
		return state.RepoHash(root)
	}
	return ""
}

// applyEvent folds one normalized event into the stored session record.
func applyEvent(st *state.Store, e events.Event) (state.Session, error) {
	return st.Update(e.SessionID, repoHashFor(e.CWD), func(s *state.Session) {
		applyTo(s, e)
	})
}

func applyTo(s *state.Session, e events.Event) {
	{
		if e.Harness != "" {
			s.Harness = e.Harness
		}
		if e.HarnessVersion != "" {
			s.HarnessVer = e.HarnessVersion
		}
		if e.Model != "" {
			s.Model = e.Model
		}
		if e.ContextPct != nil {
			v := *e.ContextPct
			s.ContextPct = &v
		}
		if e.ContextSize != nil {
			v := *e.ContextSize
			s.ContextSize = &v
		}
		if e.FiveHourPct != nil {
			v := *e.FiveHourPct
			s.FiveHourPct = &v
		}
		if e.FiveHourResetsAt != nil {
			v := *e.FiveHourResetsAt
			s.FiveHourResetsAt = &v
		}
		if e.SevenDayPct != nil {
			v := *e.SevenDayPct
			s.SevenDayPct = &v
		}
		if e.SevenDayResetsAt != nil {
			v := *e.SevenDayResetsAt
			s.SevenDayResetsAt = &v
		}
		if e.DurationMS != nil {
			v := *e.DurationMS
			s.DurationMS = &v
		}
		if e.BackgroundTasks != nil {
			s.BackgroundTasks = *e.BackgroundTasks
		}

		switch e.Type {
		case events.TypeSubagentStrt:
			// A subagent starting after an ask is the developer having
			// approved it. It is the only outcome the harness reports.
			s.ResolveGate()
			s.AddActive(e.AgentID)
		case events.TypeSubagentStop:
			s.RemoveActive(e.AgentID)
		case events.TypeSessionStop, events.TypeSessionEnd:
			// A subagent still counted as running when a turn ends never
			// reported stopping. Clearing here keeps a later count honest
			// rather than asking about subagents that are long gone.
			s.ClearActive()
		}
	}
}

// maxScan bounds the work done while a developer waits. Credentials live
// in small files: an environment file, a key, a configuration file. A
// file above this size is almost always data or a log, and scanning one
// would hold up the read for close to a second in the worst case. The
// limit is stated in the documentation rather than hidden, because it is
// a limit of the check.
const maxScan = 1 << 20

// credentialGate scans the file a read tool is about to open. It reads
// the file, never the prompt, and reports the file, the line, and the
// category, never the value.
func credentialGate(st *state.Store, e events.Event) error {
	if e.FilePath == "" {
		return nil
	}
	cfg := sessionConfig(st, e.CWD)
	if cfg.Guard.Credentials.Mode == config.CredentialOff {
		return nil
	}
	info, err := os.Stat(e.FilePath)
	// Only an ordinary file is scanned. A directory has nothing to read,
	// and a device such as /dev/zero reports a size of zero and then
	// never reaches the end, which would hang the read the developer is
	// waiting for.
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxScan {
		return nil
	}
	content, err := ioutil.ReadFile(e.FilePath)
	if err != nil {
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
	st.Update(e.SessionID, repoHashFor(e.CWD), func(s *state.Session) { s.CredentialWarnings++ })
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
	cfg := sessionConfig(st, e.CWD)
	findings, err := security.ScanBytes("message", []byte(e.Prompt))
	if err != nil || len(findings) == 0 {
		return nil
	}
	block, reason := policy.PromptCredentialDecision(cfg.Guard.Credentials.Prompts, security.Categories(findings))
	if !block {
		return nil
	}
	st.Update(e.SessionID, repoHashFor(e.CWD), func(s *state.Session) { s.CredentialWarnings++ })

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
func cmdEvent(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("event", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	harness := fs.String("harness", "claude", "harness sending the event: claude, or normalized for other adapters")
	name := fs.String("event", "", "harness event name, for example PreToolUse")
	if err := fs.Parse(args); err != nil {
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
		if _, uerr := applyEvent(st, e); uerr != nil {
			return nil
		}
		return nil
	}

	// The gate folds the event and the decision into one state write,
	// because it runs while the developer waits for the subagent.
	cfg := sessionConfig(st, e.CWD)
	var res policy.Result
	if _, uerr := st.Update(e.SessionID, repoHashFor(e.CWD), func(s *state.Session) {
		applyTo(s, e)
		res = policy.Evaluate(cfg, *s)
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
