package main

import (
	"context"
	"time"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/events"
	"github.com/cris-wendler/zeroturn/internal/evidence"
	"github.com/cris-wendler/zeroturn/internal/git"
	"github.com/cris-wendler/zeroturn/internal/snapshot"
	"github.com/cris-wendler/zeroturn/internal/state"
)

// recordObservedStep records that a validation step passed, when the
// command a session ran was one.
//
// Nobody ran verify in this repository for five days while fifteen
// changes were merged, because it wraps commands a developer runs
// anyway. Watching for those commands makes the record a by-product of
// the work rather than an errand to remember.
//
// It answers nothing to the harness and returns no error. This is a hook
// on every successful command, so it must be cheap and it must never
// stop a session: a repository that cannot be read, or a state directory
// that cannot be written, leaves the record as it was.
func recordObservedStep(st *state.Store, c config.Config, e events.Event) {
	if e.Command == "" || e.CWD == "" {
		return
	}
	kind, step := evidence.Match(e.Command, c.Verify.Steps)
	if kind != evidence.Exact {
		// A near miss is not recorded. It is counted on the session so
		// that doctor can say the commands being run are close to the
		// plan without matching it, which is otherwise indistinguishable
		// from the feature doing nothing.
		if kind == evidence.Near {
			st.Update(e.SessionID, state.RepoHash(e.CWD), func(s *state.Session) {
				s.NearValidationRuns++
			})
		}
		return
	}

	// The digest is taken only for a command that matched, which is rare.
	// Taking it for every command a session runs would put the cost of
	// reading the repository behind every shell call.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	repo, err := git.Open(ctx, e.CWD)
	if err != nil {
		return
	}
	snap, err := snapshot.Compute(ctx, repo, c.Hash())
	if err != nil {
		return
	}
	if _, oerr := evidence.Observe(st, state.RepoHash(repo.Root), version(), snap, step, c.Verify.Steps, 0, time.Now()); oerr != nil {
		return
	}
	st.Update(e.SessionID, state.RepoHash(repo.Root), func(s *state.Session) {
		s.ObservedSteps++
	})
}
