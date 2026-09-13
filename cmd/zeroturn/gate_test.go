package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/config"
)

func TestGateObserveAllowsSilently(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	statusline(t, work, "s", contextAt(95))
	r := gate(t, work, "s")
	if r.code != 0 || r.stdout != "" {
		t.Fatalf("observe produced output or failed: %+v", r)
	}
}

func TestGateConfirmAsks(t *testing.T) {
	work, _ := repoWithConfig(t, func(c *config.Config) { c.Guard.Mode = config.ModeConfirm })
	statusline(t, work, "s", contextAt(70))
	if d, _ := decision(t, gate(t, work, "s")); d != "" {
		t.Fatalf("below the confirm threshold the gate returned %q", d)
	}
	statusline(t, work, "s", contextAt(85))
	d, reason := decision(t, gate(t, work, "s"))
	if d != "ask" {
		t.Fatalf("decision %q", d)
	}
	if reason != "New subagent requires approval. Context is 85%." {
		t.Fatalf("reason %q", reason)
	}
}

func TestGateStrictDeniesAtCritical(t *testing.T) {
	work, _ := repoWithConfig(t, func(c *config.Config) { c.Guard.Mode = config.ModeStrict })
	approveStrict(t, work)
	statusline(t, work, "s", contextAt(95))
	if d, _ := decision(t, gate(t, work, "s")); d != "deny" {
		t.Fatalf("decision %q", d)
	}
}

// A mode committed to a repository must not switch on Strict for someone
// who clones it. Without local approval the gate asks instead of denying.
func TestStrictFromRepositoryNeedsLocalApproval(t *testing.T) {
	work, _ := repoWithConfig(t, func(c *config.Config) { c.Guard.Mode = config.ModeStrict })
	statusline(t, work, "s", contextAt(95))
	if d, _ := decision(t, gate(t, work, "s")); d != "ask" {
		t.Fatalf("unapproved strict mode returned %q, want ask", d)
	}
}

func TestGateCountsSubagentsFromEvents(t *testing.T) {
	work, _ := repoWithConfig(t, func(c *config.Config) { c.Guard.Mode = config.ModeConfirm })
	statusline(t, work, "s", contextAt(10))
	for _, id := range []string{"a1", "a2"} {
		agent := id
		run(t, work, claudeEvent(t, "claude/subagent-start.json", work, "s",
			func(m map[string]interface{}) { m["agent_id"] = agent }),
			"event", "--harness", "claude", "--event", "SubagentStart")
	}
	d, reason := decision(t, gate(t, work, "s"))
	if d != "ask" || !strings.Contains(reason, "2 subagents are active") {
		t.Fatalf("decision %q reason %q", d, reason)
	}
	run(t, work, claudeEvent(t, "claude/subagent-stop.json", work, "s",
		func(m map[string]interface{}) { m["agent_id"] = "a1" }),
		"event", "--harness", "claude", "--event", "SubagentStop")
	line := statusline(t, work, "s", contextAt(10))
	if !strings.Contains(line.stdout, "agents 1") {
		t.Fatalf("status line %q", line.stdout)
	}
}

func TestGateIgnoresOtherTools(t *testing.T) {
	work, _ := repoWithConfig(t, func(c *config.Config) { c.Guard.Mode = config.ModeStrict })
	r := run(t, work, claudeEvent(t, "claude/pretooluse-bash.json", work, "s", nil),
		"event", "--harness", "claude", "--event", "PreToolUse")
	if r.code != 0 || r.stdout != "" {
		t.Fatalf("%+v", r)
	}
}

