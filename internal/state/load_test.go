package state

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

// A session that runs for hours writes a record on every repaint and on
// every event. The record must stay a fixed size rather than growing
// with the session, and the work must not grow either.
func TestLongSessionKeepsRecordsSmall(t *testing.T) {
	st := testStore(t)
	var sizes []int64
	for i := 0; i < 2000; i++ {
		pct := float64(i%100) + 1
		if _, err := st.Update("long", "repo", func(s *Session) {
			s.ContextPct = &pct
			if i%20 == 0 {
				s.AddActive(fmt.Sprintf("agent-%d", i))
			}
			if i%20 == 10 {
				s.RemoveActive(fmt.Sprintf("agent-%d", i-10))
			}
		}); err != nil {
			t.Fatal(err)
		}
		if i%500 == 0 {
			info, err := os.Stat(filepath.Join(st.SessionsDir(), "long.json"))
			if err != nil {
				t.Fatal(err)
			}
			sizes = append(sizes, info.Size())
		}
	}
	first, last := sizes[0], sizes[len(sizes)-1]
	if last > first*2 {
		t.Fatalf("the record grew from %d to %d bytes over one session", first, last)
	}
	got, _, _ := st.Load("long")
	if got.SubagentStarts != 100 || got.ActiveSubagents != 0 {
		t.Fatalf("counts after a long session: starts %d active %d", got.SubagentStarts, got.ActiveSubagents)
	}
	t.Logf("record size over 2000 updates: %d to %d bytes", first, last)
}

// A session where subagents start and never report stopping, which is
// what a crash looks like, keeps their identifiers so a late stop is
// still counted correctly. The record therefore grows with the number of
// open subagents, and this records how much.
func TestRecordGrowthWithSubagentsThatNeverStop(t *testing.T) {
	st := testStore(t)
	for i := 0; i < 500; i++ {
		n := i
		if _, err := st.Update("crashy", "repo", func(s *Session) {
			s.AddActive(fmt.Sprintf("agent-%d", n))
		}); err != nil {
			t.Fatal(err)
		}
	}
	info, err := os.Stat(filepath.Join(st.SessionsDir(), "crashy.json"))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("500 subagents with no stop event: %d bytes", info.Size())
	if info.Size() > 64*1024 {
		t.Fatalf("the record reached %d bytes, which is more than a session record should ever need", info.Size())
	}
}

// Hooks fire in parallel: a status line repaint, a subagent starting,
// another stopping, all at once. No update may be lost and no writer may
// wait unreasonably long for the lock.
func TestParallelHooksLoseNothing(t *testing.T) {
	st := testStore(t)
	const writers = 60
	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if _, err := st.Update("busy", "repo", func(s *Session) {
				s.AddActive(fmt.Sprintf("a%d", i))
			}); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	elapsed := time.Since(start)

	got, _, _ := st.Load("busy")
	if got.SubagentStarts != writers || got.ActiveSubagents != writers {
		t.Fatalf("starts %d active %d, want %d", got.SubagentStarts, got.ActiveSubagents, writers)
	}
	// Generously wide. The point is that no writer is starved or lost.
	// Real timings belong in the benchmark harness, not in a test that
	// shares a machine with the rest of the suite.
	if elapsed > 60*time.Second {
		t.Fatalf("%d parallel writers took %s", writers, elapsed)
	}
	t.Logf("%d parallel writers in %s", writers, elapsed)
}

// Sessions accumulate. Reading the newest one, which the status line and
// the gate do, must not slow down as the directory fills, and retention
// must keep the directory from growing without limit.
func TestManySessionsStayFast(t *testing.T) {
	st := testStore(t)
	for i := 0; i < 400; i++ {
		if _, err := st.Update(fmt.Sprintf("session-%d", i), "repo", func(s *Session) {
			s.SubagentStarts = i
		}); err != nil {
			t.Fatal(err)
		}
	}
	start := time.Now()
	recent, err := st.Since(12 * time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	elapsed := time.Since(start)
	if len(recent) != 400 {
		t.Fatalf("read %d sessions", len(recent))
	}
	if elapsed > 30*time.Second {
		t.Fatalf("reading 400 session records took %s", elapsed)
	}
	t.Logf("400 session records read in %s", elapsed)

	// Retention runs when a new session appears, and old records go.
	old := time.Now().AddDate(0, 0, -(DefaultRetentionDays + 1))
	entries, _ := ioutil.ReadDir(st.SessionsDir())
	for _, e := range entries {
		p := filepath.Join(st.SessionsDir(), e.Name())
		os.Chtimes(p, old, old)
	}
	if _, err := st.Update("fresh", "repo", func(*Session) {}); err != nil {
		t.Fatal(err)
	}
	left, _ := ioutil.ReadDir(st.SessionsDir())
	if len(left) != 1 {
		t.Fatalf("%d records left after retention, want only the new one", len(left))
	}
}

// A record left behind by an interrupted write must not stop the next
// session, and must not be read as if it were complete.
func TestInterruptedWriteIsRecoverable(t *testing.T) {
	st := testStore(t)
	st.Update("s", "repo", func(s *Session) { s.SubagentStarts = 3 })
	path := filepath.Join(st.SessionsDir(), "s.json")
	whole, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(path, whole[:len(whole)/2], 0600); err != nil {
		t.Fatal(err)
	}
	if _, found, err := st.Load("s"); found || err != nil {
		t.Fatalf("a half written record was read as complete: found=%v err=%v", found, err)
	}
	sess, err := st.Update("s", "repo", func(s *Session) { s.AddActive("a1") })
	if err != nil {
		t.Fatalf("a half written record stopped the next update: %v", err)
	}
	if sess.SubagentStarts != 1 {
		t.Fatalf("counts after recovery: %+v", sess)
	}
}
