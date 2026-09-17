// Package evidence records what a validation run proved, and about which
// code it proved it.
//
// Until this existed, a run left behind a count. The session record held
// directValidations, one higher than before, which says that something
// was validated and nothing about what, or whether the answer still
// holds. A developer who ran the checks an hour and four commits ago had
// the same record as one who ran them a moment ago.
//
// So a run now writes down the state of the repository it ran against,
// and a later reading compares that state with the one on disk. Evidence
// that no longer matches the code is stale, and saying so is the whole
// purpose of the record.
//
// What is kept is deliberately narrow, for the same reason the session
// record is: step names, counts, times, and digests. No output, because
// output carries whatever the tools printed, and no source, because
// source is what the digest stands in for.
package evidence

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/cris-wendler/zeroturn/internal/snapshot"
	"github.com/cris-wendler/zeroturn/internal/state"
	"github.com/cris-wendler/zeroturn/internal/verify"
)

// SchemaVersion is the stored record's format. A record written by a
// build that used a different one is not read, and not deleted: an older
// ZeroTurn's evidence is not this build's to interpret, and the answer
// when it cannot be read is that there is no evidence, which is true.
const SchemaVersion = 1

// Result is what a completed run concluded. These are stored values, so
// they are written here once and read everywhere.
const (
	// ResultNeverRun is not stored. It is what an assessment reports when
	// there is no record at all.
	ResultNeverRun = "never-run"
	// ResultRunning is stored before the steps start and replaced when
	// they finish. A record still holding it describes a run that never
	// reported back.
	ResultRunning   = "running"
	ResultPassed    = "passed"
	ResultFailed    = "failed"
	ResultCancelled = "cancelled"
)

// State is the single word for a person, derived from the result and from
// whether the repository still matches. The two are kept apart in the
// record on purpose: a failure that has gone stale is still a failure,
// and a state word that only said "stale" would hide it.
const (
	StateNeverRun  = "never-run"
	StateRunning   = "running"
	StatePassed    = "passed"
	StateFailed    = "failed"
	StateCancelled = "cancelled"
	StateStale     = "stale"
)

// Step is the safe summary of one validation step. The command is not
// here because the plan digest identifies it, and the output is not here
// because output carries whatever the tool printed, which can include a
// credential. LogName is the file inside the repository's Git directory
// that holds the full output, named without its directory so that no
// absolute path is stored.
type Step struct {
	Name     string  `json:"name"`
	Status   string  `json:"status"`
	ExitCode int     `json:"exitCode"`
	Seconds  float64 `json:"seconds"`
	LogName  string  `json:"logName,omitempty"`
}

// Record is one validation run and the repository state it ran against.
type Record struct {
	SchemaVersion int    `json:"schemaVersion"`
	EvidenceID    string `json:"evidenceId"`
	RepoHash      string `json:"repoHash"`
	ZeroTurnVer   string `json:"zeroturnVersion"`

	Snapshot snapshot.Snapshot `json:"snapshot"`

	StartedAt  time.Time  `json:"startedAt"`
	FinishedAt *time.Time `json:"finishedAt,omitempty"`

	Result  string  `json:"result"`
	Passed  int     `json:"passed"`
	Failed  int     `json:"failed"`
	Skipped int     `json:"skipped"`
	Seconds float64 `json:"seconds"`

	Steps []Step `json:"steps"`
}

// Assessment answers what a reader wants to know: does validation cover
// the code that is here now, and if not, why not.
//
// Result and Current are kept separate from State so that a machine
// reading this is never forced to work out from one word both what the
// run concluded and whether it still applies.
type Assessment struct {
	State   string `json:"state"`
	Result  string `json:"result"`
	Current bool   `json:"current"`
	// Changed names each input that differs from the recorded one. It is
	// empty when the evidence is current, and when there is none.
	Changed []string `json:"changed,omitempty"`
	Reason  string   `json:"reason"`
	// Evidence is the record this assessment is about, absent when no
	// validation has been recorded.
	Evidence *Record `json:"evidence,omitempty"`
}

// Dir is where evidence is kept, beside the session records rather than
// inside the repository, so that uninstall removes it with everything
// else and a repository never carries ZeroTurn's own files.
func Dir(st *state.Store) string { return filepath.Join(st.Dir(), "evidence") }

func Path(st *state.Store, repoHash string) string {
	return filepath.Join(Dir(st), repoHash+".json")
}

// ID names one run. It is derived rather than random so that the same run
// cannot be written under two names by a retry.
func ID(repoHash, digest string, startedAt time.Time) string {
	h := sha256.Sum256([]byte(repoHash + "\n" + digest + "\n" + startedAt.UTC().Format(time.RFC3339Nano)))
	return hex.EncodeToString(h[:])[:16]
}

// Begin records that a run started, before it has a result.
//
// A run that is killed halfway leaves this behind, and a record saying a
// run started and never reported is the truth about that repository. The
// alternative, writing only at the end, reports an interrupted run as if
// it had never happened, which is how a developer comes to believe the
// last successful run is the last run.
func Begin(st *state.Store, repoHash, version string, snap snapshot.Snapshot, now time.Time) (Record, error) {
	r := Record{
		SchemaVersion: SchemaVersion,
		EvidenceID:    ID(repoHash, snap.Digest, now),
		RepoHash:      repoHash,
		ZeroTurnVer:   version,
		Snapshot:      snap,
		StartedAt:     now.UTC(),
		Result:        ResultRunning,
		Steps:         []Step{},
	}
	return r, Save(st, r)
}

