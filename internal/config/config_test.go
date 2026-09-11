package config

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDefaultIsValid(t *testing.T) {
	if err := Default().Validate(); err != nil {
		t.Fatal(err)
	}
	if Default().Guard.Mode != ModeObserve {
		t.Fatal("the default mode must be observe")
	}
}

func TestValidateRejects(t *testing.T) {
	cases := map[string]func(*Config){
		"version":                           func(c *Config) { c.Version = 2 },
		"guard.mode":                        func(c *Config) { c.Guard.Mode = "block" },
		"guard.context.warn":                func(c *Config) { c.Guard.Context.Warn = 0 },
		"guard.context.critical":            func(c *Config) { c.Guard.Context.Critical = 101 },
		"guard.limits.fiveHourWarn":         func(c *Config) { c.Guard.Limits.FiveHourWarn = -5 },
		"guard.session.durationWarnMinutes": func(c *Config) { c.Guard.Session.DurationWarnMinutes = 0 },
		"guard.session.activeSubagentsWarn": func(c *Config) { c.Guard.Session.ActiveSubagentsWarn = 0 },
		"verify.steps[0].command":           func(c *Config) { c.Verify.Steps = []Step{{Name: "x"}} },
		"verify.steps[0].command[0]":        func(c *Config) { c.Verify.Steps = []Step{{Name: "x", Command: []string{" "}}} },
		"verify.steps[1].name": func(c *Config) {
			c.Verify.Steps = []Step{{Name: "a", Command: []string{"a"}}, {Name: "a", Command: []string{"b"}}}
		},
		"git.remote": func(c *Config) { c.Git.Remote = "" },
	}
	for field, mutate := range cases {
		c := Default()
		mutate(&c)
		err := c.Validate()
		ve, ok := err.(ValidationError)
		if !ok {
			t.Errorf("%s: got %v, want a ValidationError", field, err)
			continue
		}
		if ve.Field != field {
			t.Errorf("%s: error names field %s", field, ve.Field)
		}
		if ve.Fix == "" {
			t.Errorf("%s: error has no next action", field)
		}
	}
}

func TestWarnMustStayBelowCritical(t *testing.T) {
	c := Default()
	c.Guard.Context.Warn = 85
	if err := c.Validate(); err == nil {
		t.Fatal("warn above confirm was accepted")
	}
	c = Default()
	c.Guard.Context.Confirm = 90
	if err := c.Validate(); err == nil {
		t.Fatal("confirm equal to critical was accepted")
	}
}

func TestLoadRejectsShellStringCommand(t *testing.T) {
	dir := t.TempDir()
	body := `{"version":1,"guard":{"mode":"observe","context":{"warn":70,"confirm":80,"critical":90},
"limits":{"fiveHourWarn":75,"sevenDayWarn":75},"session":{"durationWarnMinutes":240,"activeSubagentsWarn":2,"subagentStartsWarn":4}},
"verify":{"steps":[{"name":"test","command":"npm test && rm -rf /"}]},"git":{"remote":"origin","protectedBranches":["main"]}}`
	if err := ioutil.WriteFile(filepath.Join(dir, FileName), []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil {
		t.Fatal("a shell string command was accepted")
	}
}

func TestLoadRejectsUnknownFields(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, Default()); err != nil {
		t.Fatal(err)
	}
	b, _ := ioutil.ReadFile(Path(dir))
	b = []byte(strings.Replace(string(b), `"version": 1,`, `"version": 1, "shell": "bash",`, 1))
	ioutil.WriteFile(Path(dir), b, 0644)
	if _, err := Load(dir); err == nil {
		t.Fatal("an unknown field was accepted")
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	c := Default()
	c.Verify.Steps = []Step{{Name: "test", Command: []string{"go", "test", "./..."}}}
	if err := Save(dir, c); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Hash() != c.Hash() || got.Guard != c.Guard {
		t.Fatal("round trip changed the configuration")
	}
}

func TestSaveRefusesInvalidAndLeavesFileUntouched(t *testing.T) {
	dir := t.TempDir()
	if err := Save(dir, Default()); err != nil {
		t.Fatal(err)
	}
	before, _ := ioutil.ReadFile(Path(dir))
	bad := Default()
	bad.Guard.Mode = "nope"
	if err := Save(dir, bad); err == nil {
		t.Fatal("invalid configuration was saved")
	}
	after, _ := ioutil.ReadFile(Path(dir))
	if string(before) != string(after) {
		t.Fatal("a refused save changed the file")
	}
}

// An interrupted write must leave either the old file or the new one, and
// no temporary file behind.
func TestAtomicWriteLeavesNoTemporaryFiles(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "f.json")
	for i := 0; i < 3; i++ {
		if err := AtomicWrite(p, []byte("x"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	entries, _ := ioutil.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("directory holds %d entries", len(entries))
	}
	if err := AtomicWrite(filepath.Join(dir, "missing", "f.json"), []byte("x"), 0644); err == nil {
		t.Fatal("write into a missing directory reported success")
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatal(err)
	}
}

func TestHashTracksCommandsNotGuard(t *testing.T) {
	a := Default()
	a.Verify.Steps = []Step{{Name: "test", Command: []string{"npm", "test"}}}
	b := a
	b.Guard.Mode = ModeStrict
	if a.Hash() != b.Hash() {
		t.Fatal("changing guard thresholds must not withdraw command approval")
	}
	c := a
	c.Verify.Steps = []Step{{Name: "test", Command: []string{"npm", "test", "--", "--bail"}}}
	if a.Hash() == c.Hash() {
		t.Fatal("changing a command must change the hash")
	}
	// Argument boundaries are part of the hash, so ["a b"] differs from ["a", "b"].
	d := a
	d.Verify.Steps = []Step{{Name: "test", Command: []string{"npm test"}}}
	if a.Hash() == d.Hash() {
		t.Fatal("argument boundaries are not covered by the hash")
	}
}

func TestIsProtected(t *testing.T) {
	c := Default()
	if !c.IsProtected("main") || c.IsProtected("feature/x") {
		t.Fatal("protected branch matching is wrong")
	}
}

func TestExampleFileIsValid(t *testing.T) {
	dir := t.TempDir()
	b, err := ioutil.ReadFile(filepath.Join("..", "..", ".zeroturn.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	ioutil.WriteFile(Path(dir), b, 0644)
	if _, err := Load(dir); err != nil {
		t.Fatalf(".zeroturn.example.json does not load: %v", err)
	}
}
