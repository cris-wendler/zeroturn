// Package policy turns observed session state into a guard decision.
//
// Percentages from different measurements are compared against their own
// thresholds and never summed. Context use, the five hour window, and the
// seven day window describe different things.
package policy

import (
	"fmt"
	"sort"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/state"
)

const (
	DecisionAllow = "allow"
	DecisionAsk   = "ask"
	DecisionDeny  = "deny"
)

const (
	LevelOK       = "ok"
	LevelWarn     = "warn"
	LevelConfirm  = "confirm"
	LevelCritical = "critical"
)

// Trigger records one threshold that has been crossed.
type Trigger struct {
	Name     string  `json:"name"`
	Observed float64 `json:"observed"`
	Limit    float64 `json:"limit"`
	Level    string  `json:"level"`
	Text     string  `json:"text"`
	// Available is false when the harness did not supply the measurement.
	Available bool `json:"available"`
}

type Result struct {
	Mode     string    `json:"mode"`
	Level    string    `json:"level"`
	Decision string    `json:"decision"`
	Reason   string    `json:"reason,omitempty"`
	Triggers []Trigger `json:"triggers"`
}

func rank(level string) int {
	switch level {
	case LevelCritical:
		return 3
	case LevelConfirm:
		return 2
	case LevelWarn:
		return 1
	}
	return 0
}

// Evaluate reports the condition of a session without changing anything.
func Evaluate(c config.Config, s state.Session) Result {
	var t []Trigger

	if s.ContextPct != nil {
		v := *s.ContextPct
		switch {
		case v >= float64(c.Guard.Context.Critical):
			t = append(t, Trigger{"context", v, float64(c.Guard.Context.Critical), LevelCritical,
				fmt.Sprintf("Context is %.0f%%", v), true})
		case v >= float64(c.Guard.Context.Confirm):
			t = append(t, Trigger{"context", v, float64(c.Guard.Context.Confirm), LevelConfirm,
				fmt.Sprintf("Context is %.0f%%", v), true})
		case v >= float64(c.Guard.Context.Warn):
			t = append(t, Trigger{"context", v, float64(c.Guard.Context.Warn), LevelWarn,
				fmt.Sprintf("Context is %.0f%%", v), true})
		}
	}

	if s.FiveHourPct != nil {
		v := *s.FiveHourPct
		if v >= float64(c.Guard.Limits.FiveHourWarn) {
			t = append(t, Trigger{"fiveHour", v, float64(c.Guard.Limits.FiveHourWarn), LevelConfirm,
				fmt.Sprintf("Five hour usage is %.0f%%", v), true})
		}
	}
	if s.SevenDayPct != nil {
		v := *s.SevenDayPct
		if v >= float64(c.Guard.Limits.SevenDayWarn) {
			t = append(t, Trigger{"sevenDay", v, float64(c.Guard.Limits.SevenDayWarn), LevelConfirm,
				fmt.Sprintf("Seven day usage is %.0f%%", v), true})
		}
	}

	if s.DurationMS != nil {
		mins := float64(*s.DurationMS) / 60000.0
		if mins >= float64(c.Guard.Session.DurationWarnMinutes) {
			t = append(t, Trigger{"duration", mins, float64(c.Guard.Session.DurationWarnMinutes), LevelConfirm,
				fmt.Sprintf("Session has run %s", HumanMinutes(mins)), true})
		}
	}

	if s.ActiveSubagents >= c.Guard.Session.ActiveSubagentsWarn {
		t = append(t, Trigger{"activeSubagents", float64(s.ActiveSubagents), float64(c.Guard.Session.ActiveSubagentsWarn),
			LevelConfirm, fmt.Sprintf("%d subagents are active", s.ActiveSubagents), true})
	}
	if s.SubagentStarts >= c.Guard.Session.SubagentStartsWarn {
		t = append(t, Trigger{"subagentStarts", float64(s.SubagentStarts), float64(c.Guard.Session.SubagentStartsWarn),
			LevelConfirm, fmt.Sprintf("%d subagents started this session", s.SubagentStarts), true})
	}
	if s.BackgroundTasks > 0 {
		t = append(t, Trigger{"backgroundTasks", float64(s.BackgroundTasks), 1, LevelWarn,
			fmt.Sprintf("%d background tasks are running", s.BackgroundTasks), true})
	}

	sort.SliceStable(t, func(i, j int) bool { return rank(t[i].Level) > rank(t[j].Level) })

	level := LevelOK
	for _, x := range t {
		if rank(x.Level) > rank(level) {
			level = x.Level
		}
	}

	r := Result{Mode: c.Guard.Mode, Level: level, Triggers: t, Decision: DecisionAllow}
	if t == nil {
		r.Triggers = []Trigger{}
	}

	switch c.Guard.Mode {
	case config.ModeObserve:
		r.Decision = DecisionAllow
	case config.ModeConfirm:
		if rank(level) >= rank(LevelConfirm) {
			r.Decision = DecisionAsk
			r.Reason = Reason(t, DecisionAsk)
		}
	case config.ModeStrict:
		if level == LevelCritical {
			r.Decision = DecisionDeny
			r.Reason = Reason(t, DecisionDeny)
		} else if rank(level) >= rank(LevelConfirm) {
			r.Decision = DecisionAsk
			r.Reason = Reason(t, DecisionAsk)
		}
	}
	return r
}

// CredentialDecision reports what to do when a file the model is about
// to read holds something shaped like a credential. The reason names the
// file, the line, and the category, and never the value, because a
// warning that repeats the secret has moved it rather than contained it.
func CredentialDecision(mode, file string, line int, categories []string) (decision, reason string) {
	if len(categories) == 0 {
		return DecisionAllow, ""
	}
	switch mode {
	case config.CredentialAsk:
		decision = DecisionAsk
	case config.CredentialDeny:
		decision = DecisionDeny
	default:
		return DecisionAllow, ""
	}
	what := categories[0]
	if len(categories) > 1 {
		what = fmt.Sprintf("%s and %d more", what, len(categories)-1)
	}
	head := "Reading this file would put a credential into the conversation."
	if decision == DecisionDeny {
		head = "ZeroTurn denied reading this file, because it holds a credential."
	}
	return decision, fmt.Sprintf("%s %s line %d looks like %s. Approving means the value is shared and should be rotated.",
		head, file, line, what)
}

// Reason builds the short message shown in the harness permission prompt.
// It names at most two triggers so the prompt stays readable, and it never
// carries the full policy or the session report into model context.
func Reason(t []Trigger, decision string) string {
	head := "New subagent requires approval."
	if decision == DecisionDeny {
		head = "New subagent denied by ZeroTurn strict mode."
	}
	if len(t) == 0 {
		return head
	}
	if len(t) == 1 {
		return head + " " + t[0].Text + "."
	}
	return fmt.Sprintf("%s %s and %s.", head, t[0].Text, lowerFirst(t[1].Text))
}

func lowerFirst(s string) string {
	if s == "" {
		return s
	}
	if s[0] >= 'A' && s[0] <= 'Z' {
		return string(s[0]+32) + s[1:]
	}
	return s
}

func HumanMinutes(m float64) string {
	total := int(m)
	h := total / 60
	mm := total % 60
	if h == 0 {
		return fmt.Sprintf("%dm", mm)
	}
	return fmt.Sprintf("%dh%02dm", h, mm)
}
