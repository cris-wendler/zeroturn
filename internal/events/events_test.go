package events

import (
	"encoding/json"
	"io/ioutil"
	"path/filepath"
	"reflect"
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

func TestParseClaudeFileRead(t *testing.T) {
	e, err := ParseClaude(strings.NewReader(fixture(t, "claude/pretooluse-read.json")), "PreToolUse")
	if err != nil {
		t.Fatal(err)
	}
	if e.Type != TypeFileRead {
		t.Fatalf("type %q", e.Type)
	}
	if e.FilePath != "/fixture/repo/deploy.env" {
		t.Fatalf("file path %q", e.FilePath)
	}
}

// The read payload carries a limit and a transcript path. Neither is
// declared, so neither can reach the rest of ZeroTurn.
func TestFileReadEventCarriesOnlyThePath(t *testing.T) {
	e, err := ParseClaude(strings.NewReader(fixture(t, "claude/pretooluse-read.json")), "PreToolUse")
	if err != nil {
		t.Fatal(err)
	}
	blob, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"transcript", "limit", "toolu_fixture_read"} {
		if strings.Contains(string(blob), forbidden) {
			t.Errorf("the event retained %q", forbidden)
		}
	}
}

func TestPreToolUseForAnotherToolIsUnsupported(t *testing.T) {
	_, err := ParseClaude(strings.NewReader(`{"session_id":"s","hook_event_name":"PreToolUse","tool_name":"Grep"}`), "PreToolUse")
	if !IsUnsupportedTool(err) {
		t.Fatalf("err %v", err)
	}
}

// An adapter for another harness has to be able to say everything the
// Claude path says. When the two parsers disagree, a feature built on
// the Claude payload is silently unavailable everywhere else: the rate
// projection shipped that way, because the normalized event could not
// carry a window reset time and nothing compared the two.
func TestBothParsersCarryTheSameSessionFacts(t *testing.T) {
	const claude = `{
		"session_id": "s-1",
		"cwd": "/tmp/project",
		"version": "2.1.265",
		"model": {"display_name": "Opus"},
		"context_window": {"used_percentage": 76, "context_window_size": 200000},
		"rate_limits": {
			"five_hour": {"used_percentage": 81, "resets_at": 1788000000},
			"seven_day": {"used_percentage": 47, "resets_at": 1788400000}
		},
		"cost": {"total_duration_ms": 11520000}
	}`
	const normalized = `{
		"contract": "zeroturn.event/1",
		"harness": "other",
		"harnessVersion": "2.1.265",
		"type": "status",
		"sessionId": "s-1",
		"cwd": "/tmp/project",
		"model": "Opus",
		"contextPercent": 76,
		"contextWindowSize": 200000,
		"fiveHourPercent": 81,
		"fiveHourResetsAt": 1788000000,
		"sevenDayPercent": 47,
		"sevenDayResetsAt": 1788400000,
		"durationMs": 11520000
	}`

	fromClaude, err := ParseClaude(strings.NewReader(claude), "statusline")
	if err != nil {
		t.Fatal(err)
	}
	fromAdapter, err := ParseNormalized(strings.NewReader(normalized))
	if err != nil {
		t.Fatal(err)
	}

	// The harness name is meant to differ. Everything describing the
	// session has to match.
	fromClaude.Harness, fromAdapter.Harness = "", ""
	fromClaude.Type, fromAdapter.Type = "", ""

	if !reflect.DeepEqual(fromClaude, fromAdapter) {
		t.Errorf("the two parsers disagree about the same session\nclaude:     %+v\nnormalized: %+v",
			fromClaude, fromAdapter)
	}
}

// Every measurement the Claude payload can report must have somewhere to
// live in the adapter facing shape, or an adapter cannot report it.
func TestTheNormalizedShapeCanCarryEveryMeasurement(t *testing.T) {
	normalized := map[string]bool{}
	nt := reflect.TypeOf(Normalized{})
	for i := 0; i < nt.NumField(); i++ {
		normalized[nt.Field(i).Name] = true
	}
	// Fields of Event the normalized form deliberately does not carry.
	// Prompt exists only on the Claude path, for the prompt guard.
	skip := map[string]bool{"Contract": true, "Prompt": true}

	et := reflect.TypeOf(Event{})
	for i := 0; i < et.NumField(); i++ {
		name := et.Field(i).Name
		if skip[name] || normalized[name] {
			continue
		}
		t.Errorf("an event can carry %s and an adapter has no way to send it", name)
	}
}

// The published schema requires a session identifier and the record is
// keyed by it, so a payload without one is refused by both parsers
// rather than stored where it would be shared.
func TestAnEventMustNameItsSession(t *testing.T) {
	if _, err := ParseClaude(strings.NewReader(`{"hook_event_name":"Stop"}`), "Stop"); err == nil {
		t.Error("the claude parser accepted an event with no session")
	}
	normalized := `{"contract":"` + Contract + `","harness":"other","type":"` + TypeStatus + `"}`
	if _, err := ParseNormalized(strings.NewReader(normalized)); err == nil {
		t.Error("the normalized parser accepted an event with no session")
	}
}
