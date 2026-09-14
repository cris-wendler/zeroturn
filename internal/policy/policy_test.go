package policy

import (
	"strings"
	"testing"
	"time"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/state"
)

func f(v float64) *float64 { return &v }
func i64(v int64) *int64   { return &v }

func modeConfig(mode string) config.Config {
	c := config.Default()
	c.Guard.Mode = mode
	return c
}

// approved builds the guard for a machine that has approved Strict,
// which is what the tests below are about. The downgrade itself is
// tested in guard_test.go.
func approved(c config.Config) Guard { return NewGuard(c, true) }

func withMode(mode string) Guard { return approved(modeConfig(mode)) }

func TestEmptySessionAllowsInEveryMode(t *testing.T) {
	for _, mode := range []string{config.ModeObserve, config.ModeConfirm, config.ModeStrict} {
		r := Evaluate(withMode(mode), state.Session{})
		if r.Decision != DecisionAllow || r.Level != LevelOK {
			t.Errorf("%s: got %s/%s, want allow/ok", mode, r.Decision, r.Level)
		}
		if r.Triggers == nil {
			t.Errorf("%s: triggers must be an empty list, not null, for the JSON contract", mode)
		}
	}
}

func TestObserveNeverBlocks(t *testing.T) {
	s := state.Session{ContextPct: f(99), FiveHourPct: f(99), ActiveSubagents: 9, SubagentStarts: 20}
	r := Evaluate(withMode(config.ModeObserve), s)
	if r.Decision != DecisionAllow {
		t.Fatalf("observe returned %s", r.Decision)
	}
	if r.Level != LevelCritical {
		t.Fatalf("observe must still report the level, got %s", r.Level)
	}
	if r.Reason != "" {
		t.Fatalf("observe must not produce a reason for the harness, got %q", r.Reason)
	}
}

func TestContextThresholdBoundaries(t *testing.T) {
	cases := []struct {
		pct   float64
		level string
	}{
		{69.9, LevelOK},
		{70, LevelWarn},
		{79.9, LevelWarn},
		{80, LevelConfirm},
		{89.9, LevelConfirm},
		{90, LevelCritical},
		{100, LevelCritical},
	}
	for _, c := range cases {
		r := Evaluate(withMode(config.ModeObserve), state.Session{ContextPct: f(c.pct)})
		if r.Level != c.level {
			t.Errorf("context %.1f: level %s, want %s", c.pct, r.Level, c.level)
		}
	}
}

func TestConfirmAsksOnlyAtConfirmLevel(t *testing.T) {
	c := withMode(config.ModeConfirm)
	if r := Evaluate(c, state.Session{ContextPct: f(75)}); r.Decision != DecisionAllow {
		t.Errorf("warn level must not ask, got %s", r.Decision)
	}
	r := Evaluate(c, state.Session{ContextPct: f(82), ActiveSubagents: 2})
	if r.Decision != DecisionAsk {
		t.Fatalf("got %s, want ask", r.Decision)
	}
	want := "New subagent requires approval. Context is 82% and 2 subagents are active."
	if r.Reason != want {
		t.Errorf("reason\n got %q\nwant %q", r.Reason, want)
	}
	if r := Evaluate(c, state.Session{ContextPct: f(95)}); r.Decision != DecisionAsk {
		t.Errorf("confirm mode must never deny, got %s", r.Decision)
	}
}

func TestStrictDeniesOnlyAtCritical(t *testing.T) {
	c := withMode(config.ModeStrict)
	if r := Evaluate(c, state.Session{ContextPct: f(85)}); r.Decision != DecisionAsk {
		t.Errorf("confirm level in strict mode must ask, got %s", r.Decision)
	}
	if r := Evaluate(c, state.Session{FiveHourPct: f(100), SevenDayPct: f(100), ActiveSubagents: 10}); r.Decision != DecisionAsk {
		t.Errorf("no hard limit crossed, strict must ask rather than deny, got %s", r.Decision)
	}
	r := Evaluate(c, state.Session{ContextPct: f(91)})
	if r.Decision != DecisionDeny {
		t.Fatalf("got %s, want deny", r.Decision)
	}
	if !strings.HasPrefix(r.Reason, "New subagent denied by ZeroTurn strict mode.") {
		t.Errorf("reason %q", r.Reason)
	}
}

