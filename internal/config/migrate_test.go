package config

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"strings"
	"testing"
)

// tree renders a configuration as the nested maps a file holds, so a test
// can remove or add one value by the name a person would type.
func tree(t *testing.T, c Config) map[string]interface{} {
	t.Helper()
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

// drop removes one dotted path from the tree, and reports whether it was
// there. A test that silently removes nothing proves nothing.
func drop(node map[string]interface{}, path string) bool {
	parts := strings.Split(path, ".")
	for _, p := range parts[:len(parts)-1] {
		child, ok := node[p].(map[string]interface{})
		if !ok {
			return false
		}
		node = child
	}
	last := parts[len(parts)-1]
	if _, ok := node[last]; !ok {
		return false
	}
	delete(node, last)
	return true
}

func set(node map[string]interface{}, path string, value interface{}) {
	parts := strings.Split(path, ".")
	for _, p := range parts[:len(parts)-1] {
		child, ok := node[p].(map[string]interface{})
		if !ok {
			child = map[string]interface{}{}
			node[p] = child
		}
		node = child
	}
	node[parts[len(parts)-1]] = value
}

func save(t *testing.T, node map[string]interface{}) string {
	t.Helper()
	dir := t.TempDir()
	b, err := json.Marshal(node)
	if err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(Path(dir), b, 0600); err != nil {
		t.Fatal(err)
	}
	return dir
}

// This is the promise migration makes, and it is checked against the key
// registry rather than a list written here, so a setting added later is
// covered without anyone remembering this test. Before migration existed,
// three settings had a hand written case each in Load and a fourth would
// have loaded as zero and been refused as out of range.
func TestEverySettingCanBeMissingFromTheFile(t *testing.T) {
	for _, k := range Keys() {
		node := tree(t, Default())
		if !drop(node, k.Name) {
			t.Errorf("%s: the default configuration does not write this setting, so dropping it tests nothing", k.Name)
			continue
		}
		dir := save(t, node)

		c, err := Load(dir)
		if err != nil {
			t.Errorf("%s: a file without it does not load: %v", k.Name, err)
			continue
		}
		if got, want := k.Value(c), k.Value(Default()); got != want {
			t.Errorf("%s: a file without it loaded as %q, want the default %q", k.Name, got, want)
		}
	}
}

// What was filled has to be reported, because policy migrate writes those
// values into the file and a person is entitled to see which.
func TestAMissingSettingIsReported(t *testing.T) {
	for _, k := range Keys() {
		node := tree(t, Default())
		drop(node, k.Name)
		b, err := json.Marshal(node)
		if err != nil {
			t.Fatal(err)
		}
		_, report, err := Migrate(b)
		if err != nil {
			t.Errorf("%s: %v", k.Name, err)
			continue
		}
		if len(report.Filled) != 1 || report.Filled[0] != k.Name {
			t.Errorf("%s: the report names %v", k.Name, report.Filled)
		}
	}
}

// A file that carries a value must come out holding it. Filling in what is
// missing is worth nothing if it overwrites what is there.
func TestMigrationKeepsEveryValueTheFileCarries(t *testing.T) {
	node := tree(t, Default())
	want := map[string]string{}
	for _, k := range Keys() {
		var v interface{}
		switch k.Kind {
		case KindChoice:
			for _, c := range k.Choices {
				if c != k.Value(Default()) {
					v = c
				}
			}
		case KindText:
			v = k.Value(Default()) + "-other"
		case KindPercent:
			v = 42
		default:
			v = 7
		}
		set(node, k.Name, v)
		want[k.Name] = strings.TrimSpace(strings.Trim(jsonText(t, v), `"`))
	}
	b, err := json.Marshal(node)
	if err != nil {
		t.Fatal(err)
	}
	c, report, err := Migrate(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Filled) != 0 {
		t.Errorf("a complete file reported missing settings: %v", report.Filled)
	}
	for _, k := range Keys() {
		if got := k.Value(c); got != want[k.Name] {
			t.Errorf("%s came back as %q, want %q", k.Name, got, want[k.Name])
		}
	}
}

func jsonText(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A fragment somebody typed by hand is the oldest file there is.
func TestAFileWithNothingInItLoadsAsTheDefaults(t *testing.T) {
	dir := save(t, map[string]interface{}{})

	c, err := Load(dir)
	if err != nil {
		t.Fatalf("an empty object does not load: %v", err)
	}
	for _, k := range Keys() {
		if got, want := k.Value(c), k.Value(Default()); got != want {
			t.Errorf("%s is %q, want the default %q", k.Name, got, want)
		}
	}
	if c.Git.Remote != Default().Git.Remote {
		t.Errorf("git remote is %q, want the default", c.Git.Remote)
	}
}

// The old message for a version this build does not know told a person to
// run init, which answers the problem by overwriting the policy they
// wrote. It is the one action they cannot undo.
func TestANewerFileIsNotAnsweredByOverwritingIt(t *testing.T) {
	node := tree(t, Default())
	node["version"] = Version + 1
	b, err := json.Marshal(node)
	if err != nil {
		t.Fatal(err)
	}

	_, _, err = Migrate(b)
	var newer ErrNewer
	if !errors.As(err, &newer) {
		t.Fatalf("a newer file gave %v, want ErrNewer", err)
	}
	if newer.FileVersion != Version+1 || newer.BuildVersion != Version {
		t.Errorf("the error says file %d build %d", newer.FileVersion, newer.BuildVersion)
	}
	if strings.Contains(newer.Error(), "init") {
		t.Errorf("the message still suggests overwriting the file: %q", newer.Error())
	}
}

// A setting that does not exist, in a file of the current version, is a
// spelling mistake. Ignoring it would leave somebody believing a value
// applies when nothing reads it.
func TestAnUnknownSettingAtThisVersionIsRefused(t *testing.T) {
	node := tree(t, Default())
	set(node, "guard.context.warm", 70)
	dir := save(t, node)

	_, err := Load(dir)
	if err == nil {
		t.Fatal("a misspelled setting was accepted")
	}
	if !strings.Contains(err.Error(), "guard.context.warm") {
		t.Errorf("the error does not name it: %v", err)
	}
}

// The same value in an older file is a setting a later version removed,
// which is exactly what migration is for. It is dropped and named.
func TestAnUnknownSettingInAnOlderFileIsCarriedPastAndNamed(t *testing.T) {
	node := tree(t, Default())
	delete(node, "version")
	set(node, "guard.limits.tenDayWarn", 60)
	b, err := json.Marshal(node)
	if err != nil {
		t.Fatal(err)
	}

	c, report, err := Migrate(b)
	if err != nil {
		t.Fatalf("an older file with a removed setting does not load: %v", err)
	}
	if c.Guard.Mode != Default().Guard.Mode {
		t.Errorf("mode is %q", c.Guard.Mode)
	}
	if len(report.Unknown) != 1 || report.Unknown[0] != "guard.limits.tenDayWarn" {
		t.Errorf("the report names %v", report.Unknown)
	}
	if report.From != 0 {
		t.Errorf("the report says version %d, want 0", report.From)
	}
}

// Absent and empty were the same thing before migration, because Go reads
// a missing string as an empty one. They are different mistakes: one is an
// old file, the other is a value somebody typed wrong.
func TestAnEmptyChoiceIsStillRefused(t *testing.T) {
	node := tree(t, Default())
	set(node, "guard.mode", "")
	dir := save(t, node)

	if _, err := Load(dir); err == nil {
		t.Fatal("an empty guard mode was accepted as if the setting were absent")
	}
}

// Nothing to do has to read as nothing to do, or policy migrate would
// rewrite a file every time it ran.
func TestAFileThisBuildWroteNeedsNoMigration(t *testing.T) {
	b, err := json.Marshal(Default())
	if err != nil {
		t.Fatal(err)
	}
	_, report, err := Migrate(b)
	if err != nil {
		t.Fatal(err)
	}
	if report.Changed() {
		t.Errorf("a file this build wrote reports changes: %+v", report)
	}
}
