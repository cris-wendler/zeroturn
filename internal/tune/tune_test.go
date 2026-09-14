package tune

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
	r := Analyse(c, []state.Session{sessionWith("repo", outcomes...)}, "repo")
	if r.Asks != 5 || r.Approved != 5 {
		t.Fatalf("counts %+v", r)
	}
	if len(r.Thresholds) != 1 {
		t.Fatalf("tunings %+v", r.Thresholds)
	}
	got := r.Thresholds[0]
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
	r := Analyse(c, []state.Session{sessionWith("repo", outcomes...)}, "repo")
	got := r.Thresholds[0]
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
	r := Analyse(c, []state.Session{sessionWith("repo", outcomes...)}, "repo")
	got := r.Thresholds[0]
	if got.Suggested == nil || *got.Suggested != 92 {
		t.Fatalf("suggested %v, want the value where the answer changed", got.Suggested)
	}
}

func TestTuningStaysQuietWithoutEnoughObservations(t *testing.T) {
	c := config.Default()
	outcomes := []state.GateOutcome{ask("context", 82, true), ask("context", 84, true)}
	r := Analyse(c, []state.Session{sessionWith("repo", outcomes...)}, "repo")
	if r.Thresholds[0].Suggested != nil {
		t.Fatal("suggested a change from two observations")
	}
	if !strings.Contains(r.Thresholds[0].Reason, "too few") {
		t.Fatalf("reason %q", r.Thresholds[0].Reason)
	}
}

func TestTuningIgnoresOtherRepositories(t *testing.T) {
	c := config.Default()
	mine := sessionWith("mine", ask("context", 82, true))
	other := sessionWith("other", ask("context", 99, false), ask("context", 98, false))
	r := Analyse(c, []state.Session{mine, other}, "mine")
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
	r := Analyse(c, []state.Session{sessionWith("repo", outcomes...)}, "repo")
	if r.Denied != 2 || r.Asks != 0 || len(r.Thresholds) != 0 {
		t.Fatalf("%+v", r)
	}
}

// The JSON names are what zeroturn policy tune --json publishes, and
// schemas/policy-tune.schema.json refuses any name it does not list. The
// conformance run only ever sees an empty list of thresholds, so the
// names inside one are pinned here.
func TestTheJSONNamesOfAThresholdAreTheContract(t *testing.T) {
	c := config.Default()
	var outcomes []state.GateOutcome
	for _, v := range []float64{76, 78, 79, 77, 81} {
		outcomes = append(outcomes, ask("fiveHour", v, true))
	}
	b, err := json.Marshal(Analyse(c, []state.Session{sessionWith("repo", outcomes...)}, "repo"))
	if err != nil {
		t.Fatal(err)
	}
	var back map[string]json.RawMessage
	if err := json.Unmarshal(b, &back); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"sessionsObserved", "asks", "approvedAfterAsking", "denied", "thresholds", "generatedAt", "note"} {
		if _, ok := back[name]; !ok {
			t.Errorf("the report no longer publishes %q", name)
		}
	}
	var list []map[string]json.RawMessage
	if err := json.Unmarshal(back["thresholds"], &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 {
		t.Fatalf("thresholds: %d", len(list))
	}
	want := map[string]bool{"trigger": true, "setting": true, "current": true,
		"asks": true, "approved": true, "suggested": true, "reason": true}
	for name := range list[0] {
		if !want[name] {
			// The schema sets additionalProperties false, so a name it
			// does not list makes the output invalid for every adapter.
			t.Errorf("a threshold publishes %q, which the schema does not allow", name)
		}
		delete(want, name)
	}
	for name := range want {
		t.Errorf("a threshold no longer publishes %q", name)
	}
}
