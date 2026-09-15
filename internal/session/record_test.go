package session

import (
	"testing"
	"time"

	"github.com/cris-wendler/zeroturn/internal/events"
	"github.com/cris-wendler/zeroturn/internal/state"
)

// The fold records what an event carries and keeps what it does not.
// Nothing failed when those guards were inverted, so a build that stopped
// recording the harness, or that overwrote a known one with nothing,
// would have gone unnoticed.
func TestTheFoldRecordsWhatAnEventCarries(t *testing.T) {
	var s state.Session
	now := time.Now().UTC()

	ApplyAt(&s, events.Event{
		Type: events.TypeStatus, Harness: "claude",
		HarnessVersion: "2.1.270", Model: "a-model",
	}, now)

	if s.Harness != "claude" {
		t.Errorf("harness is %q, want the one the event carried", s.Harness)
	}
	if s.HarnessVer != "2.1.270" {
		t.Errorf("harness version is %q, want the one the event carried", s.HarnessVer)
	}
	if s.Model != "a-model" {
		t.Errorf("model is %q, want the one the event carried", s.Model)
	}
}

// A later event that carries none of them leaves what is already known
// alone. Overwriting with nothing would lose the harness partway through
// a session, and every report of it afterwards.
func TestAnEventCarryingNothingOverwritesNothing(t *testing.T) {
	s := state.Session{Harness: "claude", HarnessVer: "2.1.270", Model: "a-model"}
	now := time.Now().UTC()

	ApplyAt(&s, events.Event{Type: events.TypeSubagentStrt}, now)

	if s.Harness != "claude" || s.HarnessVer != "2.1.270" || s.Model != "a-model" {
		t.Errorf("an event carrying nothing changed what was known: %+v", s)
	}
}
