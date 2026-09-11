// Package events normalizes harness payloads into the small event shape
// the rest of ZeroTurn understands.
//
// The decoder declares fields only for permitted information. Anything a
// harness sends that is not listed here is never bound to a variable, so
// prompts, responses, transcript paths, and tool arguments unrelated to
// ZeroTurn cannot reach policy evaluation or local state.
package events

import (
	"encoding/json"
	"errors"
	"io"
	"io/ioutil"
)

// Contract is the adapter contract version. Adapters send it so that an
// incompatible adapter fails with a clear message rather than silently
// producing wrong counts.
const Contract = "zeroturn.event/1"

const (
	TypeStatus       = "status"
	TypeSubagentPre  = "subagent.pre"
	TypeSubagentStop = "subagent.stop"
	TypeSubagentStrt = "subagent.start"
	TypeSessionStop  = "session.stop"
	TypeSessionEnd   = "session.end"
)

type Event struct {
	Contract       string
	Harness        string
	HarnessVersion string
	Type           string
	SessionID      string
	CWD            string
	Model          string

	ContextPct  *float64
	ContextSize *int

	FiveHourPct      *float64
	FiveHourResetsAt *int64
	SevenDayPct      *float64
	SevenDayResetsAt *int64

	DurationMS *int64

	AgentID   string
	AgentType string

	BackgroundTasks *int

	EndReason string
}

var ErrEmpty = errors.New("no event payload was supplied on standard input")

// claudePayload lists every field ZeroTurn reads from Claude Code. The
// absence of transcript_path, tool_input, and last_assistant_message here
// is the mechanism that keeps private content out of ZeroTurn.
type claudePayload struct {
	SessionID     string `json:"session_id"`
	HookEventName string `json:"hook_event_name"`
	CWD           string `json:"cwd"`
	Version       string `json:"version"`
	ToolName      string `json:"tool_name"`
	AgentID       string `json:"agent_id"`
	AgentType     string `json:"agent_type"`
	Reason        string `json:"reason"`

	Model *struct {
		DisplayName string `json:"display_name"`
	} `json:"model"`

	Workspace *struct {
		CurrentDir string `json:"current_dir"`
	} `json:"workspace"`

	ContextWindow *struct {
		UsedPercentage    *float64 `json:"used_percentage"`
		ContextWindowSize *int     `json:"context_window_size"`
	} `json:"context_window"`

	RateLimits *struct {
		FiveHour *window `json:"five_hour"`
		SevenDay *window `json:"seven_day"`
	} `json:"rate_limits"`

	Cost *struct {
		TotalDurationMS *int64 `json:"total_duration_ms"`
	} `json:"cost"`

	// BackgroundTasks is counted, never inspected. Entries carry a
	// description that could echo user intent, so only the length is used.
	BackgroundTasks []json.RawMessage `json:"background_tasks"`
}

type window struct {
	UsedPercentage *float64 `json:"used_percentage"`
	ResetsAt       *int64   `json:"resets_at"`
}

// ParseClaude reads one Claude Code payload. eventName selects the
// interpretation because the status line payload carries no event name.
func ParseClaude(r io.Reader, eventName string) (Event, error) {
	raw, err := ioutil.ReadAll(r)
	if err != nil {
		return Event{}, err
	}
	if len(trimSpace(raw)) == 0 {
		return Event{}, ErrEmpty
	}
	var p claudePayload
	if err := json.Unmarshal(raw, &p); err != nil {
		return Event{}, errors.New("the payload is not valid JSON")
	}

	e := Event{
		Contract:       Contract,
		Harness:        "claude",
		HarnessVersion: p.Version,
		SessionID:      p.SessionID,
		CWD:            p.CWD,
		AgentID:        p.AgentID,
		AgentType:      p.AgentType,
		EndReason:      p.Reason,
	}
	if p.Model != nil {
		e.Model = p.Model.DisplayName
	}
	if e.CWD == "" && p.Workspace != nil {
		e.CWD = p.Workspace.CurrentDir
	}
	if p.ContextWindow != nil {
		e.ContextPct = p.ContextWindow.UsedPercentage
		e.ContextSize = p.ContextWindow.ContextWindowSize
	}
	if p.RateLimits != nil {
		if p.RateLimits.FiveHour != nil {
			e.FiveHourPct = p.RateLimits.FiveHour.UsedPercentage
			e.FiveHourResetsAt = p.RateLimits.FiveHour.ResetsAt
		}
		if p.RateLimits.SevenDay != nil {
			e.SevenDayPct = p.RateLimits.SevenDay.UsedPercentage
			e.SevenDayResetsAt = p.RateLimits.SevenDay.ResetsAt
		}
	}
	if p.Cost != nil {
		e.DurationMS = p.Cost.TotalDurationMS
	}
	if p.BackgroundTasks != nil {
		n := len(p.BackgroundTasks)
		e.BackgroundTasks = &n
	}

	name := eventName
	if name == "" {
		name = p.HookEventName
	}
	switch name {
	case "", "statusline", "StatusLine":
		e.Type = TypeStatus
	case "PreToolUse":
		if p.ToolName != "Agent" {
			return Event{}, errUnsupportedTool{p.ToolName}
		}
		e.Type = TypeSubagentPre
	case "SubagentStart":
		e.Type = TypeSubagentStrt
	case "SubagentStop":
		e.Type = TypeSubagentStop
	case "Stop":
		e.Type = TypeSessionStop
	case "SessionEnd":
		e.Type = TypeSessionEnd
	default:
		return Event{}, errors.New("event " + name + " is not part of the ZeroTurn contract")
	}
	return e, nil
}

