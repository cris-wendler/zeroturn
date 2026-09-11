// Package testutil builds temporary repositories and local bare remotes
// for tests. No test in this module contacts a real remote.
package testutil

import (
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Isolate points Git and ZeroTurn at throwaway configuration so a test
// cannot read or change the developer's own settings, hooks, or state.
func Isolate(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	global := filepath.Join(dir, "gitconfig")
	if err := ioutil.WriteFile(global, []byte("[init]\n\tdefaultBranch = main\n"), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GIT_CONFIG_GLOBAL", global)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_AUTHOR_NAME", "Test")
	t.Setenv("GIT_AUTHOR_EMAIL", "test@example.invalid")
	t.Setenv("GIT_COMMITTER_NAME", "Test")
	t.Setenv("GIT_COMMITTER_EMAIL", "test@example.invalid")
	t.Setenv("ZEROTURN_STATE_DIR", filepath.Join(dir, "state"))
	home := filepath.Join(dir, "home")
	if err := os.Mkdir(home, 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HOME", home)
	t.Setenv("NO_COLOR", "1")
}

// Git runs git in dir and fails the test on error.
func Git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	out, err := GitErr(dir, args...)
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}

func GitErr(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

func Write(t *testing.T, dir, rel, content string) {
	t.Helper()
	p := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0755); err != nil {
		t.Fatal(err)
	}
	if err := ioutil.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// Canonical resolves symbolic links so paths compare equal to the ones
// Git reports, for example /var and /private/var on macOS.
func Canonical(t *testing.T, p string) string {
	t.Helper()
	r, err := filepath.EvalSymlinks(p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// Remote creates a bare remote and a clone of it on branch main with one
// pushed commit. It returns the clone and the bare remote paths.
func Remote(t *testing.T) (work, bare string) {
	t.Helper()
	root := Canonical(t, t.TempDir())
	bare = filepath.Join(root, "remote.git")
	work = filepath.Join(root, "work")
	Git(t, root, "init", "--bare", "--initial-branch=main", bare)
	Git(t, root, "clone", "--quiet", bare, work)
	Write(t, work, "README.md", "fixture\n")
	Git(t, work, "add", "README.md")
	Git(t, work, "commit", "--quiet", "--message", "initial")
	Git(t, work, "push", "--quiet", "--set-upstream", "origin", "main")
	return work, bare
}

// Clone makes a second working copy of bare, used to create remote
// commits the first copy has not seen.
func Clone(t *testing.T, bare string) string {
	t.Helper()
	dir := filepath.Join(Canonical(t, t.TempDir()), "other")
	Git(t, filepath.Dir(dir), "clone", "--quiet", bare, dir)
	return dir
}

// PushRemoteCommit adds a commit to bare from a separate clone.
func PushRemoteCommit(t *testing.T, bare, branch, file string) {
	t.Helper()
	other := Clone(t, bare)
	Git(t, other, "checkout", "--quiet", branch)
	Write(t, other, file, "remote change\n")
	Git(t, other, "add", file)
	Git(t, other, "commit", "--quiet", "--message", "remote "+file)
	Git(t, other, "push", "--quiet", "origin", branch)
}
