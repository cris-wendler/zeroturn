package session

import (
	"testing"
	"time"

	"github.com/cris-wendler/zeroturn/internal/events"
	"github.com/cris-wendler/zeroturn/internal/state"
)

func f(v float64) *float64 { return &v }
func i(v int) *int         { return &v }
func i64(v int64) *int64   { return &v }

// An absent reading is not a reading of zero. A status line arriving
// without the five hour window, which is what the harness sends before
// the first API call of a session, must not erase what is known.
func TestAbsentMeasurementsLeaveTheRecordAlone(t *testing.T) {
	s := state.Session{
		Harness: "claude", Model: "opus",
		ContextPct: f(70), FiveHourPct: f(40), SevenDayPct: f(30), DurationMS: i64(60000),
	}
	Apply(&s, events.Event{Type: events.TypeStatus})

	if s.Harness != "claude" || s.Model != "opus" {
		t.Errorf("identity overwritten: harness %q model %q", s.Harness, s.Model)
	}
	// The value is checked, not only its presence. A fold that wrote the
	// zero value of a missing reading would leave a field that is there
	// and wrong, which is worse than one that is gone.
	for name, c := range map[string]struct {
		got  *float64
		want float64
	}{
		"context":  {s.ContextPct, 70},
		"fiveHour": {s.FiveHourPct, 40},
		"sevenDay": {s.SevenDayPct, 30},
	} {
		if c.got == nil {
			t.Errorf("%s was erased by an event that did not carry it", name)
		} else if *c.got != c.want {
			t.Errorf("%s became %v, want %v: an absent reading is not a reading of zero", name, *c.got, c.want)
		}
	}
	if s.DurationMS == nil || *s.DurationMS != 60000 {
		t.Errorf("duration became %v, want 60000", s.DurationMS)
	}
}

func TestMeasurementsAreCopiedNotShared(t *testing.T) {
	pct := 55.0
	e := events.Event{Type: events.TypeStatus, ContextPct: &pct}
	var s state.Session
	Apply(&s, e)
	pct = 99
	if s.ContextPct == nil || *s.ContextPct != 55 {
		t.Fatalf("the record follows the event value: %v", s.ContextPct)
	}
}

func TestSubagentLifecycle(t *testing.T) {
	var s state.Session
	Apply(&s, events.Event{Type: events.TypeSubagentStrt, AgentID: "a"})
	Apply(&s, events.Event{Type: events.TypeSubagentStrt, AgentID: "b"})
	if s.ActiveSubagents != 2 || s.SubagentStarts != 2 {
		t.Fatalf("after two starts: active %d starts %d", s.ActiveSubagents, s.SubagentStarts)
	}
	Apply(&s, events.Event{Type: events.TypeSubagentStop, AgentID: "a"})
	if s.ActiveSubagents != 1 {
		t.Fatalf("after one stop: active %d", s.ActiveSubagents)
	}
	// A subagent cannot outlive the turn that started it, so anything
	// still counted when the turn ends never reported stopping.
	Apply(&s, events.Event{Type: events.TypeSessionStop})
	if s.ActiveSubagents != 0 || len(s.ActiveIDs) != 0 {
		t.Fatalf("a turn ending left %d subagents active", s.ActiveSubagents)
	}
	if s.SubagentStarts != 2 {
		t.Errorf("starts are the record of what happened, got %d", s.SubagentStarts)
	}
}

// A subagent starting after an ask is the only report the harness makes
// of the developer having approved it.
func TestAStartResolvesAWaitingAsk(t *testing.T) {
	var s state.Session
	s.RecordGate("ask", []string{"context"})
	Apply(&s, events.Event{Type: events.TypeSubagentStrt, AgentID: "a"})
	if len(s.GateOutcomes) != 1 || !s.GateOutcomes[0].Approved {
		t.Fatalf("the waiting ask was not resolved: %+v", s.GateOutcomes)
	}
}

// The baseline the rate projection measures from is taken when the first
// reading of a window arrives, and is not moved by later readings of the
// same window.
func TestTheFirstReadingOfAWindowBecomesTheBaseline(t *testing.T) {
	start := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
	reset := i64(start.Add(3 * time.Hour).Unix())
	var s state.Session

	ApplyAt(&s, events.Event{Type: events.TypeStatus, FiveHourPct: f(20), FiveHourResetsAt: reset}, start)
	if s.FiveHourBasePct == nil || *s.FiveHourBasePct != 20 {
		t.Fatalf("baseline %v, want 20", s.FiveHourBasePct)
	}
	if s.FiveHourBaseAt == nil || !s.FiveHourBaseAt.Equal(start) {
		t.Fatalf("baseline taken at %v, want %v", s.FiveHourBaseAt, start)
	}

	ApplyAt(&s, events.Event{Type: events.TypeStatus, FiveHourPct: f(45), FiveHourResetsAt: reset}, start.Add(time.Hour))
	if *s.FiveHourBasePct != 20 {
		t.Fatalf("baseline moved to %v within the same window", *s.FiveHourBasePct)
	}
	if s.FiveHourPct == nil || *s.FiveHourPct != 45 {
		t.Fatalf("current reading %v, want 45", s.FiveHourPct)
	}
}

func TestBackgroundTasksFollowTheReport(t *testing.T) {
	s := state.Session{BackgroundTasks: 3}
	Apply(&s, events.Event{Type: events.TypeStatus, BackgroundTasks: i(0)})
	if s.BackgroundTasks != 0 {
		t.Fatalf("a report of none left %d: zero is a reading here, not an absence", s.BackgroundTasks)
	}
}

func TestRepoHashIsEmptyOutsideARepository(t *testing.T) {
	if h := RepoHashFor(t.TempDir()); h != "" {
		t.Fatalf("a directory outside a repository produced the key %q", h)
	}
}
