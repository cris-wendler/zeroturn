package state

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	t.Setenv("ZEROTURN_STATE_DIR", t.TempDir())
	st, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestUpdateCreatesAndPersists(t *testing.T) {
	st := testStore(t)
	v := 42.0
	if _, err := st.Update("s1", "abc", func(s *Session) { s.ContextPct = &v }); err != nil {
		t.Fatal(err)
	}
	got, found, err := st.Load("s1")
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if got.RepoHash != "abc" || *got.ContextPct != 42 || *got.PeakContextPct != 42 {
		t.Fatalf("stored %+v", got)
	}
}

func TestPeakContextKeepsMaximum(t *testing.T) {
	st := testStore(t)
	for _, v := range []float64{30, 80, 50} {
		x := v
		st.Update("s", "", func(s *Session) { s.ContextPct = &x })
	}
	got, _, _ := st.Load("s")
	if *got.PeakContextPct != 80 || *got.ContextPct != 50 {
		t.Fatalf("peak %v current %v", *got.PeakContextPct, *got.ContextPct)
	}
}

func TestSubagentCounting(t *testing.T) {
	var s Session
	s.AddActive("a")
	s.AddActive("b")
	s.AddActive("a")
	if s.ActiveSubagents != 2 || s.SubagentStarts != 2 {
		t.Fatalf("duplicate start counted: %+v", s)
	}
	s.RemoveActive("a")
	s.RemoveActive("a")
	if s.ActiveSubagents != 1 || s.SubagentStops != 2 {
		t.Fatalf("after stops: %+v", s)
	}
	s.RemoveActive("unknown")
	if s.ActiveSubagents != 1 {
		t.Fatal("a stop for an unknown agent changed the active count")
	}
}

func TestStopWithoutStartNeverGoesNegative(t *testing.T) {
	var s Session
	s.RemoveActive("")
	s.RemoveActive("x")
	if s.ActiveSubagents != 0 {
		t.Fatalf("active %d", s.ActiveSubagents)
	}
}

func TestPeakActiveTracked(t *testing.T) {
	st := testStore(t)
	for _, id := range []string{"a", "b", "c"} {
		agent := id
		st.Update("s", "", func(s *Session) { s.AddActive(agent) })
	}
	st.Update("s", "", func(s *Session) { s.RemoveActive("a") })
	got, _, _ := st.Load("s")
	if got.PeakActive != 3 || got.ActiveSubagents != 2 {
		t.Fatalf("peak %d active %d", got.PeakActive, got.ActiveSubagents)
	}
}

// Concurrent hook processes must not lose updates.
func TestConcurrentUpdatesAreSerialised(t *testing.T) {
	st := testStore(t)
	const n = 25
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := st.Update("s", "", func(s *Session) { s.AddActive("") }); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	got, _, _ := st.Load("s")
	if got.SubagentStarts != n {
		t.Fatalf("starts %d, want %d", got.SubagentStarts, n)
	}
}

func TestStaleLockIsRecovered(t *testing.T) {
	st := testStore(t)
	p := filepath.Join(st.Dir(), "state.lock")
	ioutil.WriteFile(p, []byte("99999\n"), 0600)
	old := time.Now().Add(-time.Minute)
	os.Chtimes(p, old, old)
	unlock, err := st.Lock()
	if err != nil {
		t.Fatalf("stale lock was not recovered: %v", err)
	}
	unlock()
}

