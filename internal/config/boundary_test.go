package config

import (
	"io/ioutil"
	"path/filepath"
	"testing"
)

// These cover conditions in the configuration package that scripts/mutate
// could change with no test disagreeing. This package decides what a
// guard is allowed to do, so a rule here with nothing holding it is a
// rule that can quietly stop applying.

// Exists answers whether a repository has been set up at all, and every
// command asks it before doing anything. Inverting it left every test
// passing.
func TestExistsAnswersBothWays(t *testing.T) {
	dir := t.TempDir()
	if Exists(dir) {
		t.Error("a directory with no configuration was reported as having one")
	}
	if err := Save(dir, Default()); err != nil {
		t.Fatal(err)
	}
	if !Exists(dir) {
		t.Error("a directory with a configuration was reported as having none")
	}
}

// A percentage runs from 1 to 100 and both ends are inside the range.
func TestThePercentageRangeIncludesItsEnds(t *testing.T) {
	for _, v := range []int{1, 50, 100} {
		if err := pct("guard.context.warn", v); err != nil {
			t.Errorf("%d is inside the range and was refused: %v", v, err)
		}
	}
	for _, v := range []int{0, -1, 101, 1000} {
		if err := pct("guard.context.warn", v); err == nil {
			t.Errorf("%d is outside the range and was accepted", v)
		}
	}
}

// The three context thresholds have to stay in order, and two that are
// equal are not in order: a warning and a confirmation at the same
// reading means one of them never happens.
func TestTheContextThresholdsMustDiffer(t *testing.T) {
	equalWarn := Default()
	equalWarn.Guard.Context = Context{Warn: 80, Confirm: 80, Critical: 90}
	if err := equalWarn.Validate(); err == nil {
		t.Error("warn equal to confirm was accepted")
	}

	equalConfirm := Default()
	equalConfirm.Guard.Context = Context{Warn: 70, Confirm: 90, Critical: 90}
	if err := equalConfirm.Validate(); err == nil {
		t.Error("confirm equal to critical was accepted")
	}

	// One apart is in order, and is the tightest arrangement allowed.
	tight := Default()
	tight.Guard.Context = Context{Warn: 88, Confirm: 89, Critical: 90}
	if err := tight.Validate(); err != nil {
		t.Errorf("thresholds one apart were refused: %v", err)
	}
}

// A count of one is a count. These read "value must be at least 1", and
// each of the three could be changed to mean at least 2 with nothing
// disagreeing.
func TestASessionSettingOfOneIsAllowed(t *testing.T) {
	for name, set := range map[string]func(*Config, int){
		"guard.session.durationWarnMinutes": func(c *Config, v int) { c.Guard.Session.DurationWarnMinutes = v },
		"guard.session.activeSubagentsWarn": func(c *Config, v int) { c.Guard.Session.ActiveSubagentsWarn = v },
		"guard.session.subagentStartsWarn":  func(c *Config, v int) { c.Guard.Session.SubagentStartsWarn = v },
	} {
		one := Default()
		set(&one, 1)
		if err := one.Validate(); err != nil {
			t.Errorf("%s of 1 was refused: %v", name, err)
		}

		zero := Default()
		set(&zero, 0)
		if err := zero.Validate(); err == nil {
			t.Errorf("%s of 0 was accepted", name)
		}
	}
}

// Step finds a validation step by name, and ship and verify both ask it
// before running anything.
func TestStepFindsOnlyTheStepItWasAskedFor(t *testing.T) {
	c := Default()
	c.Verify.Steps = []Step{
		{Name: "vet", Command: []string{"go", "vet", "./..."}},
		{Name: "test", Command: []string{"go", "test", "./..."}},
	}
	got, ok := c.Step("test")
	if !ok {
		t.Fatal("a step that is there was not found")
	}
	if got.Name != "test" {
		t.Errorf("asked for test and got %q", got.Name)
	}
	if _, ok := c.Step("lint"); ok {
		t.Error("a step that is not there was found")
	}
}

// Changed decides whether policy migrate writes anything. Each of the
// three reasons has to be enough on its own.
func TestEachReasonToRewriteIsEnoughOnItsOwn(t *testing.T) {
	for name, m := range map[string]Migration{
		"an older version":     {From: 0},
		"a setting was filled": {From: Version, Filled: []string{"guard.mode"}},
		"a setting is unknown": {From: Version, Unknown: []string{"guard.nonsense"}},
	} {
		if !m.Changed() {
			t.Errorf("%s does not count as a change", name)
		}
	}
	if (Migration{From: Version}).Changed() {
		t.Error("a file that needs nothing reports a change")
	}
}

// A section written as null is not a section. It has to be reported
// rather than walked into, or a file that says guard is nothing would
// look like a file that says nothing about the guard.
func TestASectionWrittenAsNullIsReported(t *testing.T) {
	dir := t.TempDir()
	if err := ioutil.WriteFile(filepath.Join(dir, FileName),
		[]byte(`{"version":1,"guard":null}`), 0600); err != nil {
		t.Fatal(err)
	}

	_, m, err := Migrate([]byte(`{"version":1,"guard":null}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Unknown) != 1 || m.Unknown[0] != "guard" {
		t.Errorf("a null section was reported as %v, want guard", m.Unknown)
	}

	// And Load refuses it rather than running on the defaults in silence.
	if _, err := Load(dir); err == nil {
		t.Error("a file with a null section loaded as though it were valid")
	}
}
