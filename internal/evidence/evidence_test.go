package evidence

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/git"
	"github.com/cris-wendler/zeroturn/internal/snapshot"
	"github.com/cris-wendler/zeroturn/internal/state"
	"github.com/cris-wendler/zeroturn/internal/testutil"
	"github.com/cris-wendler/zeroturn/internal/verify"
)

// fixture is a repository with a store beside it, which is the pair every
// test here needs: evidence is written for a repository and kept outside it.
func fixture(t *testing.T) (*state.Store, git.Repo, string) {
	t.Helper()
	testutil.Isolate(t)
	work, _ := testutil.Remote(t)
	st, err := state.Open()
	if err != nil {
		t.Fatal(err)
	}
	repo, err := git.Open(context.Background(), work)
	if err != nil {
		t.Fatal(err)
	}
	return st, repo, work
}

func snapOf(t *testing.T, repo git.Repo, plan string) snapshot.Snapshot {
	t.Helper()
	s, err := snapshot.Compute(context.Background(), repo, plan)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// record writes a completed run with the given result against the
// repository as it stands now.
func record(t *testing.T, st *state.Store, repo git.Repo, plan string, res verify.Result) Record {
	t.Helper()
	r, err := Begin(st, state.RepoHash(repo.Root), "test", snapOf(t, repo, plan), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	r, err = Complete(st, r, res, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func passing() verify.Result {
	return verify.Result{
		Steps:  []verify.StepResult{{Name: "test", Status: verify.StatusPass, Seconds: 0.5}},
		Passed: 1, Seconds: 0.5,
	}
}

func assess(t *testing.T, st *state.Store, repo git.Repo, plan string) Assessment {
	t.Helper()
	r, found, err := Load(st, state.RepoHash(repo.Root))
	if err != nil {
		t.Fatal(err)
	}
	return Assess(r, found, snapOf(t, repo, plan))
}

func TestWithNoRecordValidationHasNeverRun(t *testing.T) {
	st, repo, _ := fixture(t)
	a := assess(t, st, repo, "plan")
	if a.State != StateNeverRun || a.Result != ResultNeverRun {
		t.Fatalf("state %q result %q, want never-run", a.State, a.Result)
	}
	if a.Evidence != nil {
		t.Error("an assessment with no record carries one")
	}
	if !strings.Contains(a.Reason, "zeroturn verify") {
		t.Errorf("the reason does not name the command: %q", a.Reason)
	}
}

func TestEvidenceIsCurrentWhenNothingChanged(t *testing.T) {
	st, repo, _ := fixture(t)
	record(t, st, repo, "plan", passing())

	a := assess(t, st, repo, "plan")
	if !a.Current || a.State != StatePassed {
		t.Fatalf("state %q current %v, want passed and current", a.State, a.Current)
	}
	if len(a.Changed) != 0 {
		t.Errorf("nothing changed and the assessment names %v", a.Changed)
	}
}

func TestACommitAfterValidationMakesTheEvidenceStale(t *testing.T) {
	st, repo, work := fixture(t)
	record(t, st, repo, "plan", passing())

	testutil.Write(t, work, "e.txt", "new\n")
	testutil.Git(t, work, "add", "e.txt")
	testutil.Git(t, work, "commit", "--quiet", "--message", "add e")

	a := assess(t, st, repo, "plan")
	if a.State != StateStale || a.Current {
		t.Fatalf("state %q current %v, want stale", a.State, a.Current)
	}
	if !containsString(a.Changed, "the commit changed") {
		t.Errorf("the reason does not name the commit: %v", a.Changed)
	}
	// The run that was recorded still passed. Losing that would leave a
	// reader unable to tell a stale success from a stale failure.
	if a.Result != ResultPassed {
		t.Errorf("result is %q, and the recorded run passed", a.Result)
	}
}

func TestADirtyTreeChangeAfterValidationMakesTheEvidenceStale(t *testing.T) {
	st, repo, work := fixture(t)
	record(t, st, repo, "plan", passing())

	testutil.Write(t, work, "README.md", "edited after the run\n")

	a := assess(t, st, repo, "plan")
	if a.State != StateStale || a.Current {
		t.Fatalf("state %q current %v, want stale", a.State, a.Current)
	}
	if !containsString(a.Changed, "the working tree changed") {
		t.Errorf("the reason does not name the working tree: %v", a.Changed)
	}
}

func TestChangingTheValidationPlanMakesTheEvidenceStale(t *testing.T) {
	st, repo, _ := fixture(t)
	record(t, st, repo, "before", passing())

	a := assess(t, st, repo, "after")
	if a.State != StateStale {
		t.Fatalf("state %q, want stale", a.State)
	}
	if !containsString(a.Changed, "the validation steps changed") {
		t.Errorf("the reason does not name the steps: %v", a.Changed)
	}
}

// The configured steps reach the plan digest through config.Hash, which
// is the same value trust approval is bound to. This holds the two
// together: editing a step both withdraws approval and makes evidence
// stale, and it would be strange for one to happen without the other.
func TestTheConfiguredStepsAreWhatMakesThePlanDigest(t *testing.T) {
	st, repo, _ := fixture(t)
	c := config.Default()
	c.Verify.Steps = []config.Step{{Name: "test", Command: []string{"go", "test", "./..."}}}
	record(t, st, repo, c.Hash(), passing())

	if a := assess(t, st, repo, c.Hash()); a.State != StatePassed {
		t.Fatalf("the same configuration reads as %q", a.State)
	}

	c.Verify.Steps[0].Command = []string{"go", "test", "-race", "./..."}
	a := assess(t, st, repo, c.Hash())
	if a.State != StateStale || !containsString(a.Changed, "the validation steps changed") {
		t.Fatalf("editing a step left the evidence %q, changed %v", a.State, a.Changed)
	}
}

// A failure that has gone out of date is still a failure. If the word
// "stale" ever replaced the result, a developer would read it as evidence
// waiting to be refreshed rather than a fix waiting to be made.
func TestAStaleFailureStillReportsTheFailure(t *testing.T) {
	st, repo, work := fixture(t)
	failed := verify.Result{
		Steps: []verify.StepResult{
			{Name: "lint", Status: verify.StatusPass, Seconds: 0.1},
			{Name: "test", Status: verify.StatusFail, ExitCode: 2, Seconds: 0.2},
		},
		Passed: 1, Failed: 1, Seconds: 0.3,
	}
	record(t, st, repo, "plan", failed)

	testutil.Write(t, work, "README.md", "changed since the failure\n")
	a := assess(t, st, repo, "plan")

	if a.State != StateStale {
		t.Fatalf("state %q, want stale", a.State)
	}
	if a.Result != ResultFailed {
		t.Fatalf("result %q, want the failure preserved", a.Result)
	}
	if !strings.Contains(a.Reason, "failed") {
		t.Errorf("the sentence hides the failure: %q", a.Reason)
	}
	if a.Evidence == nil || a.Evidence.Failed != 1 {
		t.Error("the failure count did not survive")
	}
}

// Every outcome a run can have must stay distinguishable in what is
// stored, including the two that are not a plain pass or fail.
func TestEveryResultRemainsDistinguishable(t *testing.T) {
	cases := []struct {
		name   string
		res    verify.Result
		result string
		state  string
	}{
		{"passed", passing(), ResultPassed, StatePassed},
		{
			"failed",
			verify.Result{Steps: []verify.StepResult{{Name: "t", Status: verify.StatusFail, ExitCode: 1}}, Failed: 1},
			ResultFailed, StateFailed,
		},
		{
			"cancelled",
			verify.Result{
				Steps:     []verify.StepResult{{Name: "t", Status: verify.StatusCancelled}},
				Skipped:   1,
				Cancelled: true,
			},
			ResultCancelled, StateCancelled,
		},
		{
			// A cancelled run that also has a failure is cancelled: the
			// failure count cannot be trusted when the rest never ran.
			"cancelled after a failure",
			verify.Result{Failed: 1, Cancelled: true},
			ResultCancelled, StateCancelled,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			st, repo, _ := fixture(t)
			record(t, st, repo, "plan", c.res)
			a := assess(t, st, repo, "plan")
			if a.Result != c.result || a.State != c.state {
				t.Fatalf("result %q state %q, want %q and %q", a.Result, a.State, c.result, c.state)
			}
		})
	}
}

// Skipped steps are part of the record. A run that passed one check and
// skipped four is not a run that passed.
func TestSkippedStepsSurviveInTheRecord(t *testing.T) {
	st, repo, _ := fixture(t)
	res := verify.Result{
		Steps: []verify.StepResult{
			{Name: "lint", Status: verify.StatusFail, ExitCode: 1},
			{Name: "test", Status: verify.StatusSkipped},
			{Name: "build", Status: verify.StatusSkipped},
		},
		Failed: 1, Skipped: 2,
	}
	r := record(t, st, repo, "plan", res)
	if r.Skipped != 2 {
		t.Errorf("skipped count is %d, want 2", r.Skipped)
	}
	if len(r.Steps) != 3 {
		t.Fatalf("the record holds %d steps, want 3", len(r.Steps))
	}
	if r.Steps[1].Status != verify.StatusSkipped {
		t.Errorf("a skipped step reads as %q", r.Steps[1].Status)
	}
}

// A run that started and never reported is neither current nor stale. It
// has no result to have gone out of date, and reporting it as though the
// previous successful run were the last word is how a developer comes to
// trust evidence for code that was never validated.
func TestAnInterruptedRunIsRecordedAsRunning(t *testing.T) {
	st, repo, _ := fixture(t)
	if _, err := Begin(st, state.RepoHash(repo.Root), "test", snapOf(t, repo, "plan"), time.Now()); err != nil {
		t.Fatal(err)
	}

	a := assess(t, st, repo, "plan")
	if a.State != StateRunning || a.Result != ResultRunning {
		t.Fatalf("state %q result %q, want running", a.State, a.Result)
	}
	if a.Current {
		t.Error("a run that never reported is presented as current evidence")
	}
	if !strings.Contains(a.Reason, "interrupted") {
		t.Errorf("the sentence does not offer the interrupted reading: %q", a.Reason)
	}
}

// A started record must not be left behind when the run finishes, or
// every completed run would still read as running.
func TestCompletingReplacesTheStartedRecord(t *testing.T) {
	st, repo, _ := fixture(t)
	r := record(t, st, repo, "plan", passing())
	if r.Result != ResultPassed {
		t.Fatalf("result %q", r.Result)
	}
	if r.FinishedAt == nil {
		t.Fatal("a completed record has no finish time")
	}
	loaded, found, err := Load(st, state.RepoHash(repo.Root))
	if err != nil || !found {
		t.Fatalf("the completed record was not stored: %v", err)
	}
	if loaded.Result != ResultPassed || loaded.EvidenceID != r.EvidenceID {
		t.Fatalf("the stored record is %q under %q", loaded.Result, loaded.EvidenceID)
	}
}

// permittedFields is every name the evidence record may carry. The same
// rule the session record follows: adding a field is a decision about
// what ZeroTurn keeps, made here rather than by writing a struct field.
// Nothing here may name source contents, command output, or a path on
// somebody's machine.
var permittedFields = map[string]bool{
	"schemaVersion": true, "evidenceId": true, "repoHash": true, "zeroturnVersion": true,
	"startedAt": true, "finishedAt": true,
	"result": true, "passed": true, "failed": true, "skipped": true, "seconds": true,
	"snapshot": true, "steps": true,

	"snapshot.snapshotVersion": true, "snapshot.digest": true, "snapshot.head": true,
	"snapshot.planDigest": true, "snapshot.branch": true, "snapshot.clean": true,
	"snapshot.trackedChanges": true, "snapshot.untrackedFiles": true,
	"snapshot.dirtySubmodules": true,

	"steps.name": true, "steps.status": true, "steps.exitCode": true,
	"steps.seconds": true, "steps.logName": true,
}

func jsonNames(t reflect.Type, prefix string, out map[string]bool) {
	for i := 0; i < t.NumField(); i++ {
		f := t.Field(i)
		name := strings.Split(f.Tag.Get("json"), ",")[0]
		if name == "" || name == "-" {
			continue
		}
		out[prefix+name] = true
		ft := f.Type
		for ft.Kind() == reflect.Ptr || ft.Kind() == reflect.Slice {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct && ft != reflect.TypeOf(time.Time{}) {
			jsonNames(ft, prefix+name+".", out)
		}
	}
}

func TestTheRecordCanHoldOnlyPermittedFields(t *testing.T) {
	declared := map[string]bool{}
	jsonNames(reflect.TypeOf(Record{}), "", declared)

	for name := range declared {
		if !permittedFields[name] {
			t.Errorf("the record can store %q, which is not on the permitted list. "+
				"Add it deliberately, or do not store it", name)
		}
	}
	for name := range permittedFields {
		if !declared[name] {
			t.Errorf("%q is permitted but the record no longer has it, so the list is stale", name)
		}
	}
}

// The type says what can be written. This says what a real run actually
// writes, with a step whose output holds a credential and whose log path
// names a directory on this machine.
func TestWhatIsWrittenHoldsNoSecretsOrSourceOrPaths(t *testing.T) {
	st, repo, work := fixture(t)
	testutil.Write(t, work, "app.go", "package app // const key = \"topsecretsource\"\n")
	testutil.Git(t, work, "add", "app.go")

	logPath := filepath.Join(work, ".git", "zeroturn", "logs", "20260916-101500-test.log")
	res := verify.Result{
		Steps: []verify.StepResult{{
			Name: "test", Status: verify.StatusFail, ExitCode: 1, Seconds: 1,
			LogFile: logPath,
			Excerpt: "AKIAIOSFODNN7EXAMPLE was rejected by the server",
		}},
		Failed: 1, Seconds: 1,
	}
	record(t, st, repo, "plan", res)

	b, err := ioutil.ReadFile(Path(st, state.RepoHash(repo.Root)))
	if err != nil {
		t.Fatal(err)
	}
	stored := string(b)
	forbidden := map[string]string{
		"AKIAIOSFODNN7EXAMPLE": "the step output, which carries whatever the tool printed",
		"topsecretsource":      "source contents",
		"app.go":               "the path of a changed file",
		work:                   "an absolute repository path",
		logPath:                "an absolute log path",
	}
	for value, what := range forbidden {
		if strings.Contains(stored, value) {
			t.Errorf("the stored record carries %s (%q)", what, value)
		}
	}
	// The log is still findable by name, without saying which machine it
	// is on, so the failure can be read in full from the repository.
	if !strings.Contains(stored, "20260916-101500-test.log") {
		t.Error("the log file name was not kept, so the full output cannot be found")
	}

	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for k := range m {
		if !permittedFields[k] {
			t.Errorf("unexpected stored field %q", k)
		}
	}
}

// The record is read back by a build that agrees on the format. One
// written under a different schema version is not this build's to
// interpret, and the honest answer is that there is no evidence.
func TestARecordFromAnotherSchemaVersionIsNotRead(t *testing.T) {
	st, repo, _ := fixture(t)
	r := record(t, st, repo, "plan", passing())

	r.SchemaVersion = SchemaVersion + 1
	b, _ := json.MarshalIndent(r, "", "  ")
	if err := state.AtomicWrite(Path(st, r.RepoHash), b, 0600); err != nil {
		t.Fatal(err)
	}

	if _, found, err := Load(st, r.RepoHash); found || err != nil {
		t.Fatalf("a record from another schema version was read: found %v err %v", found, err)
	}
	if a := assess(t, st, repo, "plan"); a.State != StateNeverRun {
		t.Errorf("state is %q, want never-run", a.State)
	}
}

// A record left half written would be read as evidence. The write is
// atomic, so a reader sees the whole of one record or the whole of the
// one before it.
func TestAnInterruptedWriteLeavesTheEarlierRecordIntact(t *testing.T) {
	st, repo, _ := fixture(t)
	first := record(t, st, repo, "plan", passing())

	// A temporary file left behind by a write that died must not be
	// mistaken for the record, and must not stop the record being read.
	if err := ioutil.WriteFile(filepath.Join(Dir(st), ".zt-tmp-broken"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, found, err := Load(st, first.RepoHash)
	if err != nil || !found {
		t.Fatalf("the record could not be read beside a half written file: %v", err)
	}
	if loaded.EvidenceID != first.EvidenceID {
		t.Error("a half written file was read as the record")
	}
}

// Nothing may be able to leave a record that is not valid JSON, whichever
// writer wins. Every reader either gets a whole record or none.
func TestConcurrentWritesLeaveAReadableRecord(t *testing.T) {
	st, repo, _ := fixture(t)
	snap := snapOf(t, repo, "plan")
	hash := state.RepoHash(repo.Root)

	var wg sync.WaitGroup
	var mu sync.Mutex
	var writeErrs []error
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			fail := func(err error) {
				mu.Lock()
				writeErrs = append(writeErrs, err)
				mu.Unlock()
			}
			r, err := Begin(st, hash, "test", snap, time.Now().Add(time.Duration(n)*time.Millisecond))
			if err != nil {
				fail(err)
				return
			}
			if _, err := Complete(st, r, passing(), time.Now()); err != nil {
				fail(err)
			}
		}(i)
	}
	// Reading while the writers run must never fail and never see a
	// partial record. On Windows a plain open of a file being replaced
	// fails outright, which is what this found.
	var readErrs []error
	for i := 0; i < 50; i++ {
		if _, _, err := Load(st, hash); err != nil {
			readErrs = append(readErrs, err)
		}
	}
	wg.Wait()

	if len(readErrs) > 0 {
		t.Errorf("%d of 50 concurrent reads failed, first: %v", len(readErrs), readErrs[0])
	}
	// A write losing to a reader is the same defect seen from the other
	// side: verify would report that it could not record evidence.
	if len(writeErrs) > 0 {
		t.Errorf("%d concurrent writes failed, first: %v", len(writeErrs), writeErrs[0])
	}

	r, found, err := Load(st, hash)
	if err != nil || !found {
		t.Fatalf("no readable record survived concurrent writes: found %v err %v", found, err)
	}
	if r.Result != ResultPassed {
		t.Errorf("the surviving record reads %q", r.Result)
	}
	// One record per repository, whatever happened.
	entries, err := ioutil.ReadDir(Dir(st))
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			n++
		}
	}
	if n != 1 {
		t.Errorf("%d records were left for one repository, want 1", n)
	}
}

// Evidence is a ZeroTurn record, so the command that removes every
// ZeroTurn record has to reach it.
func TestPurgeAllRemovesEveryEvidenceRecord(t *testing.T) {
	st, repo, _ := fixture(t)

	// Nothing to remove is not a failure: the command runs on machines
	// where no validation has ever been recorded.
	if n, err := PurgeAll(st); n != 0 || err != nil {
		t.Fatalf("purging an absent directory gave %d, %v", n, err)
	}

	record(t, st, repo, "plan", passing())
	// A second repository, so the purge is shown to clear more than one.
	other := Record{
		SchemaVersion: SchemaVersion, EvidenceID: "x", RepoHash: "otherrepo",
		ZeroTurnVer: "test", StartedAt: time.Now(), Result: ResultPassed, Steps: []Step{},
	}
	if err := Save(st, other); err != nil {
		t.Fatal(err)
	}

	n, err := PurgeAll(st)
	if err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("purged %d records, want 2", n)
	}
	if _, found, _ := Load(st, state.RepoHash(repo.Root)); found {
		t.Error("a record survived the purge")
	}
	if a := assess(t, st, repo, "plan"); a.State != StateNeverRun {
		t.Errorf("after purging, the state is %q", a.State)
	}
}

// The purge removes evidence records and nothing else. It runs over a
// directory on somebody's machine, and the README promises it removes
// ZeroTurn's records and nothing besides.
func TestPurgeAllLeavesWhatIsNotAnEvidenceRecord(t *testing.T) {
	st, repo, _ := fixture(t)
	record(t, st, repo, "plan", passing())

	stray := filepath.Join(Dir(st), "notes.txt")
	if err := ioutil.WriteFile(stray, []byte("somebody put this here\n"), 0600); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(Dir(st), "adirectory.json")
	if err := os.Mkdir(dir, 0755); err != nil {
		t.Fatal(err)
	}

	n, err := PurgeAll(st)
	if err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("purged %d records, want the one that is a record", n)
	}
	if _, err := os.Stat(stray); err != nil {
		t.Errorf("a file that is not a record was removed: %v", err)
	}
	if _, err := os.Stat(dir); err != nil {
		t.Errorf("a directory was removed: %v", err)
	}
}