type errUnsupportedTool struct{ name string }

func (e errUnsupportedTool) Error() string {
	return "PreToolUse fired for tool " + e.name + ", which ZeroTurn does not gate"
}

// IsUnsupportedTool reports whether the gate was invoked for a tool other
// than Agent. Adapters treat this as allow rather than as a failure.
func IsUnsupportedTool(err error) bool {
	_, ok := err.(errUnsupportedTool)
	return ok
}

// Normalized is the JSON form a non Claude adapter sends to ZeroTurn.
type Normalized struct {
	Contract        string   `json:"contract"`
	Harness         string   `json:"harness"`
	HarnessVersion  string   `json:"harnessVersion,omitempty"`
	Type            string   `json:"type"`
	SessionID       string   `json:"sessionId"`
	CWD             string   `json:"cwd,omitempty"`
	Model           string   `json:"model,omitempty"`
	ContextPct      *float64 `json:"contextPercent,omitempty"`
	ContextSize     *int     `json:"contextWindowSize,omitempty"`
	FiveHourPct     *float64 `json:"fiveHourPercent,omitempty"`
	SevenDayPct     *float64 `json:"sevenDayPercent,omitempty"`
	DurationMS      *int64   `json:"durationMs,omitempty"`
	AgentID         string   `json:"agentId,omitempty"`
	AgentType       string   `json:"agentType,omitempty"`
	BackgroundTasks *int     `json:"backgroundTasks,omitempty"`
	EndReason       string   `json:"endReason,omitempty"`
}

// ParseNormalized reads the adapter facing event format.
func ParseNormalized(r io.Reader) (Event, error) {
	raw, err := ioutil.ReadAll(r)
	if err != nil {
		return Event{}, err
	}
	if len(trimSpace(raw)) == 0 {
		return Event{}, ErrEmpty
	}
	var n Normalized
	if err := json.Unmarshal(raw, &n); err != nil {
		return Event{}, errors.New("the payload is not valid JSON")
	}
	if n.Contract != Contract {
		return Event{}, ContractError{Got: n.Contract, Want: Contract}
	}
	switch n.Type {
	case TypeStatus, TypeSubagentPre, TypeSubagentStrt, TypeSubagentStop, TypeSessionStop, TypeSessionEnd:
	default:
		return Event{}, errors.New("event type " + n.Type + " is not part of the ZeroTurn contract")
	}
	if n.Harness == "" {
		return Event{}, errors.New("the adapter did not name its harness")
	}
	return Event{
		Contract: Contract, Harness: n.Harness, HarnessVersion: n.HarnessVersion,
		Type: n.Type, SessionID: n.SessionID, CWD: n.CWD, Model: n.Model,
		ContextPct: n.ContextPct, ContextSize: n.ContextSize,
		FiveHourPct: n.FiveHourPct, SevenDayPct: n.SevenDayPct,
		DurationMS: n.DurationMS, AgentID: n.AgentID, AgentType: n.AgentType,
		BackgroundTasks: n.BackgroundTasks, EndReason: n.EndReason,
	}, nil
}

type ContractError struct{ Got, Want string }

func (e ContractError) Error() string {
	got := e.Got
	if got == "" {
		got = "an unnamed contract"
	}
	return "adapter sent " + got + ", this build speaks " + e.Want
}

func trimSpace(b []byte) []byte {
	i, j := 0, len(b)
	for i < j && isSpace(b[i]) {
		i++
	}
	for j > i && isSpace(b[j-1]) {
		j--
	}
	return b[i:j]
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}
