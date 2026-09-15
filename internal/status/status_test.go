package status

import (
	"io/ioutil"
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
	return Render(Line{Session: s, Result: policy.Evaluate(policy.NewGuard(c, true), s), Color: output.Color{}})
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

// The helper above builds its own disabled Color, so this used to prove
// only that a disabled Color stays disabled. What decides is
// output.NewColor, and it has to see a destination that is not a
// terminal, which is what a pipe, a log file and a harness reading the
// output all are.
func TestNoColorWithoutTerminal(t *testing.T) {
	// A harness name forces colour on, so it has to be out of the way
	// for the half that asks whether the destination is a terminal.
	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "xterm")

	sink, err := ioutil.TempFile(t.TempDir(), "not-a-terminal")
	if err != nil {
		t.Fatal(err)
	}
	defer sink.Close()

	c := config.Default()
	c.Guard.Mode = config.ModeStrict
	sess := state.Session{ContextPct: f(95)}
	got := Render(Line{
		Session: sess,
		Result:  policy.Evaluate(policy.NewGuard(c, true), sess),
		Color:   output.NewColor(sink, ""),
	})
	if strings.Contains(got, "\x1b[") {
		t.Fatalf("escape codes went to a file: %q", got)
	}

	// And the same line does carry colour where colour belongs, or the
	// test above would pass for a build that never colours anything.
	coloured := Render(Line{
		Session: sess,
		Result:  policy.Evaluate(policy.NewGuard(c, true), sess),
		// A harness renders the output itself, which is the one case
		// colour is kept for a destination that is not a terminal.
		Color: output.NewColor(sink, "claude"),
	})
	if !strings.Contains(coloured, "\x1b[") {
		t.Fatalf("an enabled colour produced no escape codes: %q", coloured)
	}
}

// A measurement the harness did not send is left out, never shown as
// zero. This is where that claim is observable: inside internal/policy a
// zero reading and an absent one produce the same empty trigger list,
// because every threshold is a percentage of at least 1.
func TestAMeasurementTheHarnessDidNotSendIsNotShownAsZero(t *testing.T) {
	absent := render(config.ModeObserve, state.Session{SubagentStarts: 1})
	for _, label := range []string{"ctx", "5h", "7d", "session"} {
		if strings.Contains(absent, label) {
			t.Errorf("a measurement that never arrived is shown as %q: %s", label, absent)
		}
	}

	// A real zero is a measurement, and it is shown, which is what makes
	// the check above mean something.
	measured := render(config.ModeObserve, state.Session{
		ContextPct: f(0), FiveHourPct: f(0), SevenDayPct: f(0), DurationMS: i64(0), SubagentStarts: 1,
	})
	for _, want := range []string{"ctx 0%", "5h 0%", "7d 0%"} {
		if !strings.Contains(measured, want) {
			t.Errorf("a measured zero is not shown as %q: %s", want, measured)
		}
	}
}