func TestEachThresholdTriggers(t *testing.T) {
	cfg := modeConfig(config.ModeConfirm)
	c := approved(cfg)
	mins := int64(cfg.Guard.Session.DurationWarnMinutes) * 60000
	cases := map[string]state.Session{
		"fiveHour":        {FiveHourPct: f(75)},
		"sevenDay":        {SevenDayPct: f(75)},
		"duration":        {DurationMS: i64(mins)},
		"activeSubagents": {ActiveSubagents: cfg.Guard.Session.ActiveSubagentsWarn},
		"subagentStarts":  {SubagentStarts: cfg.Guard.Session.SubagentStartsWarn},
	}
	for name, s := range cases {
		r := Evaluate(c, s)
		if len(r.Triggers) != 1 || r.Triggers[0].Name != name {
			t.Errorf("%s: triggers %+v", name, r.Triggers)
			continue
		}
		if r.Decision != DecisionAsk {
			t.Errorf("%s: decision %s, want ask", name, r.Decision)
		}
	}
}

func TestBackgroundWorkWarnsWithoutAsking(t *testing.T) {
	r := Evaluate(withMode(config.ModeConfirm), state.Session{BackgroundTasks: 1})
	if r.Level != LevelWarn || r.Decision != DecisionAllow {
		t.Fatalf("got %s/%s, want warn/allow", r.Level, r.Decision)
	}
}

// Missing measurements must never be treated as zero or as a crossing.
func TestMissingMeasurementsAreIgnored(t *testing.T) {
	r := Evaluate(withMode(config.ModeStrict), state.Session{ContextPct: nil, FiveHourPct: nil, DurationMS: nil})
	if len(r.Triggers) != 0 {
		t.Fatalf("triggers from absent data: %+v", r.Triggers)
	}
}

// Percentages from different measurements are compared separately. Two
// values that are each below their own threshold must not combine.
func TestPercentagesAreNeverSummed(t *testing.T) {
	r := Evaluate(withMode(config.ModeStrict), state.Session{ContextPct: f(60), FiveHourPct: f(60), SevenDayPct: f(60)})
	if len(r.Triggers) != 0 || r.Decision != DecisionAllow {
		t.Fatalf("got %s with %+v", r.Decision, r.Triggers)
	}
}

func TestTriggersSortedMostSevereFirst(t *testing.T) {
	r := Evaluate(withMode(config.ModeStrict), state.Session{BackgroundTasks: 2, FiveHourPct: f(80), ContextPct: f(95)})
	if r.Triggers[0].Level != LevelCritical || r.Triggers[len(r.Triggers)-1].Level != LevelWarn {
		t.Fatalf("order %+v", r.Triggers)
	}
}

func TestReasonNamesAtMostTwoTriggers(t *testing.T) {
	r := Evaluate(withMode(config.ModeConfirm), state.Session{
		ContextPct: f(85), FiveHourPct: f(90), SevenDayPct: f(90), ActiveSubagents: 5,
	})
	if n := strings.Count(r.Reason, "%"); n > 2 {
		t.Fatalf("reason carries %d measurements: %q", n, r.Reason)
	}
	if strings.Count(r.Reason, " and ") != 1 {
		t.Fatalf("reason %q", r.Reason)
	}
}

func TestHumanMinutes(t *testing.T) {
	for in, want := range map[float64]string{0: "0m", 59.9: "59m", 60: "1h00m", 192: "3h12m"} {
		if got := HumanMinutes(in); got != want {
			t.Errorf("HumanMinutes(%v) = %q, want %q", in, got, want)
		}
	}
}

func BenchmarkEvaluate(b *testing.B) {
	c := withMode(config.ModeConfirm)
	s := state.Session{ContextPct: f(82), FiveHourPct: f(81), SevenDayPct: f(47), DurationMS: i64(11520000), ActiveSubagents: 2, SubagentStarts: 5}
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if r := Evaluate(c, s); r.Decision == "" {
			b.Fatal("no decision")
		}
	}
}

func TestCredentialDecision(t *testing.T) {
	cases := []struct {
		mode       string
		categories []string
		want       string
	}{
		{config.CredentialAsk, []string{"aws access key id"}, DecisionAsk},
		{config.CredentialDeny, []string{"aws access key id"}, DecisionDeny},
		{config.CredentialOff, []string{"aws access key id"}, DecisionAllow},
		{config.CredentialAsk, nil, DecisionAllow},
	}
	for _, c := range cases {
		got, reason := CredentialDecision(c.mode, "deploy.env", 2, c.categories)
		if got != c.want {
			t.Errorf("%s with %v: got %s, want %s", c.mode, c.categories, got, c.want)
			continue
		}
		if c.want == DecisionAllow {
			if reason != "" {
				t.Errorf("%s: allow carried a reason %q", c.mode, reason)
			}
			continue
		}
		for _, want := range []string{"deploy.env", "line 2", "aws access key id", "rotated"} {
			if !strings.Contains(reason, want) {
				t.Errorf("%s: reason %q lacks %q", c.mode, reason, want)
			}
		}
	}
}

