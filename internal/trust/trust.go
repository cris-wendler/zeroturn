// Package trust records which repository supplied commands the user has
// approved for direct execution.
//
// Approval is bound to the canonical repository path, the configuration
// hash, and the configuration version together. Editing any verify step
// changes the hash and withdraws the approval, so a repository cannot gain
// execution rights by changing its configuration after approval.
package trust

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/state"
)

const SchemaVersion = 1

type Record struct {
	SchemaVersion int       `json:"schemaVersion"`
	RepoHash      string    `json:"repoHash"`
	ConfigHash    string    `json:"configHash"`
	ConfigVersion int       `json:"configVersion"`
	ApprovedAt    time.Time `json:"approvedAt"`
	StepCount     int       `json:"stepCount"`
}

type Status struct {
	Trusted bool
	Reason  string
}

func path(st *state.Store, repoHash string) string {
	return filepath.Join(st.TrustDir(), repoHash+".json")
}

func Check(st *state.Store, repoRoot string, c config.Config) Status {
	h := state.RepoHash(repoRoot)
	b, err := ioutil.ReadFile(path(st, h))
	if os.IsNotExist(err) {
		return Status{false, "this repository has not been approved on this machine"}
	}
	if err != nil {
		return Status{false, "the approval record could not be read"}
	}
	var r Record
	if json.Unmarshal(b, &r) != nil || r.SchemaVersion != SchemaVersion {
		return Status{false, "the approval record is not readable by this build"}
	}
	if r.RepoHash != h {
		return Status{false, "the approval record belongs to a different repository"}
	}
	if r.ConfigVersion != c.Version {
		return Status{false, "the configuration version changed since approval"}
	}
	if r.ConfigHash != c.Hash() {
		return Status{false, "the configured commands changed since approval"}
	}
	return Status{true, ""}
}

func Approve(st *state.Store, repoRoot string, c config.Config) error {
	r := Record{
		SchemaVersion: SchemaVersion,
		RepoHash:      state.RepoHash(repoRoot),
		ConfigHash:    c.Hash(),
		ConfigVersion: c.Version,
		ApprovedAt:    time.Now().UTC(),
		StepCount:     len(c.Verify.Steps),
	}
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return state.AtomicWrite(path(st, r.RepoHash), append(b, '\n'), 0600)
}

func Revoke(st *state.Store, repoRoot string) error {
	err := os.Remove(path(st, state.RepoHash(repoRoot)))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// Strict mode is approved per machine, not by the repository file. A
// .zeroturn.json committed with "strict" must not let a repository deny
// subagents for everyone who clones it, so the gate treats unapproved
// Strict as Confirm.
func strictPath(st *state.Store, repoHash string) string {
	return filepath.Join(st.TrustDir(), repoHash+".strict")
}

func StrictApproved(st *state.Store, repoRoot string) bool {
	_, err := os.Stat(strictPath(st, state.RepoHash(repoRoot)))
	return err == nil
}

func ApproveStrict(st *state.Store, repoRoot string) error {
	stamp := time.Now().UTC().Format(time.RFC3339) + "\n"
	return state.AtomicWrite(strictPath(st, state.RepoHash(repoRoot)), []byte(stamp), 0600)
}

func RevokeStrict(st *state.Store, repoRoot string) error {
	err := os.Remove(strictPath(st, state.RepoHash(repoRoot)))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}
