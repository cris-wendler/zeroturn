package policy

import (
	"testing"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/state"
)

// critical is a session that crosses a hard threshold, which is the only
// condition Strict mode denies.
func critical() state.Session { return state.Session{ContextPct: f(95)} }

// The rule the type exists to carry: no guard built without approval can
// be in Strict mode, whatever the repository file asked for. This is the
// check that a sixth call site cannot forget, because there is no way to
// reach Evaluate without going through NewGuard.
func TestNoUnapprovedGuardIsStrict(t *testing.T) {
	for _, mode := range []string{config.ModeObserve, config.ModeConfirm, config.ModeStrict} {
		if g := NewGuard(modeConfig(mode), false); g.Mode() == config.ModeStrict {
			t.Errorf("%s: unapproved guard is in strict mode", mode)
		}
	}
	if (Guard{}).Mode() == config.ModeStrict {
		t.Error("the zero guard is in strict mode")
	}
}

func TestStrictAsksUntilTheMachineApprovesIt(t *testing.T) {
	g := NewGuard(modeConfig(config.ModeStrict), false)
	if g.Mode() != config.ModeConfirm {
		t.Fatalf("mode %s, want confirm", g.Mode())
	}
	if r := Evaluate(g, critical()); r.Decision != DecisionAsk {
		t.Fatalf("got %s, want ask: a committed file must not deny for everyone who clones it", r.Decision)
	}
}

func TestApprovedStrictDenies(t *testing.T) {
	g := NewGuard(modeConfig(config.ModeStrict), true)
	if g.Mode() != config.ModeStrict {
		t.Fatalf("mode %s, want strict", g.Mode())
	}
	if r := Evaluate(g, critical()); r.Decision != DecisionDeny {
		t.Fatalf("got %s, want deny", r.Decision)
	}
}

// Approval decides one thing only. A repository that asked for Confirm
// must not become stricter because Strict was approved for it once.
func TestApprovalChangesNothingOutsideStrict(t *testing.T) {
	for _, mode := range []string{config.ModeObserve, config.ModeConfirm} {
		withApproval := NewGuard(modeConfig(mode), true)
		without := NewGuard(modeConfig(mode), false)
		if withApproval.Mode() != mode || without.Mode() != mode {
			t.Errorf("%s: became %s approved and %s unapproved", mode, withApproval.Mode(), without.Mode())
		}
	}
}

// Everything that reads settings through the guard sees the downgrade,
// not the mode the file asked for.
func TestConfigCarriesTheDowngrade(t *testing.T) {
	c := modeConfig(config.ModeStrict)
	if got := NewGuard(c, false).Config().Guard.Mode; got != config.ModeConfirm {
		t.Fatalf("Config reports %s, want confirm", got)
	}
	if c.Guard.Mode != config.ModeStrict {
		t.Fatal("NewGuard changed the configuration it was given")
	}
}
