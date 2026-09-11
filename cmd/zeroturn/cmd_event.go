package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/cris-wendler/zeroturn/internal/events"
	"github.com/cris-wendler/zeroturn/internal/git"
	"github.com/cris-wendler/zeroturn/internal/policy"
	"github.com/cris-wendler/zeroturn/internal/state"
)

// applyEvent folds one normalized event into the stored session record.
func applyEvent(st *state.Store, e events.Event) (state.Session, error) {
	repoHash := ""
	if root, ok := git.FindRoot(e.CWD); ok {
		repoHash = state.RepoHash(root)
	}
	return st.Update(e.SessionID, repoHash, func(s *state.Session) {
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
			s.AddActive(e.AgentID)
		case events.TypeSubagentStop:
			s.RemoveActive(e.AgentID)
		}
	})
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
	sess, uerr := applyEvent(st, e)
	if uerr != nil {
		return nil
	}

	if e.Type != events.TypeSubagentPre {
		return nil
	}

	res := policy.Evaluate(sessionConfig(st, e.CWD), sess)
	st.Update(e.SessionID, "", func(s *state.Session) {
		s.LastDecision = res.Decision
		switch res.Decision {
		case policy.DecisionAsk:
			s.ConfirmRequests++
		case policy.DecisionDeny:
			s.DeniedStarts++
		default:
			s.AllowedStarts++
		}
	})

	// An allow decision prints nothing so that ZeroTurn does not override
	// the permission rules the user already configured in the harness.
	if res.Decision == policy.DecisionAllow {
		return nil
	}

	var out decisionOutput
	out.HookSpecificOutput.HookEventName = "PreToolUse"
	out.HookSpecificOutput.PermissionDecision = res.Decision
	out.HookSpecificOutput.PermissionDecisionReason = res.Reason
	b, merr := json.Marshal(out)
	if merr != nil {
		return nil
	}
	fmt.Println(string(b))
	return nil
}
