package main

import (
	"encoding/json"
	"strings"
	"testing"
)

// doctorCheck runs doctor and returns the named check.
func doctorCheck(t *testing.T, work, name string) check {
	t.Helper()
	r := run(t, work, "", "doctor", "--json")
	var checks []check
	if err := json.Unmarshal([]byte(r.stdout), &checks); err != nil {
		t.Fatalf("doctor --json: %v\n%s", err, r.all())
	}
	for _, c := range checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("doctor has no %q check:\n%s", name, r.stdout)
	return check{}
}

// Context use and the usage windows arrive through the status line and
// through nothing else. An editor extension draws no status line, so it
// never invokes the command, and the guard then runs with no
// measurements while still looking installed. Doctor has to say so,
// because nothing else will.
func TestDoctorReportsASessionThatCarriedNoMeasurements(t *testing.T) {
	work, _ := repoWithConfig(t, nil)

	// A session where only hooks fired, which is what an extension
	// produces: the subagent gate ran, the status line never did.
	gate(t, work, "extension-session")

	c := doctorCheck(t, work, "session data")
	if c.Status != checkFail {
		t.Fatalf("status %q, want fail: %s", c.Status, c.Detail)
	}
	for _, want := range []string{"no context or usage values", "terminal"} {
		if !strings.Contains(c.Detail, want) {
			t.Errorf("the explanation does not mention %q: %s", want, c.Detail)
		}
	}
}

func TestDoctorIsSatisfiedWhenTheStatusLineDelivers(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	statusline(t, work, "terminal-session", contextAt(42))

	c := doctorCheck(t, work, "session data")
	if c.Status != checkOK {
		t.Fatalf("status %q, want ok: %s", c.Status, c.Detail)
	}
	if !strings.Contains(c.Detail, "42%") {
		t.Errorf("the measurement is not reported: %s", c.Detail)
	}
}

// Working in a terminal sometimes and an editor the rest of the time is
// the case that hides the problem, because the good sessions make it
// look like everything works.
func TestDoctorWarnsWhenOnlySomeSessionsCarriedMeasurements(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	statusline(t, work, "terminal-session", contextAt(42))
	gate(t, work, "extension-session")

	c := doctorCheck(t, work, "session data")
	if c.Status != checkWarn {
		t.Fatalf("status %q, want warn: %s", c.Status, c.Detail)
	}
	if !strings.Contains(c.Detail, "1 of 2") {
		t.Errorf("the count is not reported: %s", c.Detail)
	}
}

func TestDoctorSaysSoWhenThereIsNothingToJudge(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	c := doctorCheck(t, work, "session data")
	if c.Status != checkWarn {
		t.Fatalf("status %q, want warn: %s", c.Status, c.Detail)
	}
	if !strings.Contains(c.Detail, "no sessions recorded") {
		t.Errorf("detail %q", c.Detail)
	}
}
