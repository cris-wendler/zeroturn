package policy

import (
	"strings"
	"testing"
	"time"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/state"
)

func at(v float64) *float64 { return &v }

// measuredSession carries a reading for everything the gate can read, so
// nothing is absent and Unmeasured has to be empty.
func measuredSession() state.Session {
	var s state.Session
	s.ContextPct = at(10)
	s.FiveHourPct = at(5)
	s.SevenDayPct = at(2)
	ms := int64(60000)
	s.DurationMS = &ms
	return s
}

// The field was declared as "false when the harness did not supply the
// measurement" and nothing ever set it false, so it could only ever say
// one thing. This is the assertion that it now says both.
func TestAvailableIsTrueForWhatWasReadAndFalseForWhatWasNot(t *testing.T) {
	c := config.Default()
	c.Guard.Mode = config.ModeConfirm

	var s state.Session
	s.ContextPct = at(95) // crosses, and is the only thing measured
	s.ActiveSubagents = 3

	r := EvaluateAt(NewGuard(c, false), s, time.Now())

	if len(r.Triggers) == 0 {
		t.Fatal("nothing was crossed, so this test checks nothing")
	}
	for _, x := range r.Triggers {
		if !x.Available {
			t.Errorf("a crossed threshold reports available false: %+v", x)
		}
		if x.Observed == nil {
			t.Errorf("a crossed threshold reports no observation: %+v", x)
		}
	}

	if len(r.Unmeasured) == 0 {
		t.Fatal("three values were absent and none is reported")
	}
	for _, x := range r.Unmeasured {
		if x.Available {
			t.Errorf("an unmeasured threshold reports available true: %+v", x)
		}
		// There was no reading. Zero would be one nobody took.
		if x.Observed != nil {
			t.Errorf("an unmeasured threshold reports an observation: %+v", x)
		}
		if x.Limit == 0 {
			t.Errorf("an unmeasured threshold reports no limit, and the limit is configured: %+v", x)
		}
		if x.Text == "" {
			t.Errorf("an unmeasured threshold says nothing: %+v", x)
		}
	}
}

// The whole risk of this change. A threshold nobody measured was not
// crossed, so it must not move the level, the decision, or the sentence
// the developer is shown. Reason reads the first two entries of whatever
// it is given, so an unmeasured entry in that slice would put a reading
// nobody took into the prompt.
func TestUnmeasuredThresholdsChangeNoDecision(t *testing.T) {
	for _, mode := range []string{config.ModeObserve, config.ModeConfirm, config.ModeStrict} {
		c := config.Default()
		c.Guard.Mode = mode

		full := measuredSession()
		full.ActiveSubagents = 3

		// The same session with three readings taken away. The gate sees
		// less, so it can only ever decide the same or less.
		partial := full
		partial.FiveHourPct = nil
		partial.SevenDayPct = nil
		partial.DurationMS = nil

		a := EvaluateAt(NewGuard(c, true), full, time.Now())
		b := EvaluateAt(NewGuard(c, true), partial, time.Now())

		if len(b.Unmeasured) != 3 {
			t.Fatalf("%s: three readings were removed and %d are reported", mode, len(b.Unmeasured))
		}
		if a.Level != b.Level {
			t.Errorf("%s: level changed from %q to %q because readings were absent", mode, a.Level, b.Level)
		}
		if a.Decision != b.Decision {
			t.Errorf("%s: decision changed from %q to %q because readings were absent", mode, a.Decision, b.Decision)
		}
		if a.Reason != b.Reason {
			t.Errorf("%s: the reason changed from %q to %q", mode, a.Reason, b.Reason)
		}
		for _, x := range b.Unmeasured {
			if strings.Contains(b.Reason, x.Text) {
				t.Errorf("%s: an unmeasured threshold reached the developer's prompt: %q", mode, b.Reason)
			}
		}
	}
}

// triggers has to keep meaning what it meant, because a reader counting
// it to ask whether anything fired has been doing that since 1.0.
func TestNothingCrossedStillMeansNoTriggers(t *testing.T) {
	c := config.Default()
	var nothing state.Session // no readings at all, the extension case

	r := EvaluateAt(NewGuard(c, false), nothing, time.Now())
	if len(r.Triggers) != 0 {
		t.Errorf("nothing was crossed and triggers holds %d entries", len(r.Triggers))
	}
	if r.Level != LevelOK || r.Decision != DecisionAllow {
		t.Errorf("a session with no readings decided %q at level %q", r.Decision, r.Level)
	}
	if len(r.Unmeasured) != len(measurements) {
		t.Errorf("nothing was measured and %d of %d are reported", len(r.Unmeasured), len(measurements))
	}
}

// The other direction: everything was read, so there is nothing to
// report as absent, and saying otherwise would be noise on the path
// every terminal session takes.
func TestAFullyMeasuredSessionReportsNothingUnmeasured(t *testing.T) {
	r := EvaluateAt(NewGuard(config.Default(), false), measuredSession(), time.Now())
	if len(r.Unmeasured) != 0 {
		t.Errorf("every value was read and %d are reported as absent: %+v", len(r.Unmeasured), r.Unmeasured)
	}
}

// Every name it publishes has to be one the gate would use for the same
// threshold, or a reader cannot join the two lists.
func TestAnUnmeasuredEntryUsesTheNameACrossedOneWouldHave(t *testing.T) {
	c := config.Default()
	crossed := map[string]bool{}

	// Drive each measurement past its limit on its own.
	for _, s := range []state.Session{
		func() state.Session { var s state.Session; s.ContextPct = at(99); return s }(),
		func() state.Session { var s state.Session; s.FiveHourPct = at(99); return s }(),
		func() state.Session { var s state.Session; s.SevenDayPct = at(99); return s }(),
		func() state.Session {
			var s state.Session
			ms := int64(99 * 60 * 60000)
			s.DurationMS = &ms
			return s
		}(),
	} {
		for _, x := range EvaluateAt(NewGuard(c, false), s, time.Now()).Triggers {
			crossed[x.Name] = true
		}
	}

	var nothing state.Session
	for _, x := range EvaluateAt(NewGuard(c, false), nothing, time.Now()).Unmeasured {
		if !crossed[x.Name] {
			t.Errorf("unmeasured publishes %q, which is not a name the gate uses when that threshold is crossed", x.Name)
		}
	}
}
