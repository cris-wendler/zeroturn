package events

import (
	"encoding/json"
	"math/rand"
	"strings"
	"testing"
)

// A harness payload is input from another program. Whatever arrives, the
// parser must return a value or an error, never panic, and never carry
// content it does not declare.
func TestHostilePayloads(t *testing.T) {
	huge := strings.Repeat("a", 1<<20)
	cases := map[string]string{
		"empty object":            `{}`,
		"empty array":             `[]`,
		"a bare string":           `"hello"`,
		"a number":                `42`,
		"null":                    `null`,
		"truncated":               `{"session_id": "a`,
		"trailing content":        `{"session_id":"a"} and then some`,
		"duplicate keys":          `{"session_id":"a","session_id":"b"}`,
		"wrong type for a string": `{"session_id": 42, "cwd": ["a"], "version": {"x": 1}}`,
		"wrong type for a number": `{"context_window": {"used_percentage": "seventy"}}`,
		"null inside an object":   `{"context_window": {"used_percentage": null, "context_window_size": null}}`,
		"a number out of range":   `{"context_window": {"used_percentage": 1e309}}`,
		"a negative percentage":   `{"context_window": {"used_percentage": -5}}`,
		"deeply nested":           `{"cost":` + strings.Repeat(`{"a":`, 200) + `1` + strings.Repeat(`}`, 200) + `}`,
		"a very long field":       `{"session_id": "` + huge + `"}`,
		"invalid utf8":            "{\"session_id\": \"a\xff\xfeb\"}",
		"a null byte":             "{\"session_id\": \"a\\u0000b\"}",
		"unicode":                 `{"session_id": "会话", "cwd": "/tmp/项目"}`,
		"an enormous array":       `{"background_tasks": [` + strings.TrimSuffix(strings.Repeat(`{},`, 5000), ",") + `]}`,
	}
	for name, payload := range cases {
		for _, event := range []string{"", "PreToolUse", "Stop", "SessionEnd", "UserPromptSubmit", "SubagentStart"} {
			func() {
				defer func() {
					if r := recover(); r != nil {
						t.Fatalf("%s with event %q panicked: %v", name, event, r)
					}
				}()
				e, err := ParseClaude(strings.NewReader(payload), event)
				if err != nil {
					return
				}
				blob, merr := json.Marshal(e)
				if merr != nil {
					t.Errorf("%s: the parsed event cannot be marshalled: %v", name, merr)
				}
				if strings.Contains(string(blob), "transcript") {
					t.Errorf("%s: the event kept a transcript path", name)
				}
			}()
		}
	}
}

// The same, for the shape an adapter sends.
func TestHostileNormalizedPayloads(t *testing.T) {
	cases := []string{
		`{}`, `[]`, `null`, `{"contract": 1}`, `{"contract": "zeroturn.event/1"}`,
		`{"contract":"zeroturn.event/1","harness":"x","type":"status"}`,
		`{"contract":"zeroturn.event/1","harness":"x","type":"nonsense","sessionId":"a"}`,
		`{"contract":"zeroturn.event/1","harness":"","type":"status","sessionId":"a"}`,
		`{"contract":"zeroturn.event/1","harness":"x","type":"status","sessionId":"a","contextPercent":"80"}`,
		`{"contract":"zeroturn.event/1","harness":"x","type":"status","sessionId":"a","unknown":{"deep":[1,2,3]}}`,
	}
	for _, payload := range cases {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Fatalf("%q panicked: %v", payload, r)
				}
			}()
			ParseNormalized(strings.NewReader(payload))
		}()
	}
}

// Random bytes are not a realistic payload, but they are what a broken
// pipe or a half written file looks like.
func TestRandomInputNeverPanics(t *testing.T) {
	r := rand.New(rand.NewSource(20260912))
	for i := 0; i < 2000; i++ {
		b := make([]byte, r.Intn(400))
		for j := range b {
			b[j] = byte(r.Intn(256))
		}
		func() {
			defer func() {
				if rec := recover(); rec != nil {
					t.Fatalf("random input panicked: %v\ninput: %q", rec, b)
				}
			}()
			ParseClaude(strings.NewReader(string(b)), "PreToolUse")
			ParseNormalized(strings.NewReader(string(b)))
		}()
	}
}

// A payload that claims a percentage outside the range it can hold is
// still passed on as the harness sent it, so the policy sees the truth
// rather than a number ZeroTurn invented.
func TestPercentagesArePassedThroughUnchanged(t *testing.T) {
	e, err := ParseClaude(strings.NewReader(`{"context_window":{"used_percentage":140}}`), "")
	if err != nil {
		t.Fatal(err)
	}
	if e.ContextPct == nil || *e.ContextPct != 140 {
		t.Fatalf("context percentage %v", e.ContextPct)
	}
}