func TestCorruptRecordIsReplacedNotFatal(t *testing.T) {
	st := testStore(t)
	ioutil.WriteFile(filepath.Join(st.SessionsDir(), "s.json"), []byte("{not json"), 0600)
	if _, found, err := st.Load("s"); found || err != nil {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if _, err := st.Update("s", "", func(s *Session) { s.AddActive("") }); err != nil {
		t.Fatal(err)
	}
}

func TestSessionIDCannotEscapeDirectory(t *testing.T) {
	st := testStore(t)
	if _, err := st.Update("../../etc/passwd", "", func(*Session) {}); err != nil {
		t.Fatal(err)
	}
	entries, _ := ioutil.ReadDir(st.SessionsDir())
	if len(entries) != 1 || strings.Contains(entries[0].Name(), "..") || strings.Contains(entries[0].Name(), "/") {
		t.Fatalf("entries %v", entries)
	}
	if safeID("") != "unknown" || len(safeID(strings.Repeat("a", 200))) != 80 {
		t.Fatal("safeID bounds")
	}
}

func TestPurgeRemovesOnlyOldSessionRecords(t *testing.T) {
	st := testStore(t)
	st.Update("old", "", func(*Session) {})
	st.Update("new", "", func(*Session) {})
	old := time.Now().AddDate(0, 0, -8)
	os.Chtimes(filepath.Join(st.SessionsDir(), "old.json"), old, old)

	unrelated := filepath.Join(st.SessionsDir(), "notes.txt")
	ioutil.WriteFile(unrelated, []byte("keep"), 0600)
	os.Chtimes(unrelated, old, old)
	trustFile := filepath.Join(st.TrustDir(), "r.json")
	ioutil.WriteFile(trustFile, []byte("{}"), 0600)
	os.Chtimes(trustFile, old, old)

	n, err := st.Purge(DefaultRetentionDays)
	if err != nil || n != 1 {
		t.Fatalf("purged %d err %v", n, err)
	}
	for _, keep := range []string{filepath.Join(st.SessionsDir(), "new.json"), unrelated, trustFile} {
		if _, err := os.Stat(keep); err != nil {
			t.Errorf("%s was removed", filepath.Base(keep))
		}
	}
	n, _ = st.PurgeAll()
	if n != 1 {
		t.Fatalf("purge all removed %d", n)
	}
	if _, err := os.Stat(unrelated); err != nil {
		t.Fatal("purge all removed a file that is not a session record")
	}
}

func TestSinceFiltersByUpdateTime(t *testing.T) {
	st := testStore(t)
	st.Update("a", "", func(*Session) {})
	got, _ := st.Since(time.Hour)
	if len(got) != 1 {
		t.Fatalf("got %d", len(got))
	}
	s, _, _ := st.Load("a")
	s.UpdatedAt = time.Now().Add(-2 * time.Hour)
	b, _ := json.Marshal(s)
	ioutil.WriteFile(filepath.Join(st.SessionsDir(), "a.json"), b, 0600)
	got, _ = st.Since(time.Hour)
	if len(got) != 0 {
		t.Fatalf("stale record returned")
	}
}

func TestFilesAreUserOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Unix permissions")
	}
	st := testStore(t)
	st.Update("s", "", func(*Session) {})
	info, _ := os.Stat(filepath.Join(st.SessionsDir(), "s.json"))
	if info.Mode().Perm() != 0600 {
		t.Fatalf("mode %v", info.Mode().Perm())
	}
	dinfo, _ := os.Stat(st.SessionsDir())
	if dinfo.Mode().Perm() != 0700 {
		t.Fatalf("dir mode %v", dinfo.Mode().Perm())
	}
}

func TestRepoHashHidesPath(t *testing.T) {
	h := RepoHash("/Users/someone/private/project")
	if len(h) != 16 || strings.Contains(h, "someone") {
		t.Fatalf("hash %q", h)
	}
	if RepoHash("/a") == RepoHash("/b") {
		t.Fatal("different paths share a hash")
	}
}

// permittedFields is every name the stored record is allowed to carry.
// Adding a field to Session without adding it here fails the test below,
// which is the point: the list is the decision about what ZeroTurn keeps,
// and it has to be made deliberately rather than by writing a struct
// field. Nothing here may name anything a person wrote.
var permittedFields = map[string]bool{
	"schemaVersion": true, "sessionId": true, "repoHash": true, "harness": true, "harnessVersion": true,
	"model": true, "startedAt": true, "updatedAt": true, "durationMs": true, "contextPercent": true,
	"contextWindowSize": true, "peakContextPercent": true, "fiveHourPercent": true, "fiveHourResetsAt": true,
	"fiveHourBasePercent": true, "fiveHourBaseAt": true, "fiveHourBaseReset": true,
	"sevenDayPercent": true, "sevenDayResetsAt": true, "subagentStarts": true, "subagentStops": true,
	"activeSubagents": true, "peakActiveSubagents": true, "backgroundTasks": true, "confirmRequests": true,
	"credentialWarnings":   true,
	"deniedSubagentStarts": true, "allowedSubagentStarts": true, "directValidations": true,
	"directGitOperations": true, "lastDecision": true, "activeAgentIds": true,
	"gateOutcomes": true, "pendingAsk": true,
	// Inside a gate outcome.
	"gateOutcomes.decision": true, "gateOutcomes.at": true, "gateOutcomes.approved": true,
	"gateOutcomes.triggers": true, "gateOutcomes.contextPercent": true, "gateOutcomes.fiveHourPercent": true,
	"gateOutcomes.sevenDayPercent": true, "gateOutcomes.durationMinutes": true,
	"gateOutcomes.activeSubagents": true, "gateOutcomes.subagentStarts": true,
}