// Complete replaces the started record with what the run concluded. The
// snapshot is the one taken before the steps ran: it is the state the
// result is evidence about, and taking it again now would credit the run
// with any change the steps themselves made.
func Complete(st *state.Store, r Record, res verify.Result, now time.Time) (Record, error) {
	finished := now.UTC()
	r.FinishedAt = &finished
	r.Passed, r.Failed, r.Skipped, r.Seconds = res.Passed, res.Failed, res.Skipped, res.Seconds
	r.Result = resultOf(res)
	r.Steps = make([]Step, 0, len(res.Steps))
	for _, s := range res.Steps {
		r.Steps = append(r.Steps, Step{
			Name:     s.Name,
			Status:   s.Status,
			ExitCode: s.ExitCode,
			Seconds:  s.Seconds,
			LogName:  logName(s.LogFile),
		})
	}
	return r, Save(st, r)
}

// logName keeps the file name and drops the directory it is in, because
// the directory is an absolute path into somebody's machine.
func logName(p string) string {
	if p == "" {
		return ""
	}
	return filepath.Base(p)
}

func resultOf(res verify.Result) string {
	switch {
	case res.Cancelled:
		return ResultCancelled
	case res.Failed > 0:
		return ResultFailed
	default:
		return ResultPassed
	}
}

// contendedAttempts bounds how long a read or a write waits out another
// process that holds the record open. See retry for what it is for.
const contendedAttempts = 25

// retry runs an operation that can fail only because another process is
// touching the same record at that instant.
//
// The record is replaced by renaming a new file over it, which means a
// reader sees the whole of one record or the whole of the one before.
// That much holds everywhere. What does not hold everywhere is that the
// operation succeeds at all: on Windows a file being replaced cannot be
// opened, and a file being read cannot be replaced, so an open or a
// rename that lands inside the other's window fails outright rather than
// returning either version.
//
// This is not only a test condition. zeroturn report reads the record
// while zeroturn verify writes it, and the report would then drop its
// validation section with nothing said. Windows continuous integration
// found it with eight writers and fifty readers.
//
// The window is measured in microseconds, so a short bounded wait closes
// it. Something genuinely unreadable still fails, a few milliseconds
// later rather than never.
func retry(op func() error) error {
	var err error
	for i := 0; i < contendedAttempts; i++ {
		err = op()
		if err == nil || os.IsNotExist(err) {
			return err
		}
		time.Sleep(200 * time.Microsecond)
	}
	return err
}

func Save(st *state.Store, r Record) error {
	if err := os.MkdirAll(Dir(st), 0700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	data := append(b, '\n')
	return retry(func() error {
		return state.AtomicWrite(Path(st, r.RepoHash), data, 0600)
	})
}

// PurgeAll removes every evidence record on this machine.
//
// Evidence is kept until it is replaced or removed, rather than for the
// retention window session records use: there is one record per
// repository and it is the answer to a question somebody may ask at any
// time, so expiring it by age would mean forgetting whether the last run
// still holds. The promise that purging removes every ZeroTurn record has
// to cover it all the same.
func PurgeAll(st *state.Store) (int, error) {
	entries, err := ioutil.ReadDir(Dir(st))
	if os.IsNotExist(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			continue
		}
		if os.Remove(filepath.Join(Dir(st), e.Name())) == nil {
			n++
		}
	}
	return n, nil
}

func Load(st *state.Store, repoHash string) (Record, bool, error) {
	var b []byte
	err := retry(func() error {
		var rerr error
		b, rerr = ioutil.ReadFile(Path(st, repoHash))
		return rerr
	})
	if os.IsNotExist(err) {
		return Record{}, false, nil
	}
	if err != nil {
		return Record{}, false, err
	}
	var r Record
	if json.Unmarshal(b, &r) != nil || r.SchemaVersion != SchemaVersion {
		return Record{}, false, nil
	}
	return r, true, nil
}

// Assess compares recorded evidence with the repository as it is now.
func Assess(r Record, found bool, now snapshot.Snapshot) Assessment {
	if !found {
		return Assessment{
			State: StateNeverRun, Result: ResultNeverRun, Current: false,
			Reason: "No validation has been recorded for this repository. " +
				"Run zeroturn verify to record one.",
		}
	}

	rec := r
	a := Assessment{Result: r.Result, Evidence: &rec}
	a.Changed = r.Snapshot.Differences(now)
	a.Current = len(a.Changed) == 0

	// A run that never reported back is neither current nor stale. It did
	// not finish, so there is no result to have gone out of date.
	if r.Result == ResultRunning {
		a.State, a.Current, a.Changed = StateRunning, false, nil
		a.Reason = "A validation run started and did not record a result. " +
			"It may still be running, or it was interrupted. " +
			"Run zeroturn verify again for the current code."
		return a
	}

	if a.Current {
		switch r.Result {
		case ResultPassed:
			a.State = StatePassed
			a.Reason = "Validation passed for the code that is here now."
		case ResultFailed:
			a.State = StateFailed
			a.Reason = "Validation failed for the code that is here now. " +
				"Fix the failure, then run zeroturn verify again."
		default:
			a.State = StateCancelled
			a.Reason = "The last validation run was cancelled before it finished. " +
				"Run zeroturn verify again."
		}
		return a
	}

	a.State = StateStale
	why := snapshot.Describe(a.Changed)
	// A failure that has gone stale is still a failure. Saying only that
	// the evidence is out of date would read as though there were a
	// result waiting to be refreshed, when what is waiting is a fix.
	if r.Result == ResultFailed {
		a.Reason = "The last validation run failed, and " + why + " since. " +
			"Run zeroturn verify again for the current code."
		return a
	}
	if r.Result == ResultCancelled {
		a.Reason = "The last validation run was cancelled, and " + why + " since. " +
			"Run zeroturn verify again for the current code."
		return a
	}
	a.Reason = "Validation is stale because " + why + " after the last successful run. " +
		"Run zeroturn verify again for the current code."
	return a
}
