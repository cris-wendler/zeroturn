package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/evidence"
	"github.com/cris-wendler/zeroturn/internal/output"
	"github.com/cris-wendler/zeroturn/internal/state"
	"github.com/cris-wendler/zeroturn/internal/testutil"
)

// runVerify runs the command and fails the test if it did not finish, so
// each test below reads the evidence rather than the plumbing.
func okStep() config.Step {
	return config.Step{Name: "ok", Command: []string{"sh", "-c", "exit 0"}}
}

func TestVerifyRecordsEvidenceForTheStateItRanAgainst(t *testing.T) {
	work := verifyRepo(t, okStep())
	approve(t, work)

	r := run(t, work, "", "verify")
	if r.code != 0 {
		t.Fatalf("%+v", r)
	}
	if !strings.Contains(r.stdout, "Evidence ") || !strings.Contains(r.stdout, "repository state ") {
		t.Errorf("the run did not say what it recorded:\n%s", r.stdout)
	}

	st, _ := state.Open()
	rec, found, err := evidence.Load(st, state.RepoHash(testutil.Canonical(t, work)))
	if err != nil || !found {
		t.Fatalf("no evidence was recorded: found %v err %v", found, err)
	}
	if rec.Result != evidence.ResultPassed || rec.Passed != 1 {
		t.Fatalf("recorded %q with %d passed", rec.Result, rec.Passed)
	}
	if rec.Snapshot.Digest == "" || rec.Snapshot.PlanDigest == "" {
		t.Error("the evidence names no repository state")
	}
	if rec.Snapshot.Head == "" {
		t.Error("the evidence names no commit")
	}
}

// The reported state has to follow the repository, not the clock. This is
// the whole feature in one test: run the checks, change the code, ask.
func TestValidationBecomesStaleWhenTheCodeChanges(t *testing.T) {
	work := verifyRepo(t, okStep())
	approve(t, work)
	if r := run(t, work, "", "verify"); r.code != 0 {
		t.Fatalf("%+v", r)
	}

	if got := reportValidation(t, work); got.State != evidence.StatePassed || !got.Current {
		t.Fatalf("straight after a run the state is %q current %v", got.State, got.Current)
	}

	testutil.Write(t, work, "README.md", "changed after the run\n")

	got := reportValidation(t, work)
	if got.State != evidence.StateStale || got.Current {
		t.Fatalf("after a change the state is %q current %v", got.State, got.Current)
	}
	if got.Result != evidence.ResultPassed {
		t.Errorf("the recorded result was lost: %q", got.Result)
	}

	// A person reading the report has to be told why, and what to do.
	text := run(t, work, "", "report", "current")
	if !strings.Contains(text.stdout, "STALE") {
		t.Errorf("the report does not show the state:\n%s", text.stdout)
	}
	if !strings.Contains(text.stdout, "zeroturn verify") {
		t.Errorf("the report does not name the command that fixes it:\n%s", text.stdout)
	}
	if !strings.Contains(text.stdout, "working tree changed") {
		t.Errorf("the report does not say what changed:\n%s", text.stdout)
	}
}

func TestValidationIsNeverRunBeforeTheFirstVerify(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	got := reportValidation(t, work)
	if got.State != evidence.StateNeverRun {
		t.Fatalf("state %q, want never-run", got.State)
	}
	text := run(t, work, "", "report", "current")
	if !strings.Contains(text.stdout, "NEVER-RUN") || !strings.Contains(text.stdout, "zeroturn verify") {
		t.Errorf("a repository nobody has validated is not told what to do:\n%s", text.stdout)
	}
}

