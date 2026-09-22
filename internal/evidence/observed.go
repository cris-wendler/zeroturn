package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"time"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/snapshot"
	"github.com/cris-wendler/zeroturn/internal/state"
	"github.com/cris-wendler/zeroturn/internal/verify"
)

// Evidence is normally written by verify, which runs every step itself
// and knows they all passed. A step can also be observed: a coding
// harness reports the commands it ran, and one of them may be a step
// from this repository's plan.
//
// Nobody ran verify in this repository for five days while fifteen
// changes were merged, because it wraps commands a developer runs
// anyway and adds a second thing to remember. Watching for those
// commands makes the record a by-product of the work rather than an
// errand. What it must not do is claim more than was seen.

// MatchKind is what an observed command was to the validation plan.
type MatchKind int

const (
	// NoMatch is a command with nothing to do with the plan, which is
	// almost every command a session runs.
	NoMatch MatchKind = iota
	// Exact is the step, argument for argument. Only this records
	// evidence, because a command that differs by an argument did
	// different work, and the difference is usually narrower: one
	// package rather than every package.
	Exact
	// Near is the step's program and first argument with the rest
	// changed, such as go test on one package rather than on all of
	// them. It records nothing. It is reported so that somebody whose
	// commands never quite match can see why no evidence is appearing,
	// which is otherwise indistinguishable from the feature being off.
	Near
)

// Match reports what an observed command line was to the plan, and which
// step it matched when it matched one.
//
// The command arrives as the single string a harness reports rather than
// as an argument list, so it is split on spaces. That is not a shell
// parser and is not meant to be: a step whose arguments need quoting is
// not matched, which costs an unusual step its evidence and never
// invents one.
func Match(command string, steps []config.Step) (MatchKind, config.Step) {
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return NoMatch, config.Step{}
	}
	if strings.ContainsAny(command, `"'\`+"`"+`$|&;<>(){}`) {
		// A shell would read this differently from the split above, so
		// what actually ran is not known from the string alone.
		return NoMatch, config.Step{}
	}

	best := NoMatch
	var bestStep config.Step
	for _, s := range steps {
		if len(s.Command) == 0 {
			continue
		}
		if equal(fields, s.Command) {
			return Exact, s
		}
		if best == NoMatch && sameProgram(fields, s.Command) {
			best, bestStep = Near, s
		}
	}
	return best, bestStep
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// sameProgram reports that two command lines start the same way: the
// program, and the first argument when there is one. go test and go vet
// are different steps and must not read as near misses of each other.
func sameProgram(a, b []string) bool {
	if len(a) == 0 || len(b) == 0 || a[0] != b[0] {
		return false
	}
	if len(a) > 1 && len(b) > 1 {
		return a[1] == b[1]
	}
	return len(a) == 1 && len(b) == 1
}

// ResultPartial is a record assembled from observed steps where some
// step of the plan has not been seen at this state. It is stored, and it
// is deliberately not ResultPassed: the steps that were seen passed, and
// the ones that were not were not run as far as anybody here knows.
const ResultPartial = "partial"

// StatePartial is the word for a person. A partial record is not a
// failure and not a pass, and calling it either would be a claim nobody
// made.
const StatePartial = "partial"

// Observe records that one step of the plan passed at a repository
// state, and returns the record as it now stands.
//
// Evidence assembled this way is complete only when every step in the
// plan has passed at the same state. A step observed at one state says
// nothing about another, so a record for a different state is replaced
// rather than added to: the earlier steps ran against code that is no
// longer what is here.
//
// A run of verify still writes a record of its own and is not affected
// by this. Where both exist the later one wins, which is what a reader
// asking "what is known about the code here now" wants.
func Observe(st *state.Store, repoHash, version string, snap snapshot.Snapshot, step config.Step, plan []config.Step, seconds float64, now time.Time) (Record, error) {
	r, found, err := Load(st, repoHash)
	if err != nil {
		return Record{}, err
	}
	// Anything recorded against other code is not something to add to.
	if !found || r.Snapshot.Digest != snap.Digest || !observedOnly(r) {
		r = Record{
			SchemaVersion: SchemaVersion,
			RepoHash:      repoHash,
			ZeroTurnVer:   version,
			Snapshot:      snap,
			StartedAt:     now,
			Steps:         nil,
		}
	}

	replaced := false
	for i := range r.Steps {
		if r.Steps[i].Name == step.Name {
			r.Steps[i] = Step{Name: step.Name, Status: verify.StatusPass, Seconds: seconds}
			replaced = true
		}
	}
	if !replaced {
		r.Steps = append(r.Steps, Step{Name: step.Name, Status: verify.StatusPass, Seconds: seconds})
	}

	r.Passed = len(r.Steps)
	r.Seconds = 0
	for _, s := range r.Steps {
		r.Seconds += s.Seconds
	}
	finished := now
	r.FinishedAt = &finished
	r.Result = ResultPartial
	if allStepsSeen(r.Steps, plan) {
		r.Result = ResultPassed
	}
	r.EvidenceID = observedID(repoHash, snap.Digest)

	if err := Save(st, r); err != nil {
		return Record{}, err
	}
	return r, nil
}

// observedOnly reports that a record was assembled from observations
// rather than written by a run of verify. A verify record is the
// stronger statement, because it ran every step itself, so observations
// never edit one: they start a record of their own instead.
func observedOnly(r Record) bool {
	return r.Result == ResultPartial || strings.HasPrefix(r.EvidenceID, observedPrefix)
}

func allStepsSeen(steps []Step, plan []config.Step) bool {
	if len(plan) == 0 {
		return false
	}
	for _, want := range plan {
		seen := false
		for _, got := range steps {
			if got.Name == want.Name && got.Status == verify.StatusPass {
				seen = true
			}
		}
		if !seen {
			return false
		}
	}
	return true
}

// Missing names the steps of the plan that have not been seen to pass at
// this record's state, so a reader is told what is outstanding rather
// than left to compare two lists.
func Missing(r Record, plan []config.Step) []string {
	var out []string
	for _, want := range plan {
		seen := false
		for _, got := range r.Steps {
			if got.Name == want.Name && got.Status == verify.StatusPass {
				seen = true
			}
		}
		if !seen {
			out = append(out, want.Name)
		}
	}
	return out
}

const observedPrefix = "obs"

// observedID is derived from the repository and the state, so every
// observation at one state writes to one record rather than starting a
// new one each time.
func observedID(repoHash, digest string) string {
	sum := sha256.Sum256([]byte(observedPrefix + "\x00" + repoHash + "\x00" + digest))
	return observedPrefix + hex.EncodeToString(sum[:])[:13]
}
