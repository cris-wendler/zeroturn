// Package capabilities publishes what this build supports so an adapter
// can discover it rather than assume it.
package capabilities

import (
	"github.com/cris-wendler/zeroturn/internal/events"
)

// ContractVersion covers the command surface, the event shape, the JSON
// output, and the exit codes together.
//
// 1.1.0 added the file.read event and the credential guard. 1.2.0 added
// the rate projection: three session fields and the projection trigger.
// Both took nothing away.
//
// 2.0.0 does take something away, which is why it is a major version.
// `policy show --json` printed the guard, so a reader took `.mode` from
// the top level. It now prints the whole configuration, so the same
// value is at `.guard.mode`. The change was needed because retention is
// a setting outside the guard, and a document calling itself the policy
// while omitting a policy setting is wrong. Also in 2.0.0, and additive:
// a report carries `scope`, saying whether it counted one repository or
// the machine.
const ContractVersion = "2.0.0"

// ClaudeTestedVersion is the harness release on which allow and deny were
// observed end to end. Other releases may work, and are reported as
// untested rather than refused.
const ClaudeTestedVersion = "2.1.265"

type Harness struct {
	Name            string   `json:"name"`
	Status          string   `json:"status"`
	TestedVersions  []string `json:"testedVersions"`
	Events          []string `json:"events"`
	GateSupported   bool     `json:"gateSupported"`
	StatusSupported bool     `json:"statusSupported"`
	Notes           string   `json:"notes"`
}

type Doc struct {
	Product         string            `json:"product"`
	Version         string            `json:"version"`
	ContractVersion string            `json:"contractVersion"`
	EventContract   string            `json:"eventContract"`
	Modes           []string          `json:"guardModes"`
	Commands        []string          `json:"commands"`
	EventTypes      []string          `json:"eventTypes"`
	Harnesses       []Harness         `json:"harnesses"`
	ExitCodes       map[string]string `json:"exitCodes"`
	Records         []string          `json:"recordedFields"`
	NeverRecords    []string          `json:"neverRecorded"`
	Network         bool              `json:"makesNetworkRequests"`
	ModelCalls      bool              `json:"makesModelCalls"`
	Daemon          bool              `json:"runsDaemon"`
}

func Describe(version string) Doc {
	return Doc{
		Product:         "zeroturn",
		Version:         version,
		ContractVersion: ContractVersion,
		EventContract:   events.Contract,
		Modes:           []string{"observe", "confirm", "strict"},
		Commands: []string{
			"init", "integrate", "status", "policy", "report",
			"verify", "ship", "uninstall", "capabilities", "doctor", "version", "event",
		},
		EventTypes: []string{
			events.TypeStatus, events.TypeSubagentPre, events.TypeFileRead, events.TypeSubagentStrt,
			events.TypeSubagentStop, events.TypeSessionStop, events.TypeSessionEnd,
		},
		Harnesses: []Harness{
			{
				Name: "claude", Status: "supported",
				TestedVersions:  []string{ClaudeTestedVersion},
				Events:          []string{"statusLine", "PreToolUse(Agent)", "PreToolUse(Read)", "UserPromptSubmit when the prompt guard is on", "SubagentStart", "SubagentStop", "Stop", "SessionEnd"},
				GateSupported:   true,
				StatusSupported: true,
				Notes:           "Context and usage windows come from the status line payload. Subagent counts are maintained by ZeroTurn from hook events and are not supplied by the harness.",
			},
			{
				Name: "copilot", Status: "planned",
				TestedVersions:  []string{},
				Events:          []string{},
				GateSupported:   false,
				StatusSupported: false,
				Notes:           "No adapter ships in this version, so nothing here claims Copilot support. The installed CLI does expose a pre tool decision through a command hook, and session values through the status line process rather than through hooks, which is recorded in docs/decisions.md. An adapter is planned and will be marked supported only after it has run against a real session.",
			},
		},
		ExitCodes: map[string]string{
			"0": "success",
			"1": "validation or policy failure",
			"2": "invalid invocation or configuration",
			"3": "unsafe Git state",
			"4": "required executable unavailable",
			"5": "internal failure",
			"6": "confirmation declined",
			"7": "incompatible contract version",
			"8": "integration unavailable",
		},
		Records: []string{
			"session identifier", "harness name", "harness version", "model label",
			"context percentage", "context window size",
			"five hour usage percentage", "seven day usage percentage", "window reset times",
			"session duration", "subagent start count", "subagent stop count",
			"active subagent count", "background task count",
			"policy decisions", "direct command counts", "validation results", "repository hash",
		},
		NeverRecords: []string{
			"prompts", "responses", "source code", "diffs",
			"tool arguments unrelated to ZeroTurn", "subagent instructions", "subagent responses",
			"transcript contents", "credentials", "environment variables",
			"absolute repository paths", "remote URLs containing credentials",
		},
		Network:    false,
		ModelCalls: false,
		Daemon:     false,
	}
}
