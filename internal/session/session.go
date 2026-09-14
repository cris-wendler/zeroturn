// Package session folds harness events into the stored session record.
//
// This is the one place that decides what an event changes. It is a
// package of its own rather than a helper inside a command, because
// every entry point that reads an event depends on it agreeing: the
// gate, the status line, and the guards all read the record this fold
// produced.
package session

import (
	"time"

	"github.com/cris-wendler/zeroturn/internal/events"
	"github.com/cris-wendler/zeroturn/internal/git"
	"github.com/cris-wendler/zeroturn/internal/state"
)

// RepoHashFor reports the stored key for the repository an event came
// from, and an empty key when the directory is not inside one. It starts
// no git process, because it runs on every status line repaint.
func RepoHashFor(cwd string) string {
	if root, ok := git.FindRoot(cwd); ok {
		return state.RepoHash(root)
	}
	return ""
}

// Apply folds one normalized event into a session record. It reads only
// the fields the event carries: a measurement the harness did not send
// leaves the last known one in place, because an absent reading is not a
// reading of zero.
func Apply(s *state.Session, e events.Event) {
	ApplyAt(s, e, time.Now())
}

// ApplyAt is Apply with the current time supplied, which the five hour
// baseline needs and which tests need to control.
func ApplyAt(s *state.Session, e events.Event, now time.Time) {
	if e.Harness != "" {
		s.Harness = e.Harness
	}
	if e.HarnessVersion != "" {
		s.HarnessVer = e.HarnessVersion
	}
	if e.Model != "" {
		s.Model = e.Model
	}
	if e.ContextPct != nil {
		v := *e.ContextPct
		s.ContextPct = &v
	}
	if e.ContextSize != nil {
		v := *e.ContextSize
		s.ContextSize = &v
	}
	if e.FiveHourPct != nil {
		v := *e.FiveHourPct
		s.FiveHourPct = &v
		s.SampleFiveHour(v, e.FiveHourResetsAt, now)
	}
	if e.FiveHourResetsAt != nil {
		v := *e.FiveHourResetsAt
		s.FiveHourResetsAt = &v
	}
	if e.SevenDayPct != nil {
		v := *e.SevenDayPct
		s.SevenDayPct = &v
	}
	if e.SevenDayResetsAt != nil {
		v := *e.SevenDayResetsAt
		s.SevenDayResetsAt = &v
	}
	if e.DurationMS != nil {
		v := *e.DurationMS
		s.DurationMS = &v
	}
	if e.BackgroundTasks != nil {
		s.BackgroundTasks = *e.BackgroundTasks
	}

	switch e.Type {
	case events.TypeSubagentStrt:
		// A subagent starting after an ask is the developer having
		// approved it. It is the only outcome the harness reports.
		s.ResolveGate()
		s.AddActive(e.AgentID)
	case events.TypeSubagentStop:
		s.RemoveActive(e.AgentID)
	case events.TypeSessionStop, events.TypeSessionEnd:
		// A subagent still counted as running when a turn ends never
		// reported stopping. Clearing here keeps a later count honest
		// rather than asking about subagents that are long gone.
		s.ClearActive()
	}
}

// Record folds one event into the stored record for its session and
// reports the record as it now stands.
func Record(st *state.Store, e events.Event) (state.Session, error) {
	return st.Update(e.SessionID, RepoHashFor(e.CWD), func(s *state.Session) {
		Apply(s, e)
	})
}
