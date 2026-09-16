package tune

import (
	"strings"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/state"
)

// These cover the edges scripts/mutate could change in this package
// without a test disagreeing. tune prints a command for a person to run
// against their own guard, so a boundary that is one out here suggests a
// threshold that is one out.

func f(v float64) *float64 { return &v }
func iptr(v int) *int      { return &v }

// Every trigger the gate records has to be readable back. Two of them,
// the seven day window and session duration, had no test at all: the
// condition that reads them could be inverted and nothing noticed,
// because nothing ever asked for those two.
func TestEveryMeasurementCanBeReadBack(t *testing.T) {
	full := state.GateOutcome{
		ContextPct:      f(81),
		FiveHourPct:     f(62),
		SevenDayPct:     f(43),
		DurationMinutes: iptr(215),
		ActiveSubagents: 3,
		SubagentStarts:  7,
	}
	for trigger, want := range map[string]float64{
		"context":         81,
		"fiveHour":        62,
		"sevenDay":        43,
		"duration":        215,
		"activeSubagents": 3,
		"subagentStarts":  7,
	} {
		got, ok := measurement(full, trigger)
		if !ok {
			t.Errorf("%s was recorded and cannot be read back", trigger)
			continue
		}
		if got != want {
			t.Errorf("%s reads as %.0f, want %.0f", trigger, got, want)
		}
	}

	// A measurement the harness never sent is absent, not zero, and the
	// same four are the ones that can be absent.
	none := state.GateOutcome{}
	for _, trigger := range []string{"context", "fiveHour", "sevenDay", "duration"} {
		if v, ok := measurement(none, trigger); ok {
			t.Errorf("%s was not recorded and read back as %.0f", trigger, v)
		}
	}
	// A trigger nothing knows about is not a measurement of zero either.
	if _, ok := measurement(full, "somethingElse"); ok {
		t.Error("an unknown trigger read back as a measurement")
	}
}

// A suggestion has to move the threshold. Landing on the value it
// already has is not a suggestion, and the check for that turns on
// whether the new value is at or below the current one.
func TestASuggestionAlwaysMovesTheThreshold(t *testing.T) {
	// Everything approved, and one above the highest seen is exactly the
	// value already set. That is the edge: a suggestion of 80 when the
	// threshold is already 80 tells a person to change nothing.
	th := Threshold{Trigger: "context", Current: 80, Asks: 6, Approved: 6}
	reason, next := suggest(th, []float64{78, 79}, nil)
	if next == nil {
		t.Fatalf("no suggestion at all: %s", reason)
	}
	if *next <= th.Current {
		t.Errorf("suggested %d, which is not above the current %d", *next, th.Current)
	}
}

// A percentage cannot go past 100, and the check for that is at the
// boundary itself.
func TestAPercentageSuggestionStopsAtTheTopOfItsRange(t *testing.T) {
	// 100 is inside the range and 101 is not, so the cap is between them.
	at100 := Threshold{Trigger: "context", Current: 98, Asks: 6, Approved: 6}
	if reason, next := suggest(at100, []float64{98, 99}, nil); next == nil {
		t.Errorf("a suggestion of 100 was refused as out of range: %s", reason)
	} else if *next != 100 {
		t.Errorf("suggested %d, want 100", *next)
	}

	th := Threshold{Trigger: "context", Current: 99, Asks: 6, Approved: 6}
	reason, next := suggest(th, []float64{99, 100}, nil)
	if next != nil {
		t.Errorf("suggested %d for a percentage already at %d: %s", *next, th.Current, reason)
	}
	if !strings.Contains(reason, "near the top of its range") {
		t.Errorf("reason %q does not say why there is no suggestion", reason)
	}

	// A count has no such ceiling.
	count := Threshold{Trigger: "activeSubagents", Current: 99, Asks: 6, Approved: 6}
	if _, next := suggest(count, []float64{99, 100}, nil); next == nil {
		t.Error("a count was capped at 100 as though it were a percentage")
	}
}

