// Package policy turns observed session state into a guard decision.
//
// Percentages from different measurements are compared against their own
// thresholds and never summed. Context use, the five hour window, and the
// seven day window describe different things.
package policy

import (
	"fmt"
	"math"
	"sort"
	"time"

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

// Projection is what the rate of use implies about a window that has not
// filled yet.
type Projection struct {
	// RatePerHour is how many percentage points of the window are being
	// used in an hour, measured from the first reading of this window.
	RatePerHour float64
	// HoursLeft is the time until the window resets.
	HoursLeft float64
	// Percent is where the window lands at this rate when it resets.
	Percent float64
}

// minRateSpan is how long a session has to run before a rate means
// anything. A burst in the first minutes of a window would otherwise
// project into the hundreds.
const minRateSpan = 15 * time.Minute

// Project reports where the five hour window lands at the current rate of
// use, and whether that rate can be measured at all. It answers a
// different question from a threshold: not how full the window is, but
// whether it will still be there when the work needs it.
func Project(s state.Session, now time.Time) (Projection, bool) {
	if s.FiveHourPct == nil || s.FiveHourBasePct == nil || s.FiveHourBaseAt == nil || s.FiveHourResetsAt == nil {
		return Projection{}, false
	}
	// A baseline taken under a different reset time belongs to a window
	// that has already ended.
	if s.FiveHourBaseReset == nil || *s.FiveHourBaseReset != *s.FiveHourResetsAt {
		return Projection{}, false
	}
	span := now.Sub(*s.FiveHourBaseAt)
	if span < minRateSpan {
		return Projection{}, false
	}
	left := time.Unix(*s.FiveHourResetsAt, 0).Sub(now)
	if left <= 0 || left > 5*time.Hour {
		return Projection{}, false
	}
	rate := (*s.FiveHourPct - *s.FiveHourBasePct) / span.Hours()
	if rate <= 0 {
		return Projection{}, false
	}
	hoursLeft := left.Hours()
	return Projection{RatePerHour: rate, HoursLeft: hoursLeft, Percent: *s.FiveHourPct + rate*hoursLeft}, true
}

// Evaluate reports the condition of a session without changing anything.
func Evaluate(g Guard, s state.Session) Result {
	return EvaluateAt(g, s, time.Now())
}

// EvaluateAt is Evaluate with the current time supplied, which the
// projection needs and which tests need to control.
func EvaluateAt(g Guard, s state.Session, now time.Time) Result {
	c := g.cfg
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

	crossed := false
	if s.FiveHourPct != nil {
		v := *s.FiveHourPct
		if v >= float64(c.Guard.Limits.FiveHourWarn) {
			crossed = true
			t = append(t, Trigger{"fiveHour", v, float64(c.Guard.Limits.FiveHourWarn), LevelConfirm,
				fmt.Sprintf("Five hour usage is %.0f%%", v), true})
		}
	}
	// The threshold above asks how full the window is. This asks where the
	// window is heading: a heavy session that will still finish inside it
	// says nothing, and a moderate one burning faster than the clock does.
	if !crossed && c.Guard.Limits.Projection == config.ProjectionOn {
		if p, ok := Project(s, now); ok && p.Percent >= 100 {
			t = append(t, Trigger{"projection", p.Percent, 100, LevelConfirm,
				fmt.Sprintf("Five hour usage is %.0f%% and rising %.0f%% an hour, with %s left before it resets",
					*s.FiveHourPct, p.RatePerHour, HumanMinutes(math.Round(p.HoursLeft*60))), true})
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
			LevelConfirm, countPhrase(s.ActiveSubagents, "1 subagent is active", "%d subagents are active"), true})
	}
	if s.SubagentStarts >= c.Guard.Session.SubagentStartsWarn {
		t = append(t, Trigger{"subagentStarts", float64(s.SubagentStarts), float64(c.Guard.Session.SubagentStartsWarn),
			LevelConfirm, countPhrase(s.SubagentStarts, "1 subagent started this session", "%d subagents started this session"), true})
	}
	if s.BackgroundTasks > 0 {
		t = append(t, Trigger{"backgroundTasks", float64(s.BackgroundTasks), 1, LevelWarn,
			countPhrase(s.BackgroundTasks, "1 background task is running", "%d background tasks are running"), true})
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

// PromptCredentialDecision reports whether a message about to be sent
// should be stopped. The harness has no way to ask about a message, so
// the answer is block or nothing, and the reason never repeats the value.
func PromptCredentialDecision(prompts string, categories []string) (block bool, reason string) {
	if prompts != config.PromptsBlock || len(categories) == 0 {
		return false, ""
	}
	what := categories[0]
	if len(categories) > 1 {
		what = fmt.Sprintf("%s and %d more", what, len(categories)-1)
	}
	return true, fmt.Sprintf("Your message was not sent. It contains what looks like %s, and sending it would mean rotating the value. "+
		"Remove it and send the message again. Turn this check off with zeroturn policy set guard.credentials.prompts off.", what)
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

// countPhrase writes a counted noun in the right form. These strings are
// read by a person in a permission prompt, where "1 subagents are active"
// reads as a defect in the tool asking for their approval.
func countPhrase(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return fmt.Sprintf(many, n)
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
