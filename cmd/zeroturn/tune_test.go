package main

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/tune"
)

// End to end: the gate asks, a subagent starts, and that shows up as an
// approval without anything about the work being recorded.
func TestApprovalIsLearnedFromWhatFollows(t *testing.T) {
	work, _ := repoWithConfig(t, func(c *config.Config) { c.Guard.Mode = config.ModeConfirm })
	statusline(t, work, "s", contextAt(85))
	if d, _ := decision(t, gate(t, work, "s")); d != "ask" {
		t.Fatal("the gate did not ask")
	}
	run(t, work, claudeEvent(t, "claude/subagent-start.json", work, "s",
		func(m map[string]interface{}) { m["agent_id"] = "a1" }),
		"event", "--harness", "claude", "--event", "SubagentStart")

	r := run(t, work, "", "policy", "tune", "--json")
	var report tune.Report
	if err := json.Unmarshal([]byte(r.stdout), &report); err != nil {
		t.Fatalf("tune output: %v\n%s", err, r.stdout)
	}
	if report.Asks != 1 || report.Approved != 1 {
		t.Fatalf("asks %d approved %d", report.Asks, report.Approved)
	}

	// A second ask with no subagent after it is a decline.
	statusline(t, work, "s", contextAt(97))
	gate(t, work, "s")
	run(t, work, claudeEvent(t, "claude/stop.json", work, "s", nil), "event", "--harness", "claude", "--event", "Stop")
	r = run(t, work, "", "policy", "tune", "--json")
	if err := json.Unmarshal([]byte(r.stdout), &report); err != nil {
		t.Fatal(err)
	}
	if report.Asks != 2 || report.Approved != 1 {
		t.Fatalf("asks %d approved %d, want 2 and 1", report.Asks, report.Approved)
	}
	if !strings.Contains(report.Note, "inferred") {
		t.Errorf("the report does not say the approval is inferred: %q", report.Note)
	}
}