// Half declined is the point where a threshold is reported as doing its
// job rather than as asking too early, and half is on the doing its job
// side.
func TestHalfDeclinedCountsAsWorking(t *testing.T) {
	half := Threshold{Trigger: "context", Current: 80, Asks: 6, Approved: 3}
	reason, next := suggest(half, []float64{81, 82, 83}, []float64{84, 85, 86})
	if next != nil {
		t.Errorf("a threshold declined half the time was told to move to %d", *next)
	}
	if !strings.Contains(reason, "doing its job") {
		t.Errorf("reason %q", reason)
	}

	// One fewer decline and it is mostly approved, which does suggest.
	mostly := Threshold{Trigger: "context", Current: 80, Asks: 6, Approved: 4}
	if _, next := suggest(mostly, []float64{81, 82, 83, 84}, []float64{95, 96}); next == nil {
		t.Error("a threshold approved two thirds of the time suggested nothing")
	}

	// Where the answer changed is the value to suggest, and if that lands
	// on the threshold already set it is not a change worth printing.
	onTheNose := Threshold{Trigger: "context", Current: 80, Asks: 6, Approved: 4}
	noseReason, noseNext := suggest(onTheNose, []float64{70, 71, 72, 73}, []float64{80, 96})
	if noseNext != nil {
		t.Errorf("the lowest decline was the current threshold and it suggested %d: %s", *noseNext, noseReason)
	}
	if !strings.Contains(noseReason, "below the current threshold") {
		t.Errorf("reason %q does not say why there is no suggestion", noseReason)
	}
}

// One of several asks takes a singular verb. The noun stays plural
// because it counts the whole set.
func TestTheVerbAgreesWithTheCount(t *testing.T) {
	for _, c := range []struct {
		n, total int
		want     string
	}{
		{1, 7, "1 of 7 asks was"},
		{2, 7, "2 of 7 asks were"},
		{0, 7, "0 of 7 asks were"},
	} {
		if got := ofAsks(c.n, c.total); got != c.want {
			t.Errorf("ofAsks(%d, %d) is %q, want %q", c.n, c.total, got, c.want)
		}
	}
}

// Two thresholds asked the same number of times have to come out in the
// same order every time, or the same observations print differently on
// two runs and a reader cannot tell the report changed from the data
// changing.
func TestTheOrderOfAReportIsDecidedByTheData(t *testing.T) {
	want := []string{"context", "fiveHour", "activeSubagents", "duration", "subagentStarts", "sevenDay"}
	asks := map[string]int{
		"context": 9, "fiveHour": 9,
		"activeSubagents": 4, "duration": 4, "subagentStarts": 4,
		"sevenDay": 1,
	}

	// The same set in several starting orders has to come out the same
	// way every time. One fixed input can land on the right answer by
	// accident, which is how a comparator that does not order ties at
	// all can look correct.
	starts := [][]string{
		{"subagentStarts", "context", "activeSubagents", "fiveHour", "sevenDay", "duration"},
		{"sevenDay", "duration", "subagentStarts", "activeSubagents", "fiveHour", "context"},
		{"context", "fiveHour", "activeSubagents", "duration", "subagentStarts", "sevenDay"},
		{"duration", "sevenDay", "fiveHour", "subagentStarts", "context", "activeSubagents"},
	}
	for _, start := range starts {
		var in []Threshold
		for _, name := range start {
			in = append(in, Threshold{Trigger: name, Asks: asks[name]})
		}
		sortThresholds(in)

		var got []string
		for _, th := range in {
			got = append(got, th.Trigger)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("from %v the order is %v, want %v", start, got, want)
			}
		}
	}
}

// sort requires that nothing is less than itself. A comparator that says
// otherwise is invalid, and the order it produces is then not defined,
// even though it usually looks right. That cannot be seen by checking a
// sorted list, so it is asked here directly.
func TestNoThresholdIsLessThanItself(t *testing.T) {
	for _, th := range []Threshold{
		{Trigger: "context", Asks: 9},
		{Trigger: "fiveHour", Asks: 0},
		{Trigger: "duration", Asks: 4},
	} {
		if lessThreshold(th, th) {
			t.Errorf("%s is reported as less than itself, which sort does not allow", th.Trigger)
		}
	}

	// And two rows cannot both be less than the other.
	a := Threshold{Trigger: "context", Asks: 9}
	b := Threshold{Trigger: "fiveHour", Asks: 9}
	if lessThreshold(a, b) && lessThreshold(b, a) {
		t.Error("two rows are each less than the other")
	}
}