// A report covering the machine cannot answer for one repository, so it
// must not pretend to.
func TestAMachineReportCarriesNoValidationState(t *testing.T) {
	work := verifyRepo(t, okStep())
	approve(t, work)
	run(t, work, "", "verify")

	r := run(t, work, "", "report", "current", "--json", "--all-repositories")
	var rep reportJSON
	if err := json.Unmarshal([]byte(r.stdout), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.Scope != scopeMachine {
		t.Fatalf("scope %q", rep.Scope)
	}
	if rep.Validation != nil {
		t.Error("a report covering the machine claims a validation state for one repository")
	}
	if strings.Contains(run(t, work, "", "report", "current", "--all-repositories").stdout, "Validation") {
		t.Error("the machine report prints a validation section")
	}
}

// verify --json has to stay readable by anything that already reads it,
// which is what an additive contract change means.
func TestVerifyJSONKeepsItsExistingShapeAndAddsEvidence(t *testing.T) {
	work := verifyRepo(t, okStep())
	approve(t, work)
	r := run(t, work, "", "verify", "--json")
	if r.code != 0 {
		t.Fatalf("%+v", r)
	}

	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(r.stdout), &doc); err != nil {
		t.Fatal(err)
	}
	// Every field the 2.0.0 output had, at the level it had it.
	for _, name := range []string{"steps", "passed", "failed", "skipped", "seconds", "cancelled"} {
		if _, ok := doc[name]; !ok {
			t.Errorf("verify --json no longer carries %q at the top level", name)
		}
	}
	ev, ok := doc["evidence"].(map[string]interface{})
	if !ok {
		t.Fatal("verify --json carries no evidence")
	}
	if ev["evidenceId"] == "" || ev["evidenceId"] == nil {
		t.Error("the evidence has no identifier")
	}
	snap, ok := ev["snapshot"].(map[string]interface{})
	if !ok {
		t.Fatal("the evidence carries no snapshot")
	}
	// Versioned, so a reader can tell that a digest was computed a
	// different way rather than comparing two incomparable values.
	if snap["snapshotVersion"] == nil {
		t.Error("the snapshot is not versioned")
	}
}

// The same repository, unchanged, has to produce the same document twice,
// or nothing downstream can tell a real change from noise.
func TestValidationJSONIsDeterministic(t *testing.T) {
	work := verifyRepo(t, okStep())
	approve(t, work)
	run(t, work, "", "verify")

	first := reportValidation(t, work)
	second := reportValidation(t, work)
	if first.Evidence == nil || second.Evidence == nil {
		t.Fatal("the assessment carries no evidence")
	}
	if first.Evidence.Snapshot.Digest != second.Evidence.Snapshot.Digest {
		t.Error("two readings of one repository produced different digests")
	}
	if first.State != second.State || first.Reason != second.Reason {
		t.Errorf("two readings disagree: %q %q", first.State, second.State)
	}
}