func TestCredentialDecisionSummarisesSeveralCategories(t *testing.T) {
	_, reason := CredentialDecision(config.CredentialAsk, "f", 1, []string{"private key", "github token", "slack token"})
	if !strings.Contains(reason, "private key and 2 more") {
		t.Fatalf("reason %q", reason)
	}
}

// A session with a baseline reading, taken span ago in the window that is
// running now, which resets in resetsIn.
func rising(base, now float64, span, resetsIn time.Duration, at time.Time) state.Session {
	reset := at.Add(resetsIn).Unix()
	took := at.Add(-span)
	return state.Session{
		FiveHourPct:       f(now),
		FiveHourResetsAt:  &reset,
		FiveHourBasePct:   f(base),
		FiveHourBaseAt:    &took,
		FiveHourBaseReset: &reset,
	}
}

func TestProjectionAsksWhenTheRateOutrunsTheWindow(t *testing.T) {
	at := time.Now()
	// 30% in an hour, three hours to go: 40 + 90 lands well past 100.
	s := rising(10, 40, time.Hour, 3*time.Hour, at)
	r := EvaluateAt(withMode(config.ModeConfirm), s, at)
	if r.Decision != DecisionAsk {
		t.Fatalf("got %s, want ask: %+v", r.Decision, r.Triggers)
	}
	var got *Trigger
	for i := range r.Triggers {
		if r.Triggers[i].Name == "projection" {
			got = &r.Triggers[i]
		}
	}
	if got == nil {
		t.Fatalf("no projection trigger: %+v", r.Triggers)
	}
	for _, want := range []string{"40%", "30% an hour", "3h00m"} {
		if !strings.Contains(got.Text, want) {
			t.Errorf("text %q does not name %s", got.Text, want)
		}
	}
}

func TestHeavySessionThatFinishesInsideTheWindowStaysQuiet(t *testing.T) {
	at := time.Now()
	// Half the window used already, but it resets in twenty minutes and
	// the rate would add only another 10%. Nothing to say.
	s := rising(20, 50, time.Hour, 20*time.Minute, at)
	r := EvaluateAt(withMode(config.ModeConfirm), s, at)
	if r.Decision != DecisionAllow || r.Level != LevelOK {
		t.Fatalf("got %s/%s, want allow/ok: %+v", r.Decision, r.Level, r.Triggers)
	}
}

func TestProjectionIgnoresARateItCannotTrust(t *testing.T) {
	at := time.Now()
	cases := map[string]state.Session{
		"span shorter than the minimum": rising(0, 20, 5*time.Minute, 4*time.Hour, at),
		"usage not rising":              rising(40, 40, time.Hour, 4*time.Hour, at),
		"window already reset":          rising(10, 40, time.Hour, -time.Minute, at),
	}
	stale := rising(10, 40, time.Hour, 3*time.Hour, at)
	other := *stale.FiveHourBaseReset + 900
	stale.FiveHourBaseReset = &other
	cases["baseline from an earlier window"] = stale

	missing := rising(10, 40, time.Hour, 3*time.Hour, at)
	missing.FiveHourResetsAt = nil
	cases["no reset time"] = missing

	for name, s := range cases {
		if _, ok := Project(s, at); ok {
			t.Errorf("%s: projected anyway", name)
		}
		if r := EvaluateAt(withMode(config.ModeConfirm), s, at); r.Decision != DecisionAllow {
			t.Errorf("%s: decision %s", name, r.Decision)
		}
	}
}

func TestProjectionCanBeSwitchedOff(t *testing.T) {
	at := time.Now()
	c := modeConfig(config.ModeConfirm)
	c.Guard.Limits.Projection = config.ProjectionOff
	if r := EvaluateAt(approved(c), rising(10, 40, time.Hour, 3*time.Hour, at), at); r.Decision != DecisionAllow {
		t.Fatalf("got %s with projection off", r.Decision)
	}
}

func TestProjectionIsNotRepeatedOnceTheThresholdIsCrossed(t *testing.T) {
	at := time.Now()
	s := rising(50, 90, time.Hour, 3*time.Hour, at)
	r := EvaluateAt(withMode(config.ModeConfirm), s, at)
	for _, x := range r.Triggers {
		if x.Name == "projection" {
			t.Fatalf("projection repeats the five hour trigger: %+v", r.Triggers)
		}
	}
	if r.Decision != DecisionAsk {
		t.Fatalf("got %s, want ask", r.Decision)
	}
}

func TestProjectionNeverDeniesInStrictMode(t *testing.T) {
	at := time.Now()
	r := EvaluateAt(withMode(config.ModeStrict), rising(0, 40, time.Hour, 4*time.Hour, at), at)
	if r.Decision != DecisionAsk {
		t.Fatalf("got %s, want ask: a projection is an estimate, not a fact", r.Decision)
	}
}
