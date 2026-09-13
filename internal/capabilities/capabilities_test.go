package capabilities

import (
	"strconv"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/events"
	"github.com/cris-wendler/zeroturn/internal/output"
)

// An adapter reads this document instead of assuming, so a claim here
// that the code does not back is worse than no claim at all. These tests
// tie the document to the code it describes.

func TestExitCodesMatchTheOnesTheProgramUses(t *testing.T) {
	want := map[int]string{
		output.ExitOK: "success", output.ExitPolicyFailure: "validation or policy failure",
		output.ExitInvalidUsage:    "invalid invocation or configuration",
		output.ExitUnsafeGit:       "unsafe Git state",
		output.ExitMissingExec:     "required executable unavailable",
		output.ExitInternal:        "internal failure",
		output.ExitDeclined:        "confirmation declined",
		output.ExitContractVersion: "incompatible contract version",
		output.ExitNoIntegration:   "integration unavailable",
	}
	got := Describe("test").ExitCodes
	if len(got) != len(want) {
		t.Errorf("the document lists %d exit codes and the program has %d", len(got), len(want))
	}
	for code, meaning := range want {
		key := strconv.Itoa(code)
		if got[key] == "" {
			t.Errorf("exit code %d is not described", code)
			continue
		}
		if got[key] != meaning {
			t.Errorf("exit code %d is described as %q, and the program uses it for %q", code, got[key], meaning)
		}
	}
}

func TestEveryEventTypeIsDeclared(t *testing.T) {
	all := []string{
		events.TypeStatus, events.TypeSubagentPre, events.TypeFileRead,
		events.TypeSubagentStrt, events.TypeSubagentStop,
		events.TypeSessionStop, events.TypeSessionEnd,
	}
	declared := map[string]bool{}
	for _, e := range Describe("test").EventTypes {
		declared[e] = true
	}
	for _, e := range all {
		if !declared[e] {
			t.Errorf("event type %q exists but is not declared in the contract", e)
		}
	}
	if len(declared) != len(all) {
		t.Errorf("the contract declares %d event types and the code has %d", len(declared), len(all))
	}
}

// Support is claimed only where it has been observed. A harness marked
// supported must name the version it was tested against, and one that is
// not supported must claim neither the gate nor the status line.
func TestSupportIsOnlyClaimedWhereItWasTested(t *testing.T) {
	for _, h := range Describe("test").Harnesses {
		switch h.Status {
		case "supported":
			if len(h.TestedVersions) == 0 {
				t.Errorf("%s is marked supported but names no tested version", h.Name)
			}
		default:
			if h.GateSupported || h.StatusSupported {
				t.Errorf("%s is %q but claims the gate or the status line", h.Name, h.Status)
			}
			if len(h.TestedVersions) != 0 {
				t.Errorf("%s is %q but names a tested version", h.Name, h.Status)
			}
		}
	}
}

func TestContractVersionIsStated(t *testing.T) {
	d := Describe("1.2.3")
	if d.Version != "1.2.3" {
		t.Errorf("the product version is %q", d.Version)
	}
	if d.ContractVersion != ContractVersion || ContractVersion == "" {
		t.Errorf("the contract version is %q", d.ContractVersion)
	}
	if d.Network || d.ModelCalls || d.Daemon {
		t.Error("the document claims a network call, a model call, or a daemon, and this program makes none")
	}
}
