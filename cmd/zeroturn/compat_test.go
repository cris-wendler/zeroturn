package main

import (
	"encoding/json"
	"strings"
	"testing"
)

func doctorChecks(t *testing.T, work string, args ...string) []check {
	t.Helper()
	r := run(t, work, "", args...)
	var checks []check
	if err := json.Unmarshal([]byte(r.stdout), &checks); err != nil {
		t.Fatalf("%v: %v\n%s", args, err, r.all())
	}
	return checks
}

func named(checks []check, name string) (check, bool) {
	for _, c := range checks {
		if c.Name == name {
			return c, true
		}
	}
	return check{}, false
}

// doctor --compat is what tells somebody that each mode still produces
// the decision it claims. Nothing checked that it still checks, so a
// version of it that quietly stopped asserting anything would have
// reported success to everyone who ran it.
func TestCompatProvesEveryModeStillDecides(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	checks := doctorChecks(t, work, "doctor", "--compat", "--json")

	want := map[string]string{
		"policy observe":  "allow",
		"policy confirm":  "ask",
		"policy strict":   "deny",
		"event redaction": "",
	}
	for name, decision := range want {
		c, ok := named(checks, name)
		if !ok {
			t.Errorf("--compat no longer reports %q", name)
			continue
		}
		if c.Status != checkOK {
			t.Errorf("%s: %s, %s", name, c.Status, c.Detail)
		}
		if decision != "" && !strings.Contains(c.Detail, decision) {
			t.Errorf("%s does not say it produced %q: %s", name, decision, c.Detail)
		}
	}
}

// Without --live it must say the live test was not run rather than let a
// reader believe the harness was asked anything.
func TestCompatSaysTheLiveTestWasNotRun(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	c, ok := named(doctorChecks(t, work, "doctor", "--compat", "--json"), "live harness test")
	if !ok {
		t.Fatal("--compat does not mention the live test")
	}
	if c.Status != checkWarn || !strings.Contains(c.Detail, "not run") {
		t.Fatalf("%s: %s", c.Status, c.Detail)
	}
}

// The flag has to add the checks. A --compat that returned the ordinary
// ones would look like it passed.
func TestCompatAddsChecksToTheOrdinaryRun(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	plain := doctorChecks(t, work, "doctor", "--json")
	compat := doctorChecks(t, work, "doctor", "--compat", "--json")
	if len(compat) <= len(plain) {
		t.Fatalf("--compat reported %d checks, the ordinary run reported %d", len(compat), len(plain))
	}
	for _, name := range []string{"policy observe", "policy confirm", "policy strict"} {
		if _, ok := named(plain, name); ok {
			t.Errorf("%q is reported without --compat", name)
		}
	}
}

// The live test reads two counts from the session it started, and there
// are three answers in them. Running it against Claude Code 2.1.277
// produced no denials and no subagents, because the session never tried
// to start one, and the check reported that the harness had ignored a
// denial. Nothing had been put to the harness to ignore.
func TestTheLiveTestSaysWhenNothingWasPutToTheHarness(t *testing.T) {
	for _, c := range []struct {
		name            string
		denials, spawns int
		want            string
		says            string
	}{
		{"a denial was honoured", 1, 0, checkOK, "honoured"},
		// The two failures are both failures and they are not the same
		// reading. One says the harness was given a denial and went
		// ahead; the other says a subagent started with no denial in the
		// record at all, which is a different thing to look into.
		{"a denial was ignored", 1, 1, checkFail, "given a denial"},
		{"a subagent started with no denial recorded", 0, 1, checkFail, "no denial was recorded"},
		{"nothing was asked for", 0, 0, checkWarn, "never asked"},
	} {
		got := judgeLive(c.denials, c.spawns)
		if got.Status != c.want {
			t.Errorf("%s: %s, want %s (%s)", c.name, got.Status, c.want, got.Detail)
		}
		if !strings.Contains(got.Detail, c.says) {
			t.Errorf("%s: detail does not say %q: %s", c.name, c.says, got.Detail)
		}
	}

	// The one that is not a failure must still name what to do, which is
	// the rule every warning in doctor follows.
	if d := judgeLive(0, 0).Detail; !strings.Contains(d, "zeroturn doctor") {
		t.Errorf("the warning names no command: %s", d)
	}
}

// Only a denial of the subagent tool says anything about this gate. The
// live session runs with --restricted, which refuses tools of its own
// accord, and counting one of those as proof the gate was honoured is
// the same defect as the one judgeLive exists to avoid, one layer up.
//
// The two names are not interchangeable in practice: the hook payload
// this repository sends says Agent, and the session result observed on
// Claude Code 2.1.277 said Task. Counting only the first would have made
// every real run report that nothing was asked.
func TestOnlyADenialOfTheSubagentToolCounts(t *testing.T) {
	for _, c := range []struct {
		name  string
		given []string
		want  int
	}{
		{"what a real session reported", []string{"Task"}, 1},
		{"what a hook payload calls it", []string{"Agent"}, 1},
		{"a tool restricted mode refused", []string{"Bash"}, 0},
		{"the gate's denial among others", []string{"Bash", "Task", "WebFetch"}, 1},
		{"nothing denied", nil, 0},
	} {
		if got := subagentDenials(c.given); got != c.want {
			t.Errorf("%s: %v counted as %d, want %d", c.name, c.given, got, c.want)
		}
	}
}