// A run that was cancelled and a run that failed are different answers,
// and the sentence a person reads has to say which one happened.
func TestAStaleCancelledRunSaysItWasCancelled(t *testing.T) {
	st, repo, work := fixture(t)
	r := passing()
	r.Cancelled = true
	r.Steps[0].Status = verify.StatusCancelled
	record(t, st, repo, "plan", r)
	testutil.Write(t, work, "after.txt", "the code moved\n")

	a := assess(t, st, repo, "plan")
	if a.Result != ResultCancelled {
		t.Fatalf("the result is %q", a.Result)
	}
	if !strings.Contains(a.Reason, "cancelled") {
		t.Errorf("the reason does not say it was cancelled: %q", a.Reason)
	}
	if strings.Contains(a.Reason, "failed") {
		t.Errorf("a cancelled run is reported as a failure: %q", a.Reason)
	}
}

// The identifier is derived, so writing the same run twice cannot produce
// two names for it, and two different runs cannot share one.
func TestTheEvidenceIdentifierIsDerivedFromTheRun(t *testing.T) {
	at := time.Now()
	a := ID("repo", "digest", at)
	if a != ID("repo", "digest", at) {
		t.Error("the same run produced two identifiers")
	}
	if a == ID("repo", "other", at) {
		t.Error("two repository states share an identifier")
	}
	if a == ID("other", "digest", at) {
		t.Error("two repositories share an identifier")
	}
	if a == ID("repo", "digest", at.Add(time.Second)) {
		t.Error("two runs at different times share an identifier")
	}
}

// Evidence is kept outside the repository, in ZeroTurn's own state
// directory, which is what uninstall removes and what the state directory
// setting points at. A record written into the repository would be a file
// ZeroTurn left in somebody's project.
func TestEvidenceIsKeptInTheStateDirectoryAndNotTheRepository(t *testing.T) {
	st, repo, work := fixture(t)
	record(t, st, repo, "plan", passing())

	if !strings.HasPrefix(Dir(st), st.Dir()) {
		t.Errorf("evidence is kept at %q, outside the state directory %q", Dir(st), st.Dir())
	}
	err := filepath.Walk(work, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if strings.Contains(info.Name(), "evidence") {
			return errFound(p)
		}
		return nil
	})
	if err != nil {
		t.Errorf("evidence was written inside the repository: %v", err)
	}
}

type errFound string

func (e errFound) Error() string { return string(e) }

func containsString(list []string, want string) bool {
	for _, s := range list {
		if s == want {
			return true
		}
	}
	return false
}
