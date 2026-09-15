package verify

import (
	"context"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/config"
)

// In a linked worktree and in a submodule, .git is a file holding the
// path of the real Git directory. verify wrote its logs to
// <root>/.git/zeroturn/logs, so creating that directory failed with "not
// a directory" and verify and ship did not run at all. The error a person
// saw said the .git directory was not writable, which sent them looking
// at permissions.
func TestTheLogDirectoryFollowsAGitFile(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "linked")
	real := filepath.Join(dir, "real.git", "worktrees", "linked")
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(real, 0700); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(filepath.Join(root, ".git"),
		[]byte("gitdir: "+real+"\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if got, want := GitDir(root), real; got != want {
		t.Fatalf("GitDir is %q, want %q", got, want)
	}
	if got, want := LogDir(root), filepath.Join(real, "zeroturn", "logs"); got != want {
		t.Errorf("LogDir is %q, want %q", got, want)
	}

	// The whole point is that a run gets that far at all.
	res, err := Run(context.Background(), config.Default(), Options{RepoRoot: root})
	if err != nil {
		t.Fatalf("a run in a worktree failed before it started: %v", err)
	}
	if len(res.Steps) != 0 {
		t.Errorf("the default configuration has no steps, and %d ran", len(res.Steps))
	}
	if _, err := os.Stat(LogDir(root)); err != nil {
		t.Errorf("the log directory was not created: %v", err)
	}
}

// A relative gitdir is what Git writes for a submodule.
func TestARelativeGitFileIsResolvedAgainstTheWorkingTree(t *testing.T) {
	dir := t.TempDir()
	root := filepath.Join(dir, "sub")
	if err := os.MkdirAll(filepath.Join(dir, "parent.git", "modules", "sub"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(filepath.Join(root, ".git"),
		[]byte("gitdir: ../parent.git/modules/sub\n"), 0600); err != nil {
		t.Fatal(err)
	}

	want := filepath.Join(dir, "parent.git", "modules", "sub")
	if got := GitDir(root); got != want {
		t.Errorf("GitDir is %q, want %q", got, want)
	}
}

// An ordinary repository has to keep working, and anything unreadable
// has to fall back rather than produce an empty path.
func TestAnOrdinaryRepositoryAndTheOddCases(t *testing.T) {
	dir := t.TempDir()

	ordinary := filepath.Join(dir, "plain")
	if err := os.MkdirAll(filepath.Join(ordinary, ".git"), 0700); err != nil {
		t.Fatal(err)
	}
	if got, want := GitDir(ordinary), filepath.Join(ordinary, ".git"); got != want {
		t.Errorf("an ordinary repository resolved to %q, want %q", got, want)
	}

	for name, content := range map[string]string{
		"nothing-there": "",
		"not-a-gitdir":  "something else entirely\n",
		"empty-gitdir":  "gitdir:\n",
	} {
		root := filepath.Join(dir, name)
		if err := os.MkdirAll(root, 0700); err != nil {
			t.Fatal(err)
		}
		if content != "" {
			if err := ioutil.WriteFile(filepath.Join(root, ".git"), []byte(content), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if got, want := GitDir(root), filepath.Join(root, ".git"); got != want {
			t.Errorf("%s resolved to %q, want the fallback %q", name, got, want)
		}
	}
}

// The file form above is what Git writes. This checks that against Git
// itself rather than against a belief about its format.
func TestGitItselfWritesTheFormThisReads(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the worktree is created through git, which is set up differently on the Windows runners")
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not on PATH")
	}
	dir := t.TempDir()
	main := filepath.Join(dir, "main")
	if err := os.MkdirAll(main, 0700); err != nil {
		t.Fatal(err)
	}
	git := func(wd string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = wd
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL="+filepath.Join(dir, "gitconfig"),
			"GIT_CONFIG_NOSYSTEM=1", "GIT_AUTHOR_NAME=T", "GIT_AUTHOR_EMAIL=t@e.invalid",
			"GIT_COMMITTER_NAME=T", "GIT_COMMITTER_EMAIL=t@e.invalid")
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	git(main, "init", "--quiet", ".")
	git(main, "commit", "--quiet", "--allow-empty", "--message", "init")
	linked := filepath.Join(dir, "linked")
	git(main, "worktree", "add", "--quiet", linked)

	info, err := os.Stat(filepath.Join(linked, ".git"))
	if err != nil {
		t.Fatal(err)
	}
	if info.IsDir() {
		t.Skip("this version of git made .git a directory in a worktree, so there is nothing to resolve")
	}
	resolved := GitDir(linked)
	if resolved == filepath.Join(linked, ".git") {
		t.Fatalf("the gitdir file was not followed, GitDir returned %q", resolved)
	}
	if fi, serr := os.Stat(resolved); serr != nil || !fi.IsDir() {
		t.Fatalf("GitDir returned %q, which is not a directory: %v", resolved, serr)
	}

	if _, err := Run(context.Background(), config.Default(), Options{RepoRoot: linked}); err != nil {
		t.Fatalf("a run in a real worktree failed before it started: %v", err)
	}
}
