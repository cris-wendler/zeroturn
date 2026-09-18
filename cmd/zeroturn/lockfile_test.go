package main

import (
	"os/exec"
	"testing"

	"io/ioutil"
	"path/filepath"
)

// Ranging over a map is randomised, so the same files gave different
// answers on different runs.
// nodePackageManager only stats files, so these need a directory and
// not a repository.
func write(t *testing.T, dir, name string) {
	t.Helper()
	if err := ioutil.WriteFile(filepath.Join(dir, name), []byte("{}"), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestTwoLockfilesGiveTheSameAnswerEveryTime(t *testing.T) {
	work := t.TempDir()
	write(t, work, "yarn.lock")
	write(t, work, "package-lock.json")

	first := nodePackageManager(work)
	for i := 0; i < 50; i++ {
		if got := nodePackageManager(work); got != first {
			t.Fatalf("run %d answered %q and the first answered %q, for the same files", i, got, first)
		}
	}
}

func TestTheMoreSpecificLockfileWins(t *testing.T) {
	installed := map[string]bool{}
	for _, l := range lockfiles {
		if _, err := exec.LookPath(l.manager); err == nil {
			installed[l.manager] = true
		}
	}

	cases := []struct {
		name  string
		files []string
		want  string
	}{
		{"yarn beats npm", []string{"yarn.lock", "package-lock.json"}, "yarn"},
		{"pnpm beats yarn", []string{"pnpm-lock.yaml", "yarn.lock"}, "pnpm"},
		{"pnpm beats everything", []string{"pnpm-lock.yaml", "yarn.lock", "package-lock.json", "bun.lockb"}, "pnpm"},
	}
	ran := 0
	for _, c := range cases {
		if !installed[c.want] {
			continue // the winner is not on this machine, so it cannot be returned
		}
		ran++
		work := t.TempDir()
		for _, f := range c.files {
			write(t, work, f)
		}
		if got := nodePackageManager(work); got != c.want {
			t.Errorf("%s: got %q", c.name, got)
		}
	}
	if ran == 0 {
		t.Skip("none of the package managers under test are installed here")
	}
}

func TestNoLockfileFallsBackToNpm(t *testing.T) {
	work := t.TempDir()
	if got := nodePackageManager(work); got != "npm" {
		t.Errorf("no lockfile gave %q", got)
	}
}

// An empty list would make every answer npm and every test above pass
// for the wrong reason.
func TestTheLockfileListIsNotEmpty(t *testing.T) {
	if len(lockfiles) == 0 {
		t.Fatal("no lockfiles are recognised, so the detection checks nothing")
	}
	seen := map[string]bool{}
	for _, l := range lockfiles {
		if l.file == "" || l.manager == "" {
			t.Errorf("an incomplete entry: %+v", l)
		}
		if seen[l.file] {
			t.Errorf("%s is listed twice, so the later one can never win", l.file)
		}
		seen[l.file] = true
	}
}
