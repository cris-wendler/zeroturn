// Package state stores what ZeroTurn observes about a coding session.
//
// The permitted field list is deliberately narrow. Prompts, responses,
// transcripts, diffs, credentials, environment values, and absolute
// repository paths are never written here.
package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"
)

const SchemaVersion = 1

// DefaultRetentionDays bounds how long session records are kept.
const DefaultRetentionDays = 7

type Session struct {
	SchemaVersion int    `json:"schemaVersion"`
	SessionID     string `json:"sessionId"`
	RepoHash      string `json:"repoHash"`
	Harness       string `json:"harness,omitempty"`
	HarnessVer    string `json:"harnessVersion,omitempty"`
	Model         string `json:"model,omitempty"`

	StartedAt time.Time `json:"startedAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	DurationMS *int64 `json:"durationMs,omitempty"`

	ContextPct     *float64 `json:"contextPercent,omitempty"`
	ContextSize    *int     `json:"contextWindowSize,omitempty"`
	PeakContextPct *float64 `json:"peakContextPercent,omitempty"`

	FiveHourPct      *float64 `json:"fiveHourPercent,omitempty"`
	FiveHourResetsAt *int64   `json:"fiveHourResetsAt,omitempty"`
	SevenDayPct      *float64 `json:"sevenDayPercent,omitempty"`
	SevenDayResetsAt *int64   `json:"sevenDayResetsAt,omitempty"`

	SubagentStarts  int `json:"subagentStarts"`
	SubagentStops   int `json:"subagentStops"`
	ActiveSubagents int `json:"activeSubagents"`
	PeakActive      int `json:"peakActiveSubagents"`
	BackgroundTasks int `json:"backgroundTasks"`

	CredentialWarnings int `json:"credentialWarnings"`

	ConfirmRequests   int `json:"confirmRequests"`
	DeniedStarts      int `json:"deniedSubagentStarts"`
	AllowedStarts     int `json:"allowedSubagentStarts"`
	DirectValidations int `json:"directValidations"`
	DirectGitOps      int `json:"directGitOperations"`

	// LastDecision records only the decision word, never the reason text
	// and never anything from the subagent prompt.
	LastDecision string `json:"lastDecision,omitempty"`

	// activeIDs tracks which subagent identifiers are open so that a
	// repeated or out of order stop event cannot drive the count negative.
	ActiveIDs []string `json:"activeAgentIds,omitempty"`
}

type Store struct{ dir string }

// DataDir returns the operating system's standard per user data location.
func DataDir() (string, error) {
	if v := os.Getenv("ZEROTURN_STATE_DIR"); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch runtime.GOOS {
	case "darwin":
		return filepath.Join(home, "Library", "Application Support", "zeroturn"), nil
	case "windows":
		if v := os.Getenv("LOCALAPPDATA"); v != "" {
			return filepath.Join(v, "zeroturn"), nil
		}
		return filepath.Join(home, "AppData", "Local", "zeroturn"), nil
	default:
		if v := os.Getenv("XDG_DATA_HOME"); v != "" {
			return filepath.Join(v, "zeroturn"), nil
		}
		return filepath.Join(home, ".local", "share", "zeroturn"), nil
	}
}

func Open() (*Store, error) {
	d, err := DataDir()
	if err != nil {
		return nil, err
	}
	// 0700 keeps session records readable only by the current user on
	// systems that honour Unix permissions.
	if err := os.MkdirAll(filepath.Join(d, "sessions"), 0700); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Join(d, "trust"), 0700); err != nil {
		return nil, err
	}
	return &Store{dir: d}, nil
}

func (s *Store) Dir() string         { return s.dir }
func (s *Store) SessionsDir() string { return filepath.Join(s.dir, "sessions") }
func (s *Store) TrustDir() string    { return filepath.Join(s.dir, "trust") }

// RepoHash identifies a repository without recording its path.
func RepoHash(canonicalPath string) string {
	h := sha256.Sum256([]byte(canonicalPath))
	return hex.EncodeToString(h[:])[:16]
}

func safeID(id string) string {
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('_')
		}
	}
	out := b.String()
	if len(out) > 80 {
		out = out[:80]
	}
	if out == "" {
		out = "unknown"
	}
	return out
}

func (s *Store) sessionPath(id string) string {
	return filepath.Join(s.SessionsDir(), safeID(id)+".json")
}

var ErrLocked = errors.New("another ZeroTurn process holds the state lock")

const lockStale = 10 * time.Second

// Lock serialises state writes between concurrent ZeroTurn processes.
// An exclusive create is used rather than a platform lock call so the
// behaviour is the same on every supported operating system.
func (s *Store) Lock() (func(), error) {
	p := filepath.Join(s.dir, "state.lock")
	deadline := time.Now().Add(5 * time.Second)
	// The wait starts short and grows. A hook holds the lock for well
	// under a millisecond, so a fixed wait of tens of milliseconds spent
	// most of its time asleep while the lock was already free.
	wait := 200 * time.Microsecond
	for {
		f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if err == nil {
			fmt.Fprintf(f, "%d\n", os.Getpid())
			f.Close()
			return func() { os.Remove(p) }, nil
		}
		if info, statErr := os.Stat(p); statErr == nil && time.Since(info.ModTime()) > lockStale {
			os.Remove(p)
			continue
		}
		if time.Now().After(deadline) {
			return nil, ErrLocked
		}
		time.Sleep(wait)
		// The wait is capped low. One hook holds the lock for about a
		// millisecond, so a longer wait leaves a free lock unused while
		// every other hook is still asleep.
		if wait < 2*time.Millisecond {
			wait *= 2
		}
	}
}

func (s *Store) Load(id string) (Session, bool, error) {
	b, err := ioutil.ReadFile(s.sessionPath(id))
	if os.IsNotExist(err) {
		return Session{}, false, nil
	}
	if err != nil {
		return Session{}, false, err
	}
	var sess Session
	if err := json.Unmarshal(b, &sess); err != nil {
		return Session{}, false, nil
	}
	if sess.SchemaVersion != SchemaVersion {
		return Session{}, false, nil
	}
	return sess, true, nil
}

func (s *Store) Save(sess Session) error {
	sess.SchemaVersion = SchemaVersion
	sess.UpdatedAt = time.Now().UTC()
	b, err := json.MarshalIndent(sess, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(s.sessionPath(sess.SessionID), append(b, '\n'), 0600)
}

// Update applies fn to a session under the state lock.
func (s *Store) Update(id, repoHash string, fn func(*Session)) (Session, error) {
	unlock, err := s.Lock()
	if err != nil {
		return Session{}, err
	}
	defer unlock()
	sess, found, err := s.Load(id)
	if err != nil {
		return Session{}, err
	}
	if !found {
		sess = Session{SessionID: id, StartedAt: time.Now().UTC()}
		// Retention is applied when a session is first seen, which bounds
		// the work to once per session rather than once per repaint.
		s.purgeLocked(DefaultRetentionDays)
	}
	if repoHash != "" {
		sess.RepoHash = repoHash
	}
	fn(&sess)
	if sess.ActiveSubagents < 0 {
		sess.ActiveSubagents = 0
	}
	if sess.ActiveSubagents > sess.PeakActive {
		sess.PeakActive = sess.ActiveSubagents
	}
	if sess.ContextPct != nil {
		if sess.PeakContextPct == nil || *sess.ContextPct > *sess.PeakContextPct {
			v := *sess.ContextPct
			sess.PeakContextPct = &v
		}
	}
	return sess, s.Save(sess)
}

func (s *Store) List() ([]Session, error) {
	entries, err := ioutil.ReadDir(s.SessionsDir())
	if err != nil {
		return nil, err
	}
	var out []Session
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		b, err := ioutil.ReadFile(filepath.Join(s.SessionsDir(), e.Name()))
		if err != nil {
			continue
		}
		var sess Session
		if json.Unmarshal(b, &sess) != nil || sess.SchemaVersion != SchemaVersion {
			continue
		}
		out = append(out, sess)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UpdatedAt.After(out[j].UpdatedAt) })
	return out, nil
}

func (s *Store) Since(d time.Duration) ([]Session, error) {
	all, err := s.List()
	if err != nil {
		return nil, err
	}
	cut := time.Now().Add(-d)
	var out []Session
	for _, sess := range all {
		if sess.UpdatedAt.After(cut) {
			out = append(out, sess)
		}
	}
	return out, nil
}

// Purge removes ZeroTurn session records older than the retention window.
// It touches nothing outside ZeroTurn's own sessions directory.
func (s *Store) Purge(retentionDays int) (int, error) {
	unlock, err := s.Lock()
	if err != nil {
		return 0, err
	}
	defer unlock()
	return s.purgeLocked(retentionDays)
}

func (s *Store) purgeLocked(retentionDays int) (int, error) {
	entries, err := ioutil.ReadDir(s.SessionsDir())
	if err != nil {
		return 0, err
	}
	cut := time.Now().AddDate(0, 0, -retentionDays)
	n := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if e.ModTime().Before(cut) {
			if os.Remove(filepath.Join(s.SessionsDir(), e.Name())) == nil {
				n++
			}
		}
	}
	return n, nil
}

func (s *Store) PurgeAll() (int, error) {
	unlock, err := s.Lock()
	if err != nil {
		return 0, err
	}
	defer unlock()
	entries, err := ioutil.ReadDir(s.SessionsDir())
	if err != nil {
		return 0, err
	}
	n := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		if os.Remove(filepath.Join(s.SessionsDir(), e.Name())) == nil {
			n++
		}
	}
	return n, nil
}

func atomicWrite(path string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := ioutil.TempFile(dir, ".zt-tmp-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	// The file is renamed into place, so a reader sees either the old
	// record or the new one. The contents are deliberately not flushed to
	// the disk: these are session records that are rewritten many times a
	// minute, a flush costs more than the records are worth, and a record
	// lost to a power failure is replaced by the next event.
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func AtomicWrite(path string, data []byte, perm os.FileMode) error {
	return atomicWrite(path, data, perm)
}

func (s *Session) AddActive(agentID string) {
	if agentID == "" {
		s.ActiveSubagents++
		s.SubagentStarts++
		return
	}
	for _, id := range s.ActiveIDs {
		if id == agentID {
			return
		}
	}
	s.ActiveIDs = append(s.ActiveIDs, agentID)
	s.ActiveSubagents = len(s.ActiveIDs)
	s.SubagentStarts++
}

// ClearActive forgets which subagents are running. A subagent cannot
// outlive the turn that started it, so anything still counted when a
// turn ends never reported stopping, usually because the session was
// interrupted. Starts, stops, and the peak are left alone: they are the
// record of what happened.
func (s *Session) ClearActive() {
	s.ActiveIDs = nil
	s.ActiveSubagents = 0
}

func (s *Session) RemoveActive(agentID string) {
	s.SubagentStops++
	if agentID == "" {
		if s.ActiveSubagents > 0 {
			s.ActiveSubagents--
		}
		return
	}
	out := s.ActiveIDs[:0]
	found := false
	for _, id := range s.ActiveIDs {
		if id == agentID {
			found = true
			continue
		}
		out = append(out, id)
	}
	s.ActiveIDs = out
	if found {
		s.ActiveSubagents = len(s.ActiveIDs)
	}
}
