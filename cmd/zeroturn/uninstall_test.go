package main

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/testutil"
)

// installed puts ZeroTurn on a throwaway machine the way a person would:
// a policy file for the repository, the repository integration, and the
// user wide one. It returns the repository root.
func installed(t *testing.T) string {
	t.Helper()
	testutil.Isolate(t)
	work, _ := testutil.Remote(t)
	testutil.Write(t, work, "go.mod", "module example.com/app\n\ngo 1.17\n")

	if err := inProcess(t, work, true, func() error {
		return cmdInit(context.Background(), []string{"--yes"})
	}); err != nil {
		t.Fatalf("init: %v", err)
	}
	if err := integrate(t, work, true, "--apply"); err != nil {
		t.Fatalf("integrate: %v", err)
	}
	if err := integrate(t, work, true, "--apply", "--user"); err != nil {
		t.Fatalf("integrate --user: %v", err)
	}
	return work
}

// capture runs a command in dir with stdout collected, so a test can read
// what a person would have seen.
func capture(t *testing.T, dir string, answer bool, fn func() error) (string, error) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan string, 1)
	go func() {
		b, _ := ioutil.ReadAll(r)
		done <- string(b)
	}()

	orig := confirm
	confirm = func(string) (bool, error) { return answer, nil }
	wd, _ := os.Getwd()
	if cerr := os.Chdir(dir); cerr != nil {
		t.Fatal(cerr)
	}
	stdout := os.Stdout
	os.Stdout = w

	cmdErr := fn()

	os.Stdout = stdout
	w.Close()
	os.Chdir(wd)
	confirm = orig
	return <-done, cmdErr
}

// zeroTurnLeftovers walks a tree and reports every file that still names
// ZeroTurn, by its path or by what is inside it. The list is derived from
// the disk rather than written out here, so a file ZeroTurn starts writing
// later is covered by this test without anyone remembering to add it.
func zeroTurnLeftovers(t *testing.T, root string) []string {
	t.Helper()
	var found []string
	filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		// Git's own storage holds the test's commits and is not ZeroTurn's
		// to clean.
		if info.IsDir() {
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			if strings.Contains(strings.ToLower(info.Name()), "zeroturn") {
				found = append(found, path)
			}
			return nil
		}
		if strings.Contains(strings.ToLower(info.Name()), "zeroturn") {
			found = append(found, path)
			return nil
		}
		b, rerr := ioutil.ReadFile(path)
		if rerr != nil {
			return nil
		}
		if strings.Contains(strings.ToLower(string(b)), "zeroturn") {
			found = append(found, path)
		}
		return nil
	})
	return found
}

// The promise uninstall makes is that a machine can be put back. Nothing
// here names the files: the check walks what is on disk, so it fails for
// anything ZeroTurn writes, not only for what it wrote the day this test
// was written.
func TestUninstallRemovesEverythingAnInstallLeft(t *testing.T) {
	work := installed(t)
	home := os.Getenv("HOME")
	stateDir := os.Getenv("ZEROTURN_STATE_DIR")

	if len(zeroTurnLeftovers(t, home)) == 0 {
		t.Fatal("the install left nothing in the home directory, so this test proves nothing")
	}
	if _, err := os.Stat(stateDir); err != nil {
		t.Fatalf("the install created no state directory: %v", err)
	}

	out, err := capture(t, work, true, func() error { return cmdUninstall([]string{"--apply"}) })
	if err != nil {
		t.Fatalf("uninstall: %v\n%s", err, out)
	}

	for _, tree := range []string{home, work} {
		if left := zeroTurnLeftovers(t, tree); len(left) > 0 {
			t.Errorf("uninstall left %v", left)
		}
	}
	if _, err := os.Stat(stateDir); !os.IsNotExist(err) {
		t.Errorf("the state directory is still there: %v", err)
	}
}

