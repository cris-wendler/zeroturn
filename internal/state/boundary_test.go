package state

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// These cover conditions in the record store that scripts/mutate could
// change with no test disagreeing. This package decides what is kept and
// what a gate outcome means, so a rule here that stops applying is one
// nobody would see stop.

// A retention of one day is a choice, not an absence of one. The fallback
// applies only when nothing was set.
func TestARetentionOfOneDayIsHonoured(t *testing.T) {
	var s Store
	if got := s.retention(); got != DefaultRetentionDays {
		t.Errorf("nothing set falls back to %d, want %d", got, DefaultRetentionDays)
	}
	s.SetRetention(1)
	if got := s.retention(); got != 1 {
		t.Errorf("a retention of one day reads back as %d", got)
	}
	s.SetRetention(0)
	if got := s.retention(); got != DefaultRetentionDays {
		t.Errorf("a retention of zero reads back as %d, want the default", got)
	}
}

// Every test sets ZEROTURN_STATE_DIR, so the branch that works out where
// records belong on each operating system was never reached.
func TestTheDataDirectoryFollowsTheOperatingSystem(t *testing.T) {
	home := t.TempDir()
	t.Setenv("ZEROTURN_STATE_DIR", "")
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_DATA_HOME", "")
	t.Setenv("LOCALAPPDATA", "")

	dir, err := DataDir()
	if err != nil {
		t.Fatalf("no data directory: %v", err)
	}
	if !strings.HasPrefix(dir, home) {
		t.Errorf("the data directory is %s, which is not under the home directory %s", dir, home)
	}
	if filepath.Base(dir) != "zeroturn" {
		t.Errorf("the data directory is %s, which is not named for the project", dir)
	}

	// The override is the override.
	chosen := filepath.Join(home, "somewhere-else")
	t.Setenv("ZEROTURN_STATE_DIR", chosen)
	if got, _ := DataDir(); got != chosen {
		t.Errorf("the override was ignored, got %s", got)
	}

}

// Every operating system's answer, from whichever one is running the
// tests. Two of the three branches used to be unreachable wherever the
// tests happened to run.
func TestTheDataDirectoryOnEveryOperatingSystem(t *testing.T) {
	none := func(string) string { return "" }
	for name, c := range map[string]struct {
		goos, want string
		env        func(string) string
	}{
		"macOS":            {"darwin", "/h/Library/Application Support/zeroturn", none},
		"linux":            {"linux", "/h/.local/share/zeroturn", none},
		"linux with XDG":   {"linux", "/xdg/zeroturn", func(k string) string { return map[string]string{"XDG_DATA_HOME": "/xdg"}[k] }},
		"windows":          {"windows", filepath.Join("/h", "AppData", "Local", "zeroturn"), none},
		"windows with app": {"windows", filepath.Join("/app", "zeroturn"), func(k string) string { return map[string]string{"LOCALAPPDATA": "/app"}[k] }},
		"anything else":    {"plan9", "/h/.local/share/zeroturn", none},
	} {
		got := dataDirFor(c.goos, "/h", c.env)
		if got != filepath.FromSlash(c.want) {
			t.Errorf("%s: %s, want %s", name, got, filepath.FromSlash(c.want))
		}
	}
}

// Two identifiers must never name one record, and the characters that
// are kept are kept at their edges too.
func TestTheRecordNameKeepsTheCharactersItSays(t *testing.T) {
	for _, id := range []string{"az", "AZ", "09", "a-z_0"} {
		if got := safeID(id); got != id {
			t.Errorf("safeID(%q) is %q, and every character in it is permitted", id, got)
		}
	}

	// Anything else is replaced, and the replacement alone would let two
	// identifiers share a file, so a digest keeps them apart.
	seen := map[string]string{}
	for _, id := range []string{"a/b", "a_b", "a:b", "a b", "a.b"} {
		got := safeID(id)
		if prev, ok := seen[got]; ok {
			t.Errorf("%q and %q both name the record %q", prev, id, got)
		}
		seen[got] = id
	}

	// A long identifier is cut to fit, and still names one record.
	long := strings.Repeat("a", 200)
	if got := safeID(long); len(got) > 80 {
		t.Errorf("a long identifier produced a name of %d characters", len(got))
	}
	if safeID(long) == safeID(long+"b") {
		t.Error("two long identifiers produced the same name")
	}
	// One at the boundary is left alone, because nothing about it changed.
	eighty := strings.Repeat("a", 80)
	if got := safeID(eighty); got != eighty {
		t.Errorf("an identifier of exactly 80 permitted characters became %q", got)
	}
}

