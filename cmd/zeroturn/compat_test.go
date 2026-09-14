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