// A person reads the list before deciding, so the list has to be right
// while nothing has changed yet.
func TestUninstallWithoutApplyChangesNothing(t *testing.T) {
	work := installed(t)
	home := os.Getenv("HOME")
	before := zeroTurnLeftovers(t, home)

	out, err := capture(t, work, true, func() error { return cmdUninstall(nil) })
	if err != nil {
		t.Fatalf("uninstall: %v\n%s", err, out)
	}

	after := zeroTurnLeftovers(t, home)
	if len(after) != len(before) {
		t.Errorf("the plan changed the machine: %d files before, %d after", len(before), len(after))
	}
	if !strings.Contains(out, "Nothing has changed") {
		t.Errorf("the plan did not say it changed nothing:\n%s", out)
	}
	// The state directory is compared whole because the command reads it
	// from the same environment variable this test set. The other three
	// are matched on their last elements, on an item line: Windows hands
	// a program the long form of a path and a test the short one, and
	// both name the same file.
	for _, want := range []string{
		`(?m)^  delete\s+.*` + regexp.QuoteMeta(config.FileName) + `$`,
		`(?m)^  edit\s+.*` + regexp.QuoteMeta(filepath.Join(".claude", "settings.json")) + `$`,
		`(?m)^  edit\s+.*` + regexp.QuoteMeta(filepath.Join(".claude", "settings.local.json")) + `$`,
		`(?m)^  delete\s+` + regexp.QuoteMeta(os.Getenv("ZEROTURN_STATE_DIR")) + `$`,
	} {
		if !regexp.MustCompile(want).MatchString(out) {
			t.Errorf("the plan has no item matching %s:\n%s", want, out)
		}
	}
}

// Taking ZeroTurn out must not take anything else with it. This is the
// same promise integrate --remove makes, and uninstall reaches more files.
func TestUninstallKeepsWhatItDoesNotOwn(t *testing.T) {
	work := installed(t)
	path := filepath.Join(work, ".claude", "settings.local.json")

	raw, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatal(err)
	}
	var hooks map[string][]json.RawMessage
	if err := json.Unmarshal(top["hooks"], &hooks); err != nil {
		t.Fatal(err)
	}
	mine := json.RawMessage(`{"matcher":"Bash","hooks":[{"type":"command","command":"my-own-audit"}]}`)
	hooks["PreToolUse"] = append(hooks["PreToolUse"], mine)
	b, err := json.Marshal(hooks)
	if err != nil {
		t.Fatal(err)
	}
	top["hooks"] = json.RawMessage(b)
	top["someOtherTool"] = json.RawMessage(`{"kept":true}`)
	out, err := json.Marshal(top)
	if err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(path, out, 0600); err != nil {
		t.Fatal(err)
	}

	if _, err := capture(t, work, true, func() error { return cmdUninstall([]string{"--apply"}) }); err != nil {
		t.Fatalf("uninstall: %v", err)
	}

	after, err := ioutil.ReadFile(path)
	if err != nil {
		t.Fatalf("uninstall deleted a settings file it does not own: %v", err)
	}
	for _, want := range []string{"my-own-audit", "someOtherTool"} {
		if !strings.Contains(string(after), want) {
			t.Errorf("uninstall removed %s:\n%s", want, after)
		}
	}
	if strings.Contains(strings.ToLower(string(after)), "zeroturn") {
		t.Errorf("uninstall left its own entries:\n%s", after)
	}
}

// Declining has to mean nothing happened, not most of nothing.
func TestUninstallDeclinedChangesNothing(t *testing.T) {
	work := installed(t)
	home := os.Getenv("HOME")
	before := len(zeroTurnLeftovers(t, home))

	if _, err := capture(t, work, false, func() error { return cmdUninstall([]string{"--apply"}) }); err == nil {
		t.Fatal("declining the removal reported success")
	}
	if after := len(zeroTurnLeftovers(t, home)); after != before {
		t.Errorf("declining still changed the machine: %d files before, %d after", before, after)
	}
	if _, err := os.Stat(os.Getenv("ZEROTURN_STATE_DIR")); err != nil {
		t.Errorf("declining deleted the state directory: %v", err)
	}
}

// Running it twice is how somebody checks the first run worked, and it
// must not read as a failure.
func TestUninstallOnAMachineWithNothingOnIt(t *testing.T) {
	testutil.Isolate(t)
	work, _ := testutil.Remote(t)

	out, err := capture(t, work, true, func() error { return cmdUninstall([]string{"--apply"}) })
	if err != nil {
		t.Fatalf("uninstall on a clean machine failed: %v\n%s", err, out)
	}
	if !strings.Contains(out, "nothing on this machine") {
		t.Errorf("the message does not say there was nothing to do:\n%s", out)
	}
}

// The executable is the one thing uninstall will not delete, and a person
// left without the path has no way to finish.
func TestUninstallSaysWhereTheExecutableIs(t *testing.T) {
	work := installed(t)
	out, err := capture(t, work, true, func() error { return cmdUninstall(nil) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, selfPath()) {
		t.Errorf("the plan does not say where the executable is:\n%s", out)
	}
}
