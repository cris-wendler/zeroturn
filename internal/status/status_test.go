package status

import (
	"strings"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/output"
	"github.com/cris-wendler/zeroturn/internal/policy"
	"github.com/cris-wendler/zeroturn/internal/state"
)

func f(v float64) *float64 { return &v }
func i64(v int64) *int64   { return &v }

func render(mode string, s state.Session) string {
	c := config.Default()
	c.Guard.Mode = mode
	return Render(Line{Session: s, Result: policy.Evaluate(c, s), Color: output.Color{}})
}

func TestFullLine(t *testing.T) {
	s := state.Session{ContextPct: f(76), FiveHourPct: f(40), SevenDayPct: f(47), DurationMS: i64(11520000), SubagentStarts: 1, ActiveSubagents: 1}
	if got := render(config.ModeConfirm, s); got != "ZT  ctx 76%  5h 40%  7d 47%  session 3h12m  agents 1  warn" {
		t.Fatalf("got %q", got)
	}
}

func TestOnlyAvailableFieldsShown(t *testing.T) {
	got := render(config.ModeObserve, state.Session{ContextPct: f(20)})
	if got != "ZT  ctx 20%" {
		t.Fatalf("got %q", got)
	}
	if got := render(config.ModeObserve, state.Session{}); got != "ZT  session data unavailable" {
		t.Fatalf("got %q", got)
	}
}

func TestLastWordNamesTheGateDecision(t *testing.T) {
	cases := []struct {
		mode string
		pct  float64
		want string
	}{
		{config.ModeObserve, 85, "warn"},
		{config.ModeConfirm, 85, "ask"},
		{config.ModeStrict, 95, "deny"},
		{config.ModeConfirm, 95, "ask"},
		{config.ModeConfirm, 10, ""},
	}
	for _, c := range cases {
		got := render(c.mode, state.Session{ContextPct: f(c.pct)})
		last := got[strings.LastIndex(got, " ")+1:]
		if c.want == "" {
			if strings.HasSuffix(got, "warn") || strings.HasSuffix(got, "ask") || strings.HasSuffix(got, "deny") {
				t.Errorf("%s %v: unexpected condition in %q", c.mode, c.pct, got)
			}
			continue
		}
		if last != c.want {
			t.Errorf("%s %v: got %q, want last word %q", c.mode, c.pct, got, c.want)
		}
	}
}

func TestNoColorWithoutTerminal(t *testing.T) {
	got := render(config.ModeStrict, state.Session{ContextPct: f(95)})
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("escape codes in %q", got)
	}
}