// The store reads only its own records. Anything else in the directory is
// left alone, and a record from a schema this build does not know is not
// read as though it were current.
func TestOnlyCurrentRecordsAreRead(t *testing.T) {
	st := testStore(t)
	st.Update("real", "", func(*Session) {})

	// Not a record, and not readable as one.
	if err := ioutil.WriteFile(filepath.Join(st.SessionsDir(), "notes.txt"), []byte("hello"), 0600); err != nil {
		t.Fatal(err)
	}
	// Readable as one, and still not a record, because the store owns
	// what it named. Without this the two halves of the rule cannot be
	// told apart: a file that fails to parse is skipped either way.
	if err := ioutil.WriteFile(filepath.Join(st.SessionsDir(), "borrowed.txt"),
		[]byte(`{"schemaVersion":1,"sessionId":"borrowed"}`), 0600); err != nil {
		t.Fatal(err)
	}
	// A record from another schema version.
	if err := ioutil.WriteFile(filepath.Join(st.SessionsDir(), "old.json"),
		[]byte(`{"schemaVersion":0,"sessionId":"old"}`), 0600); err != nil {
		t.Fatal(err)
	}
	// A directory that looks like one.
	if err := os.Mkdir(filepath.Join(st.SessionsDir(), "adir.json"), 0700); err != nil {
		t.Fatal(err)
	}

	got, err := st.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].SessionID != "real" {
		var names []string
		for _, s := range got {
			names = append(names, s.SessionID)
		}
		t.Errorf("List returned %v, want only the current record", names)
	}
}

// The baseline for the rate projection is the lowest reading of the
// window that is running now. A reading equal to the one already kept
// changes nothing, and one below it starts a new window.
func TestTheFiveHourBaselineKeepsTheLowestReading(t *testing.T) {
	now := time.Now().UTC()
	reset := now.Add(3 * time.Hour).Unix()

	var s Session
	s.SampleFiveHour(40, &reset, now)
	if s.FiveHourBasePct == nil || *s.FiveHourBasePct != 40 {
		t.Fatalf("the first reading was not kept: %+v", s.FiveHourBasePct)
	}

	// Higher, same window: the baseline stands.
	s.SampleFiveHour(60, &reset, now.Add(time.Minute))
	if *s.FiveHourBasePct != 40 {
		t.Errorf("a higher reading moved the baseline to %.0f", *s.FiveHourBasePct)
	}
	// Equal: still stands, and so does the moment it was taken. The rate
	// is measured over the span since that moment, so moving it while
	// keeping the percentage would quietly halve the measured rate.
	takenAt := *s.FiveHourBaseAt
	s.SampleFiveHour(40, &reset, now.Add(2*time.Minute))
	if *s.FiveHourBasePct != 40 {
		t.Errorf("an equal reading moved the baseline to %.0f", *s.FiveHourBasePct)
	}
	if !s.FiveHourBaseAt.Equal(takenAt) {
		t.Errorf("an equal reading moved the baseline from %s to %s", takenAt, *s.FiveHourBaseAt)
	}
	// Lower: the window rolled over, so this is the new baseline.
	s.SampleFiveHour(5, &reset, now.Add(3*time.Minute))
	if *s.FiveHourBasePct != 5 {
		t.Errorf("a lower reading left the baseline at %.0f", *s.FiveHourBasePct)
	}
}

// A baseline taken under a different reset time belongs to a window that
// has ended, and one taken under no reset time at all is not comparable.
func TestAWindowIsTheSameOnlyWhenBothResetTimesAgree(t *testing.T) {
	a, b := int64(100), int64(200)
	for name, c := range map[string]struct {
		x, y *int64
		want bool
	}{
		"both absent":  {nil, nil, true},
		"one absent":   {&a, nil, false},
		"other absent": {nil, &a, false},
		"same":         {&a, &a, true},
		"different":    {&a, &b, false},
	} {
		if got := sameWindow(c.x, c.y); got != c.want {
			t.Errorf("%s: sameWindow is %v, want %v", name, got, c.want)
		}
	}
}

// Only an ask waits for an outcome, and the wait is resolved by the next
// subagent starting.
func TestOnlyAnAskWaitsToBeApproved(t *testing.T) {
	var s Session
	s.RecordGate("deny", []string{"context"})
	if s.PendingAsk != 0 {
		t.Errorf("a denial is waiting to be approved, at %d", s.PendingAsk)
	}

	s.RecordGate("ask", []string{"context"})
	if s.PendingAsk != len(s.GateOutcomes) {
		t.Errorf("an ask is not waiting, PendingAsk %d of %d", s.PendingAsk, len(s.GateOutcomes))
	}
	s.ResolveGate()
	if !s.GateOutcomes[len(s.GateOutcomes)-1].Approved {
		t.Error("the waiting ask was not approved")
	}
	if s.PendingAsk != 0 {
		t.Error("the ask is still waiting after being resolved")
	}

	// Resolving with nothing waiting approves nothing.
	before := s.GateOutcomes[0].Approved
	s.ResolveGate()
	if s.GateOutcomes[0].Approved != before {
		t.Error("resolving with nothing waiting changed an earlier outcome")
	}
}

// A session keeps a sample of outcomes rather than a history, and the
// most recent are the ones worth keeping.
func TestTheOutcomeListIsBoundedAtItsLimit(t *testing.T) {
	var s Session
	for i := 0; i < maxOutcomes; i++ {
		s.RecordGate("ask", []string{"context"})
	}
	if len(s.GateOutcomes) != maxOutcomes {
		t.Fatalf("%d outcomes kept at the limit, want %d", len(s.GateOutcomes), maxOutcomes)
	}

	s.RecordGate("deny", []string{"fiveHour"})
	if len(s.GateOutcomes) != maxOutcomes {
		t.Errorf("%d outcomes kept past the limit, want %d", len(s.GateOutcomes), maxOutcomes)
	}
	last := s.GateOutcomes[len(s.GateOutcomes)-1]
	if last.Decision != "deny" {
		t.Errorf("the newest outcome was dropped rather than the oldest: %q", last.Decision)
	}
}
