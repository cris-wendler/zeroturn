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
//
// It says it as a warning. This was a failure, and a failure is a claim
// that something is broken: working in an editor extension is a
// supported way to use the validation half, and there the check could
// never be green while doctor closed by telling a person to fix what
// they had not broken. What is asserted here is that the fact is
// reported and what to do is named, not the colour it is reported in.
func TestDoctorReportsASessionThatCarriedNoMeasurements(t *testing.T) {
	work, _ := repoWithConfig(t, nil)

	// A session where only hooks fired, which is what an extension
	// produces: the subagent gate ran, the status line never did.
	gate(t, work, "extension-session")

	c := doctorCheck(t, work, "session data")
	if c.Status == checkOK {
		t.Fatalf("a session with no measurements reads as ok: %s", c.Detail)
	}
	for _, want := range []string{"no context or usage values", "terminal"} {
		if !strings.Contains(c.Detail, want) {
			t.Errorf("the explanation does not mention %q: %s", want, c.Detail)
		}
	}
}

// A person whose work is in an editor extension has nothing to fix, so
// doctor has to be able to come back green for them. While this was a
// failure it never could, and a check that is permanently red in a
// supported arrangement is one people learn to ignore.
func TestDoctorCanBeGreenInAnEditorExtension(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	gate(t, work, "extension-session")

	r := run(t, work, "", "doctor")
	if r.code != 0 {
		t.Errorf("doctor exits %d where the only complaint is that no status line was drawn:\n%s",
			r.code, r.stdout)
	}
	if strings.Contains(r.stdout, "FAIL") {
		t.Errorf("doctor reports a failure with nothing broken:\n%s", r.stdout)
	}
}

// The full payload, with the usage windows in it. This asserted ok for
// a payload carrying context and NO usage windows, which is the state
// the check is now warning about, so it was encoding the defect: any
// answer that called that case ok was passing this test.
func TestDoctorIsSatisfiedWhenTheStatusLineDelivers(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	statusline(t, work, "terminal-session", nil)

	c := doctorCheck(t, work, "session data")
	if c.Status != checkOK {
		t.Fatalf("status %q, want ok: %s", c.Status, c.Detail)
	}
	if !strings.Contains(c.Detail, "every measurement") {
		t.Errorf("the check does not say what it is satisfied about: %s", c.Detail)
	}
}

// rate_limits can arrive as null, which is a real payload and not a
// broken one. The check counted only the context value, so it reported
// the status line as delivering measurements while the two usage
// windows and the duration were absent and their thresholds could not
// fire. Everything looked installed and two thirds of the gate was
// unreachable.
func TestDoctorSaysWhenTheStatusLineSendsSomeButNotAll(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	statusline(t, work, "terminal-session", contextAt(42))

	c := doctorCheck(t, work, "session data")
	if c.Status != checkWarn {
		t.Fatalf("status %q, want warn: %s", c.Status, c.Detail)
	}
	// It has to name what is absent, not only that something is. This
	// payload carries context and a duration and no usage windows, so
	// naming either of the values that did arrive would be the same
	// fault in the other direction.
	for _, want := range []string{"five hour", "seven day"} {
		if !strings.Contains(c.Detail, want) {
			t.Errorf("the warning does not name %q as missing: %s", want, c.Detail)
		}
	}
	for _, arrived := range []string{"context", "duration"} {
		if strings.Contains(c.Detail, arrived) {
			t.Errorf("%q was measured and the warning calls it missing: %s", arrived, c.Detail)
		}
	}
	if !strings.Contains(c.Detail, "zeroturn ") {
		t.Errorf("the warning names no command to run: %s", c.Detail)
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

// policy check is the command a person runs to ask what the gate would
// do, so it is the one that most needs to tell "below the limit" apart
// from "never measured". Until now only doctor could, which is the
// wrong way round: doctor is where you go when something is broken,
// and this is where you go to read the gate.
func TestPolicyCheckSaysNothingWasMeasured(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	// Hooks fired and the status line never did, which is an extension.
	gate(t, work, "extension-session")

	r := run(t, work, "", "policy", "check")
	if !strings.Contains(r.stdout, "No threshold has been crossed") {
		t.Fatalf("the gate reported a trigger it should not have:\n%s", r.stdout)
	}
	if !strings.Contains(r.stdout, "Nothing was measured") {
		t.Errorf("an all clear was printed for values nobody read:\n%s", r.stdout)
	}
	for _, want := range []string{"context", "five hour", "seven day", "duration"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("the note does not name %q: %s", want, r.stdout)
		}
	}
	// The subagent counts are ZeroTurn's own and really are zero, so
	// sweeping them into the same sentence would be its own overstatement.
	if !strings.Contains(r.stdout, "Subagent counts") {
		t.Errorf("the note does not say which values are still real:\n%s", r.stdout)
	}
}

func TestPolicyCheckNamesOnlyWhatWasNotMeasured(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	statusline(t, work, "terminal-session", contextAt(42))

	r := run(t, work, "", "policy", "check")
	if !strings.Contains(r.stdout, "Not measured in this session") {
		t.Fatalf("the absent values are not reported:\n%s", r.stdout)
	}
	// Context arrived, so calling it unmeasured would be the same fault
	// in the other direction.
	line := ""
	for _, l := range strings.Split(r.stdout, "\n") {
		if strings.HasPrefix(l, "Not measured") {
			line = l
		}
	}
	for _, arrived := range []string{"context", "duration"} {
		if strings.Contains(line, arrived) {
			t.Errorf("%q was measured and is named as absent: %s", arrived, line)
		}
	}
	for _, want := range []string{"five hour", "seven day"} {
		if !strings.Contains(line, want) {
			t.Errorf("the line does not name %q: %s", want, line)
		}
	}
}

// When everything was measured, the all clear is real and saying
// anything else would be noise on the common path.
func TestPolicyCheckAddsNothingWhenEverythingWasMeasured(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	statusline(t, work, "terminal-session", nil)

	r := run(t, work, "", "policy", "check")
	for _, unwanted := range []string{"Not measured", "Nothing was measured"} {
		if strings.Contains(r.stdout, unwanted) {
			t.Errorf("a complete session was reported as missing something:\n%s", r.stdout)
		}
	}
}

// doctor and policy check read the same records, so the two must not
// give a reader opposite impressions of the same session. They did:
// doctor called it a failure and policy check printed an unqualified
// all clear.
func TestDoctorAndPolicyCheckAgreeAboutTheSameSession(t *testing.T) {
	cases := []struct {
		name  string
		setup func(work string)
	}{
		{"nothing measured", func(work string) { gate(t, work, "extension-session") }},
		{"some measured", func(work string) { statusline(t, work, "s", contextAt(42)) }},
		{"all measured", func(work string) { statusline(t, work, "s", nil) }},
	}
	for _, c := range cases {
		work, _ := repoWithConfig(t, nil)
		c.setup(work)

		ck := doctorCheck(t, work, "session data")
		r := run(t, work, "", "policy", "check")
		quiet := !strings.Contains(r.stdout, "measured")

		if ck.Status == checkOK && !quiet {
			t.Errorf("%s: doctor is satisfied and policy check reports something unmeasured:\n%s",
				c.name, r.stdout)
		}
		if ck.Status != checkOK && quiet {
			t.Errorf("%s: doctor reports %q and policy check says nothing about it:\n%s",
				c.name, ck.Status, r.stdout)
		}
	}
}
