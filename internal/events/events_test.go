package events

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, name string) string {
	t.Helper()
	b, err := ioutil.ReadFile(filepath.Join("..", "..", "fixtures", name))
	if err != nil {
		t.Fatalf("fixture %s: %v", name, err)
	}
	return string(b)
}

func TestStatusLineFullPayload(t *testing.T) {
	e, err := ParseClaude(strings.NewReader(fixture(t, "claude/statusline-full.json")), "statusline")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if e.Type != TypeStatus {
		t.Errorf("type = %q, want %q", e.Type, TypeStatus)
	}
	if e.SessionID != "fixture-full" {
		t.Errorf("session = %q", e.SessionID)
	}
	if e.ContextPct == nil || *e.ContextPct != 76 {
		t.Errorf("context percent not read")
	}
	if e.FiveHourPct == nil || *e.FiveHourPct != 81 {
		t.Errorf("five hour percent not read")
	}
	if e.SevenDayPct == nil || *e.SevenDayPct != 47 {
		t.Errorf("seven day percent not read")
	}
	if e.DurationMS == nil || *e.DurationMS != 11520000 {
		t.Errorf("duration not read")
	}
	if e.Model != "Opus" {
		t.Errorf("model = %q", e.Model)
	}
	if e.HarnessVersion != "2.1.265" {
		t.Errorf("harness version = %q", e.HarnessVersion)
	}
}

func TestMissingFieldsStayNil(t *testing.T) {
	e, err := ParseClaude(strings.NewReader(fixture(t, "claude/statusline-missing.json")), "statusline")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if e.ContextPct != nil {
		t.Error("a null context percentage must stay unavailable, not become zero")
	}
	if e.FiveHourPct != nil || e.SevenDayPct != nil {
		t.Error("absent rate limits must stay unavailable")
	}
	if e.DurationMS != nil {
		t.Error("absent duration must stay unavailable")
	}
}

func TestNullObjectsDoNotPanic(t *testing.T) {
	e, err := ParseClaude(strings.NewReader(fixture(t, "claude/statusline-null-ratelimits.json")), "statusline")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if e.ContextPct != nil || e.FiveHourPct != nil || e.Model != "" {
		t.Error("explicit nulls must produce unavailable values")
	}
}

func TestMalformedJSONIsRejected(t *testing.T) {
	_, err := ParseClaude(strings.NewReader(fixture(t, "claude/malformed.json")), "statusline")
	if err == nil {
		t.Fatal("malformed JSON must be rejected")
	}
}

func TestEmptyInputIsRejected(t *testing.T) {
	if _, err := ParseClaude(strings.NewReader("   \n"), "statusline"); err != ErrEmpty {
		t.Fatalf("empty input error = %v, want ErrEmpty", err)
	}
}

// TestNoPrivateContentSurvivesParsing is the privacy guarantee in test
// form. The gate payload carries a subagent prompt and a transcript path,
// and neither may appear anywhere in the parsed event.
func TestNoPrivateContentSurvivesParsing(t *testing.T) {
	raw := fixture(t, "claude/pretooluse-agent.json")
	e, err := ParseClaude(strings.NewReader(raw), "PreToolUse")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	blob, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{
		"Read every file", "sk-live-not-a-real-key", "transcript.jsonl",
		"Review the payment module", "scratchpad",
	} {
		if strings.Contains(string(blob), forbidden) {
			t.Errorf("parsed event retained private content %q", forbidden)
		}
	}
}

func TestSubagentStopDiscardsLastMessage(t *testing.T) {
	e, err := ParseClaude(strings.NewReader(fixture(t, "claude/subagent-stop.json")), "SubagentStop")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	blob, _ := json.Marshal(e)
	if strings.Contains(string(blob), "billing module") {
		t.Error("the subagent response must not be retained")
	}
	if e.AgentID != "a7eb794ae13c0510c" {
		t.Errorf("agent id = %q", e.AgentID)
	}
	if e.BackgroundTasks == nil || *e.BackgroundTasks != 1 {
		t.Error("background tasks must be counted")
	}
}

func TestGateIgnoresOtherTools(t *testing.T) {
	_, err := ParseClaude(strings.NewReader(fixture(t, "claude/pretooluse-bash.json")), "PreToolUse")
	if err == nil {
		t.Fatal("a PreToolUse for Bash must not be treated as a subagent gate")
	}
	if !IsUnsupportedTool(err) {
		t.Fatalf("error = %v, want an unsupported tool error", err)
	}
}

func TestSessionEndReason(t *testing.T) {
	e, err := ParseClaude(strings.NewReader(fixture(t, "claude/session-end.json")), "SessionEnd")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if e.Type != TypeSessionEnd || e.EndReason != "clear" {
		t.Errorf("type = %q reason = %q", e.Type, e.EndReason)
	}
}

func TestUnknownEventIsRejected(t *testing.T) {
	_, err := ParseClaude(strings.NewReader(`{"session_id":"x"}`), "PostToolUse")
	if err == nil {
		t.Fatal("an event outside the contract must be rejected")
	}
}

func TestNormalizedContractMismatch(t *testing.T) {
	_, err := ParseNormalized(strings.NewReader(fixture(t, "normalized/bad-contract.json")))
	if err == nil {
		t.Fatal("a mismatched contract version must be rejected")
	}
	if _, ok := err.(ContractError); !ok {
		t.Fatalf("error type = %T, want ContractError", err)
	}
}

func TestNormalizedAccepted(t *testing.T) {
	e, err := ParseNormalized(strings.NewReader(fixture(t, "normalized/subagent-pre.json")))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if e.Harness != "copilot" || e.Type != TypeSubagentPre {
		t.Errorf("harness = %q type = %q", e.Harness, e.Type)
	}
}
