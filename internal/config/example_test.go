package config

import (
	"io/ioutil"
	"path/filepath"
	"reflect"
	"testing"
)

// .zeroturn.example.json is what somebody reads to see what the file
// looks like before they have one. It was checked only for loading, so
// every value in it could drift from the value a new file would get and
// nothing would notice. An example that disagrees with the tool is worse
// than no example, because it is read as a statement of the defaults.
//
// The validation steps are deliberately not compared. They are an
// illustration of a JavaScript project and are meant to differ from
// whatever init would propose here; the point of the example is the
// shape of the file and the settings around them.
func TestTheExampleFileShowsTheDefaults(t *testing.T) {
	dir := t.TempDir()
	b, err := ioutil.ReadFile(filepath.Join("..", "..", ".zeroturn.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(Path(dir), b, 0644); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatalf(".zeroturn.example.json does not load: %v", err)
	}

	want := Default()
	// The one part that is allowed to differ, on both sides, so the
	// comparison is about everything else.
	got.Verify.Steps = nil
	want.Verify.Steps = nil

	if !reflect.DeepEqual(got, want) {
		t.Errorf("the example file does not show the defaults.\n example: %+v\ndefaults: %+v", got, want)
	}
}

// The steps are excluded from the comparison above, so something has to
// hold them. An example with no steps at all would show a file that
// validates nothing, which is not what the file is for.
func TestTheExampleFileStillShowsValidationSteps(t *testing.T) {
	dir := t.TempDir()
	b, err := ioutil.ReadFile(filepath.Join("..", "..", ".zeroturn.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(Path(dir), b, 0644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Verify.Steps) == 0 {
		t.Error("the example file shows no validation steps, so it shows a file that checks nothing")
	}
	for _, s := range c.Verify.Steps {
		if s.Name == "" || len(s.Command) == 0 {
			t.Errorf("a step in the example is incomplete: %+v", s)
		}
	}
}
