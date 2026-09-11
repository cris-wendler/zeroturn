package state

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
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

// The stored record may contain only the permitted field list.
func TestStoredRecordHasOnlyPermittedFields(t *testing.T) {
	st := testStore(t)
	v := 50.0
	st.Update("s", "h", func(s *Session) {
		s.ContextPct = &v
		s.AddActive("agent-1")
		s.LastDecision = "ask"
	})
	b, _ := ioutil.ReadFile(filepath.Join(st.SessionsDir(), "s.json"))
	var m map[string]interface{}
	json.Unmarshal(b, &m)
	permitted := map[string]bool{
		"schemaVersion": true, "sessionId": true, "repoHash": true, "harness": true, "harnessVersion": true,
		"model": true, "startedAt": true, "updatedAt": true, "durationMs": true, "contextPercent": true,
		"contextWindowSize": true, "peakContextPercent": true, "fiveHourPercent": true, "fiveHourResetsAt": true,
		"sevenDayPercent": true, "sevenDayResetsAt": true, "subagentStarts": true, "subagentStops": true,
		"activeSubagents": true, "peakActiveSubagents": true, "backgroundTasks": true, "confirmRequests": true,
		"deniedSubagentStarts": true, "allowedSubagentStarts": true, "directValidations": true,
		"directGitOperations": true, "lastDecision": true, "activeAgentIds": true,
	}
	for k := range m {
		if !permitted[k] {
			t.Errorf("unexpected stored field %q", k)
		}
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