// Any failure inside the gate leaves the harness to its normal behaviour.
func TestGateFailsOpen(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	for _, in := range []string{fixture(t, "claude/malformed.json"), "", "[]"} {
		r := run(t, work, in, "event", "--harness", "claude", "--event", "PreToolUse")
		if r.code != 0 || r.stdout != "" {
			t.Fatalf("input %q: %+v", in, r)
		}
	}
	r := run(t, work, fixture(t, "normalized/bad-contract.json"), "event", "--harness", "normalized")
	if r.code != 0 || r.stdout != "" || !strings.Contains(r.stderr, "zeroturn.event/1") {
		t.Fatalf("contract mismatch: %+v", r)
	}
}

// Nothing from the subagent prompt, the transcript path, or the working
// directory may reach local state.
func TestNoPrivateContentStored(t *testing.T) {
	work, _ := repoWithConfig(t, func(c *config.Config) { c.Guard.Mode = config.ModeConfirm })
	statusline(t, work, "s", contextAt(85))
	gate(t, work, "s")
	run(t, work, claudeEvent(t, "claude/stop.json", work, "s", nil), "event", "--harness", "claude", "--event", "Stop")

	// Paths are checked in full. A bare directory name would be too
	// short to be meaningful: a runner named one "002", which also
	// appears inside a timestamp, and the test failed for no reason.
	forbidden := []string{"Read every file", "billing/", "sk-live", "transcript", "Done.", work, filepath.Dir(work)}
	filepath.Walk(os.Getenv("ZEROTURN_STATE_DIR"), func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		b, _ := ioutil.ReadFile(p)
		for _, f := range forbidden {
			if strings.Contains(string(b), f) {
				t.Errorf("%s contains %q", filepath.Base(p), f)
			}
		}
		return nil
	})

	// The repository is identified by a hash of its path, so the record
	// can be tied to a repository without holding where it is.
	sessions, err := filepath.Glob(filepath.Join(os.Getenv("ZEROTURN_STATE_DIR"), "sessions", "*.json"))
	if err != nil || len(sessions) == 0 {
		t.Fatalf("no session record was written: %v", err)
	}
	b, _ := ioutil.ReadFile(sessions[0])
	var record struct {
		RepoHash string `json:"repoHash"`
	}
	if json.Unmarshal(b, &record) != nil || len(record.RepoHash) != 16 {
		t.Fatalf("repository hash %q, want 16 characters", record.RepoHash)
	}
}

func TestStatusLineRendersFixture(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	r := statusline(t, work, "s", nil)
	want := "ZT  ctx 76%  5h 81%  7d 47%  session 3h12m"
	if r.code != 0 || !strings.HasPrefix(r.stdout, want) {
		t.Fatalf("got %q, want prefix %q", r.stdout, want)
	}
}

func TestStatusLineOmitsMissingFields(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	for _, name := range []string{"claude/statusline-null-ratelimits.json", "claude/statusline-missing.json"} {
		r := run(t, work, claudeEvent(t, name, work, "n-"+filepath.Base(name), nil), "status", "--stdin", "--harness", "claude")
		if r.code != 0 {
			t.Fatalf("%s: exit %d", name, r.code)
		}
		if strings.Contains(r.stdout, "5h 0%") || strings.Contains(r.stdout, "7d 0%") || strings.Contains(r.stdout, "ctx 0%") {
			t.Fatalf("%s: missing data shown as zero: %q", name, r.stdout)
		}
	}
}

func TestStatusLineNeverFails(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	r := run(t, work, "{not json", "status", "--stdin", "--harness", "claude")
	if r.code != 0 {
		t.Fatalf("exit %d", r.code)
	}
}

func TestStatusOutsideSessionExplains(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	r := run(t, work, "", "status")
	if r.code != 0 || !strings.Contains(r.stdout, "unavailable") || strings.Contains(r.stdout, "0%") {
		t.Fatalf("%+v", r)
	}
}