// jsonNames walks a type and returns the name of every field it can
// write, descending into the structs it contains.
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

// The stored record may contain only the permitted field list.
//
// This is checked against the type rather than against a sample record.
// Almost every field is omitted when it holds no value, so a record a
// test builds shows only the fields that test happened to set, and a new
// field nobody meant to store would pass unnoticed. Three fields did
// exactly that before this was written.
func TestStoredRecordHasOnlyPermittedFields(t *testing.T) {
	declared := map[string]bool{}
	jsonNames(reflect.TypeOf(Session{}), "", declared)

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

// The type says what can be written. This says what actually is.
func TestWrittenRecordCarriesOnlyPermittedFields(t *testing.T) {
	st := testStore(t)
	v := 50.0
	reset := int64(1788000000)
	st.Update("s", "h", func(s *Session) {
		s.ContextPct = &v
		s.FiveHourPct = &v
		s.FiveHourResetsAt = &reset
		s.SampleFiveHour(v, &reset, time.Now())
		s.AddActive("agent-1")
		s.LastDecision = "ask"
		s.RecordGate("ask", []string{"context"})
	})
	b, _ := ioutil.ReadFile(filepath.Join(st.SessionsDir(), "s.json"))
	var m map[string]interface{}
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	for k := range m {
		if !permittedFields[k] {
			t.Errorf("unexpected stored field %q", k)
		}
	}
	if _, ok := m["gateOutcomes"]; !ok {
		t.Fatal("the record under test carries no gate outcome, so the nested fields were not exercised")
	}
}

func TestNewSessionAppliesRetention(t *testing.T) {
	st := testStore(t)
	st.Update("old", "", func(*Session) {})
	old := time.Now().AddDate(0, 0, -(DefaultRetentionDays + 1))
	os.Chtimes(filepath.Join(st.SessionsDir(), "old.json"), old, old)
	st.Update("old", "", func(*Session) {})
	os.Chtimes(filepath.Join(st.SessionsDir(), "old.json"), old, old)
	if _, found, _ := st.Load("old"); !found {
		t.Fatal("an update to an existing session purged records")
	}
	st.Update("new", "", func(*Session) {})
	if _, found, _ := st.Load("old"); found {
		t.Fatal("a record past retention survived the start of a new session")
	}
}

func TestClearActiveKeepsTheRecordOfWhatHappened(t *testing.T) {
	var s Session
	s.AddActive("a1")
	s.AddActive("a2")
	s.RemoveActive("a1")
	s.ClearActive()
	if s.ActiveSubagents != 0 || len(s.ActiveIDs) != 0 {
		t.Fatalf("active after clearing: %d %v", s.ActiveSubagents, s.ActiveIDs)
	}
	if s.SubagentStarts != 2 || s.SubagentStops != 1 {
		t.Fatalf("history was lost: starts %d stops %d", s.SubagentStarts, s.SubagentStops)
	}
}

// A subagent that never reports stopping, because the session was
// interrupted, must not be counted as running for the rest of the
// session, and must not make the record grow without limit.
func TestInterruptedSubagentsDoNotAccumulate(t *testing.T) {
	st := testStore(t)
	for turn := 0; turn < 50; turn++ {
		for i := 0; i < 10; i++ {
			id := fmt.Sprintf("turn%d-agent%d", turn, i)
			st.Update("s", "repo", func(s *Session) { s.AddActive(id) })
		}
		st.Update("s", "repo", func(s *Session) { s.ClearActive() })
	}
	got, _, _ := st.Load("s")
	if got.ActiveSubagents != 0 || len(got.ActiveIDs) != 0 {
		t.Fatalf("active after 50 turns: %d", got.ActiveSubagents)
	}
	if got.SubagentStarts != 500 || got.PeakActive != 10 {
		t.Fatalf("starts %d peak %d", got.SubagentStarts, got.PeakActive)
	}
	info, err := os.Stat(filepath.Join(st.SessionsDir(), "s.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() > 2048 {
		t.Fatalf("the record reached %d bytes after 500 subagents", info.Size())
	}
}

func TestFiveHourBaselineKeepsTheFirstReadingOfTheWindow(t *testing.T) {
	reset := int64(1788000000)
	var s Session
	t0 := time.Now()
	s.SampleFiveHour(12, &reset, t0)
	s.SampleFiveHour(30, &reset, t0.Add(time.Hour))
	s.SampleFiveHour(44, &reset, t0.Add(2*time.Hour))
	if s.FiveHourBasePct == nil || *s.FiveHourBasePct != 12 {
		t.Fatalf("baseline moved: %v", s.FiveHourBasePct)
	}
	if s.FiveHourBaseAt == nil || !s.FiveHourBaseAt.Equal(t0.UTC()) {
		t.Fatalf("baseline time moved: %v", s.FiveHourBaseAt)
	}
}

func TestFiveHourBaselineRestartsWhenTheWindowDoes(t *testing.T) {
	first, second := int64(1788000000), int64(1788018000)
	var s Session
	t0 := time.Now()
	s.SampleFiveHour(80, &first, t0)
	s.SampleFiveHour(4, &second, t0.Add(time.Hour))
	if *s.FiveHourBasePct != 4 || *s.FiveHourBaseReset != second {
		t.Fatalf("baseline from the old window survived: %v", *s.FiveHourBasePct)
	}
}

// The harness may not report a reset time. A reading below the baseline
// is then the only sign that the window has rolled over.
func TestFiveHourBaselineRestartsOnAReadingThatFell(t *testing.T) {
	var s Session
	t0 := time.Now()
	s.SampleFiveHour(70, nil, t0)
	s.SampleFiveHour(75, nil, t0.Add(30*time.Minute))
	if *s.FiveHourBasePct != 70 {
		t.Fatalf("baseline moved on a rise: %v", *s.FiveHourBasePct)
	}
	s.SampleFiveHour(5, nil, t0.Add(time.Hour))
	if *s.FiveHourBasePct != 5 {
		t.Fatalf("baseline stayed above the reading: %v", *s.FiveHourBasePct)
	}
}

// Lock must answer, whatever is wrong with the directory. A retry loop
// that cannot make progress spins on a processor and never returns, and
// the hook that called it never exits.
func TestLockReturnsWhenTheLockCannotBeCreated(t *testing.T) {
	cases := map[string]func(t *testing.T, dir string){
		"state.lock is a directory": func(t *testing.T, dir string) {
			p := filepath.Join(dir, "state.lock")
			if err := os.Mkdir(p, 0700); err != nil {
				t.Fatal(err)
			}
			// Not empty, so it cannot be removed as a stale lock file.
			if err := ioutil.WriteFile(filepath.Join(p, "keep"), []byte("x"), 0600); err != nil {
				t.Fatal(err)
			}
			old := time.Now().Add(-time.Hour)
			os.Chtimes(p, old, old)
		},
		"directory is read only": func(t *testing.T, dir string) {
			p := filepath.Join(dir, "state.lock")
			if err := ioutil.WriteFile(p, []byte("99999\n"), 0600); err != nil {
				t.Fatal(err)
			}
			old := time.Now().Add(-time.Hour)
			os.Chtimes(p, old, old)
			if err := os.Chmod(dir, 0500); err != nil {
				t.Skip(err)
			}
		},
	}

	defer func(d time.Duration) { lockTimeout = d }(lockTimeout)
	lockTimeout = 200 * time.Millisecond

	for name, setup := range cases {
		t.Run(name, func(t *testing.T) {
			dir, err := ioutil.TempDir("", "zt-lock")
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				os.Chmod(dir, 0700)
				os.RemoveAll(dir)
			}()
			setup(t, dir)

			st := &Store{dir: dir}
			done := make(chan error, 1)
			go func() {
				_, lerr := st.Lock()
				done <- lerr
			}()
			select {
			case lerr := <-done:
				if lerr == nil {
					t.Fatal("the lock was reported as taken")
				}
			case <-time.After(20 * time.Second):
				t.Fatal("Lock never returned: it is spinning")
			}
		})
	}
}

// A lock nobody released is cleared, which is the case the retry loop
// exists for and must keep working.
func TestLockClearsALockLeftBehind(t *testing.T) {
	dir, err := ioutil.TempDir("", "zt-lock")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)
	p := filepath.Join(dir, "state.lock")
	if err := ioutil.WriteFile(p, []byte("99999\n"), 0600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-time.Hour)
	os.Chtimes(p, old, old)

	st := &Store{dir: dir}
	start := time.Now()
	unlock, err := st.Lock()
	if err != nil {
		t.Fatalf("a lock older than the stale limit was not cleared: %v", err)
	}
	unlock()
	if time.Since(start) > 2*time.Second {
		t.Fatalf("clearing a stale lock took %s", time.Since(start))
	}
}
