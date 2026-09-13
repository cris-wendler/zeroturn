package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/state"
)

func f64p(v float64) *float64 { return &v }

func sessionWith(repoHash string, outcomes ...state.GateOutcome) state.Session {
	return state.Session{SessionID: "s", RepoHash: repoHash, GateOutcomes: outcomes}
}

func ask(trigger string, value float64, approved bool) state.GateOutcome {
	o := state.GateOutcome{Decision: "ask", Approved: approved, Triggers: []string{trigger}}
	switch trigger {
	case "context":
		o.ContextPct = f64p(value)
	case "fiveHour":
		o.FiveHourPct = f64p(value)
	case "activeSubagents":
		o.ActiveSubagents = int(value)
	}
	return o
}

func TestTuningSuggestsRaisingAThresholdAlwaysApproved(t *testing.T) {
	c := config.Default()
	var outcomes []state.GateOutcome
	for _, v := range []float64{76, 78, 79, 77, 81} {
		outcomes = append(outcomes, ask("fiveHour", v, true))
	}
	r := analyse(c, []state.Session{sessionWith("repo", outcomes...)}, "repo")
	if r.Asks != 5 || r.Approved != 5 {
		t.Fatalf("counts %+v", r)
	}
	if len(r.Tunings) != 1 {
		t.Fatalf("tunings %+v", r.Tunings)
	}
	got := r.Tunings[0]
	if got.Suggested == nil || *got.Suggested != 82 {
		t.Fatalf("suggested %v, want 82, one above the highest approved", got.Suggested)
	}
	if !strings.Contains(got.Reason, "all 5 asks were approved") {
		t.Fatalf("reason %q", got.Reason)
	}
}

func TestTuningLeavesAThresholdThatIsWorking(t *testing.T) {
	c := config.Default()
	outcomes := []state.GateOutcome{
		ask("context", 82, false), ask("context", 85, false), ask("context", 88, false),
		ask("context", 81, true), ask("context", 90, false),
	}
	r := analyse(c, []state.Session{sessionWith("repo", outcomes...)}, "repo")
	got := r.Tunings[0]
	if got.Suggested != nil {
		t.Fatalf("suggested a change to a threshold that is doing its job: %v", *got.Suggested)
	}
	if !strings.Contains(got.Reason, "doing its job") {
		t.Fatalf("reason %q", got.Reason)
	}
}

func TestTuningUsesThePointWhereTheAnswerChanged(t *testing.T) {
	c := config.Default()
	outcomes := []state.GateOutcome{
		ask("context", 81, true), ask("context", 83, true), ask("context", 84, true),
		ask("context", 86, true), ask("context", 92, false),
	}
	r := analyse(c, []state.Session{sessionWith("repo", outcomes...)}, "repo")
	got := r.Tunings[0]
	if got.Suggested == nil || *got.Suggested != 92 {
		t.Fatalf("suggested %v, want the value where the answer changed", got.Suggested)
	}
}

func TestTuningStaysQuietWithoutEnoughObservations(t *testing.T) {
	c := config.Default()
	outcomes := []state.GateOutcome{ask("context", 82, true), ask("context", 84, true)}
	r := analyse(c, []state.Session{sessionWith("repo", outcomes...)}, "repo")
	if r.Tunings[0].Suggested != nil {
		t.Fatal("suggested a change from two observations")
	}
	if !strings.Contains(r.Tunings[0].Reason, "too few") {
		t.Fatalf("reason %q", r.Tunings[0].Reason)
	}
}

func TestTuningIgnoresOtherRepositories(t *testing.T) {
	c := config.Default()
	mine := sessionWith("mine", ask("context", 82, true))
	other := sessionWith("other", ask("context", 99, false), ask("context", 98, false))
	r := analyse(c, []state.Session{mine, other}, "mine")
	if r.Sessions != 1 || r.Asks != 1 {
		t.Fatalf("another repository's history leaked in: %+v", r)
	}
}

// The whole point is that a denial cannot be approved, so it is counted
// separately and never used to suggest a threshold.
func TestDenialsAreCountedButNotTuned(t *testing.T) {
	c := config.Default()
	outcomes := []state.GateOutcome{
		{Decision: "deny", Triggers: []string{"context"}, ContextPct: f64p(95)},
		{Decision: "deny", Triggers: []string{"context"}, ContextPct: f64p(96)},
	}
	r := analyse(c, []state.Session{sessionWith("repo", outcomes...)}, "repo")
	if r.Denied != 2 || r.Asks != 0 || len(r.Tunings) != 0 {
		t.Fatalf("%+v", r)
	}
}

// End to end: the gate asks, a subagent starts, and that shows up as an
// approval without anything about the work being recorded.
func TestApprovalIsLearnedFromWhatFollows(t *testing.T) {
	work, _ := repoWithConfig(t, func(c *config.Config) { c.Guard.Mode = config.ModeConfirm })
	statusline(t, work, "s", contextAt(85))
	if d, _ := decision(t, gate(t, work, "s")); d != "ask" {
		t.Fatal("the gate did not ask")
	}
	run(t, work, claudeEvent(t, "claude/subagent-start.json", work, "s",
		func(m map[string]interface{}) { m["agent_id"] = "a1" }),
		"event", "--harness", "claude", "--event", "SubagentStart")

	r := run(t, work, "", "policy", "tune", "--json")
	var report tuneReport
	if err := json.Unmarshal([]byte(r.stdout), &report); err != nil {
		t.Fatalf("tune output: %v\n%s", err, r.stdout)
	}
	if report.Asks != 1 || report.Approved != 1 {
		t.Fatalf("asks %d approved %d", report.Asks, report.Approved)
	}

	// A second ask with no subagent after it is a decline.
	statusline(t, work, "s", contextAt(97))
	gate(t, work, "s")
	run(t, work, claudeEvent(t, "claude/stop.json", work, "s", nil), "event", "--harness", "claude", "--event", "Stop")
	r = run(t, work, "", "policy", "tune", "--json")
	if err := json.Unmarshal([]byte(r.stdout), &report); err != nil {
		t.Fatal(err)
	}
	if report.Asks != 2 || report.Approved != 1 {
		t.Fatalf("asks %d approved %d, want 2 and 1", report.Asks, report.Approved)
	}
	if !strings.Contains(report.Note, "inferred") {
		t.Errorf("the report does not say the approval is inferred: %q", report.Note)
	}
}