// Evidence is new state. A machine that has session records and no
// evidence directory has to keep working, and its records must be left
// exactly as they were.
func TestExistingStateLoadsWithoutBeingRewritten(t *testing.T) {
	work := verifyRepo(t, okStep())
	approve(t, work)

	// A session recorded before this feature existed.
	statusline(t, work, "old-session", contextAt(42))
	st, _ := state.Open()
	sessionFile := filepath.Join(st.SessionsDir(), "old-session.json")
	before, err := ioutil.ReadFile(sessionFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(evidence.Dir(st)); !os.IsNotExist(err) {
		t.Fatal("the evidence directory exists before any run, so this test proves nothing")
	}

	if r := run(t, work, "", "verify"); r.code != 0 {
		t.Fatalf("%+v", r)
	}

	after, err := ioutil.ReadFile(sessionFile)
	if err != nil {
		t.Fatalf("the existing session record is gone: %v", err)
	}
	// verify counts a direct validation, which is the one field it is
	// allowed to change. Nothing else in the record may move.
	var was, now map[string]interface{}
	json.Unmarshal(before, &was)
	json.Unmarshal(after, &now)
	for k, v := range was {
		switch k {
		case "directValidations", "updatedAt":
			continue
		}
		if fmt.Sprint(now[k]) != fmt.Sprint(v) {
			t.Errorf("the existing record changed at %q: %v became %v", k, v, now[k])
		}
	}
	if fmt.Sprint(now["directValidations"]) != "1" {
		t.Errorf("the run was not counted: %v", now["directValidations"])
	}
}

// The Session Guard is not being replaced by this, and has to keep
// working while evidence is being tried out.
func TestTheSessionGuardStillWorksAlongsideEvidence(t *testing.T) {
	work := verifyRepo(t, okStep())
	approve(t, work)
	run(t, work, "", "verify")

	c, _ := config.Load(work)
	c.Guard.Mode = config.ModeConfirm
	config.Save(work, c)
	// The configuration changed, so the earlier approval is withdrawn and
	// the evidence is stale. Neither stops the guard.
	statusline(t, work, "s", contextAt(84))
	if d, _ := decision(t, gate(t, work, "s")); d == "" {
		t.Fatal("the gate returned no decision")
	}

	r := run(t, work, "", "status", "--json")
	if r.code != 0 {
		t.Fatalf("status failed beside evidence: %+v", r)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal([]byte(r.stdout), &doc); err != nil {
		t.Fatal(err)
	}
	sess, ok := doc["session"].(map[string]interface{})
	if !ok {
		t.Fatalf("the status output carries no session:\n%s", r.stdout)
	}
	if sess["contextPercent"] == nil {
		t.Error("the status line lost its measurements")
	}
}

// The status line repaints constantly. Reading the repository to compute
// a digest there would cost far more than the whole budget for a repaint,
// so it must not be reachable from that command.
func TestTheStatusLineDoesNotComputeASnapshot(t *testing.T) {
	work := verifyRepo(t, okStep())
	approve(t, work)
	run(t, work, "", "verify")

	before := statusDuration(t, work)
	// A repository with a large changed set would be slow to hash. If the
	// status line hashed it, this would show.
	for i := 0; i < 200; i++ {
		testutil.Write(t, work, fmt.Sprintf("bulk/%d.txt", i), strings.Repeat("x", 4096))
	}
	after := statusDuration(t, work)

	if after > before*4+50*time.Millisecond {
		t.Errorf("the status line got %v slower when the working tree grew, "+
			"which means it is reading the repository", after-before)
	}
}

func statusDuration(t *testing.T, work string) time.Duration {
	t.Helper()
	start := time.Now()
	statusline(t, work, "timing", contextAt(10))
	return time.Since(start)
}

// Deleting every record is a confirmed action, and a confirmation that
// cannot be given deletes nothing. The evidence has to survive that
// refusal along with everything else.
func TestADeclinedPurgeRemovesNoEvidence(t *testing.T) {
	work := verifyRepo(t, okStep())
	approve(t, work)
	run(t, work, "", "verify")

	st, _ := state.Open()
	hash := state.RepoHash(testutil.Canonical(t, work))
	if _, found, _ := evidence.Load(st, hash); !found {
		t.Fatal("no evidence to purge, so this test proves nothing")
	}

	// No terminal is attached, so the confirmation cannot be given.
	if r := run(t, work, "", "report", "purge", "--all"); r.code != output.ExitDeclined {
		t.Fatalf("%+v", r)
	}
	if _, found, _ := evidence.Load(st, hash); !found {
		t.Error("a purge nobody confirmed removed the evidence")
	}
	if got := reportValidation(t, work); got.State != evidence.StatePassed {
		t.Errorf("after a declined purge, the state is %q", got.State)
	}
}

// reportValidation reads the validation section out of report --json.
func reportValidation(t *testing.T, work string) evidence.Assessment {
	t.Helper()
	r := run(t, work, "", "report", "current", "--json")
	if r.code != 0 {
		t.Fatalf("report failed: %+v", r)
	}
	var rep reportJSON
	if err := json.Unmarshal([]byte(r.stdout), &rep); err != nil {
		t.Fatalf("%v\n%s", err, r.stdout)
	}
	if rep.Validation == nil {
		t.Fatalf("the report carries no validation section:\n%s", r.stdout)
	}
	return *rep.Validation
}