// The status line repaints often and must not start a git process. A fake
// git placed first on PATH records any call.
func TestStatusLineStartsNoGitProcess(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("uses a shell script as the fake git")
	}
	work, _ := repoWithConfig(t, func(c *config.Config) { c.Guard.Mode = config.ModeConfirm })
	fake := t.TempDir()
	marker := filepath.Join(fake, "called")
	script := "#!/bin/sh\necho called >> " + marker + "\nexit 1\n"
	if err := ioutil.WriteFile(filepath.Join(fake, "git"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", fake+string(os.PathListSeparator)+os.Getenv("PATH"))
	r := statusline(t, work, "s", contextAt(85))
	if !strings.Contains(r.stdout, "ctx 85%") {
		t.Fatalf("status line %q", r.stdout)
	}
	gate(t, work, "s")
	if _, err := os.Stat(marker); err == nil {
		t.Fatal("the status line or gate started a git process")
	}
}

// A subagent that never reports stopping, because a session was
// interrupted, must not keep the gate asking about it. The end of a turn
// clears what is still counted as running.
func TestTurnEndClearsSubagentsThatNeverStopped(t *testing.T) {
	work, _ := repoWithConfig(t, func(c *config.Config) { c.Guard.Mode = config.ModeConfirm })
	statusline(t, work, "s", contextAt(10))
	for _, id := range []string{"a1", "a2"} {
		agent := id
		run(t, work, claudeEvent(t, "claude/subagent-start.json", work, "s",
			func(m map[string]interface{}) { m["agent_id"] = agent }),
			"event", "--harness", "claude", "--event", "SubagentStart")
	}
	if d, _ := decision(t, gate(t, work, "s")); d != "ask" {
		t.Fatalf("two active subagents did not reach the threshold, decision %q", d)
	}

	run(t, work, claudeEvent(t, "claude/stop.json", work, "s", nil), "event", "--harness", "claude", "--event", "Stop")

	if d, _ := decision(t, gate(t, work, "s")); d != "" {
		t.Fatalf("the gate still counts subagents from a finished turn, decision %q", d)
	}
	line := statusline(t, work, "s", contextAt(10))
	if strings.Contains(line.stdout, "agents 2") {
		t.Fatalf("the status line still shows them: %q", line.stdout)
	}
	r := run(t, work, "", "report", "current", "--json")
	var rep reportJSON
	if err := json.Unmarshal([]byte(r.stdout), &rep); err != nil {
		t.Fatal(err)
	}
	if rep.SubagentStarts != 2 || rep.HighestActive != 2 {
		t.Fatalf("the report lost what happened: %+v", rep)
	}
}

// The rate of use is measured from the first reading of the window that
// is running now, so a repaint that reports a higher figure must not
// move the baseline and erase the rate.
func TestStatusLineKeepsTheFirstReadingOfTheWindow(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	at := func(pct float64) func(map[string]interface{}) {
		return func(m map[string]interface{}) {
			m["rate_limits"] = map[string]interface{}{
				"five_hour": map[string]interface{}{"used_percentage": pct, "resets_at": 1788000000},
			}
		}
	}
	statusline(t, work, "s", at(14))
	statusline(t, work, "s", at(31))

	sessions, _ := filepath.Glob(filepath.Join(os.Getenv("ZEROTURN_STATE_DIR"), "sessions", "*.json"))
	if len(sessions) == 0 {
		t.Fatal("no session record")
	}
	b, _ := ioutil.ReadFile(sessions[0])
	var record struct {
		Pct       float64 `json:"fiveHourPercent"`
		BasePct   float64 `json:"fiveHourBasePercent"`
		BaseAt    string  `json:"fiveHourBaseAt"`
		BaseReset int64   `json:"fiveHourBaseReset"`
	}
	if err := json.Unmarshal(b, &record); err != nil {
		t.Fatal(err)
	}
	if record.Pct != 31 || record.BasePct != 14 {
		t.Fatalf("reading %v, baseline %v, want 31 and 14", record.Pct, record.BasePct)
	}
	if record.BaseAt == "" || record.BaseReset != 1788000000 {
		t.Fatalf("baseline not tied to the window: %+v", record)
	}
}
