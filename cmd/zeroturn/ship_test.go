package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/output"
	"github.com/cris-wendler/zeroturn/internal/testutil"
)

func ship(t *testing.T, dir string, args ...string) result {
	t.Helper()
	return run(t, dir, "", append([]string{"ship"}, args...)...)
}

// unchanged asserts that nothing was staged, committed, or pushed.
func unchanged(t *testing.T, work, bare string) {
	t.Helper()
	if s := testutil.Git(t, work, "diff", "--cached", "--name-only"); s != "" {
		t.Fatalf("staged: %s", s)
	}
	if testutil.Git(t, work, "rev-parse", "HEAD") != testutil.Git(t, bare, "rev-parse", "work") {
		t.Fatal("a commit was made or pushed")
	}
}

func TestShipRequiresMessageAndFiles(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	if r := ship(t, work, "--files", "README.md"); r.code != output.ExitInvalidUsage {
		t.Fatalf("no message: %+v", r)
	}
	if r := ship(t, work, "--message", "x"); r.code != output.ExitInvalidUsage || !strings.Contains(r.stderr, "never stages everything") {
		t.Fatalf("no files: %+v", r)
	}
}

func TestShipRefusesProtectedBranch(t *testing.T) {
	work, bare := repoWithConfig(t, nil)
	testutil.Git(t, work, "checkout", "--quiet", "main")
	testutil.Write(t, work, "a.txt", "a")
	r := ship(t, work, "--message", "a", "--files", "a.txt", "--dry-run")
	if r.code != output.ExitUnsafeGit || !strings.Contains(r.stderr, "protected") {
		t.Fatalf("%+v", r)
	}
	testutil.Git(t, work, "checkout", "--quiet", "work")
	unchanged(t, work, bare)
}

func TestShipRefusesPathOutsideRepository(t *testing.T) {
	work, bare := repoWithConfig(t, nil)
	outside := filepath.Join(filepath.Dir(work), "outside.txt")
	testutil.Write(t, filepath.Dir(work), "outside.txt", "x")
	for _, p := range []string{"../outside.txt", outside} {
		r := ship(t, work, "--message", "x", "--files", p, "--dry-run")
		if r.code != output.ExitInvalidUsage || !strings.Contains(r.stderr, "outside the repository") {
			t.Fatalf("%s: %+v", p, r)
		}
	}
	if err := os.Symlink(outside, filepath.Join(work, "link.txt")); err == nil {
		r := ship(t, work, "--message", "x", "--files", "link.txt", "--dry-run")
		if r.code != output.ExitInvalidUsage {
			t.Fatalf("symlink escaping the repository was accepted: %+v", r)
		}
	}
	unchanged(t, work, bare)
}

func TestShipRefusesCredentialWithoutPrintingIt(t *testing.T) {
	work, bare := repoWithConfig(t, nil)
	secret := "AKIA" + "QWERTYUIOPASDFGH"
	testutil.Write(t, work, "deploy.env", "region=eu\nkey="+secret+"\n")
	r := ship(t, work, "--message", "deploy", "--files", "deploy.env")
	if r.code != output.ExitPolicyFailure {
		t.Fatalf("%+v", r)
	}
	if !strings.Contains(r.stderr, "deploy.env:2") || !strings.Contains(r.stderr, "aws access key id") {
		t.Fatalf("finding not reported: %q", r.stderr)
	}
	if strings.Contains(r.all(), secret) {
		t.Fatal("the credential value was printed")
	}
	unchanged(t, work, bare)
}

func TestShipRefusesUnrelatedStagedFiles(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	testutil.Write(t, work, "mine.txt", "m")
	testutil.Write(t, work, "other.txt", "o")
	testutil.Git(t, work, "add", "other.txt")
	r := ship(t, work, "--message", "m", "--files", "mine.txt", "--dry-run")
	if r.code != output.ExitUnsafeGit || !strings.Contains(r.stderr, "other.txt") {
		t.Fatalf("%+v", r)
	}
}

func TestShipDryRunChangesNothing(t *testing.T) {
	work, bare := repoWithConfig(t, nil)
	testutil.Write(t, work, "notes.md", "n")
	r := ship(t, work, "--message", "docs: notes", "--files", "notes.md", "--dry-run")
	if r.code != 0 || !strings.Contains(r.stdout, "READY TO SHIP") {
		t.Fatalf("%+v", r)
	}
	for _, want := range []string{"notes.md", "docs: notes", "branch:   work", "remote:   origin"} {
		if !strings.Contains(r.stdout, want) {
			t.Errorf("plan does not show %q", want)
		}
	}
	unchanged(t, work, bare)
}

func TestShipRefusesBehindAndDiverged(t *testing.T) {
	work, bare := repoWithConfig(t, nil)
	testutil.PushRemoteCommit(t, bare, "work", "remote.txt")
	testutil.Write(t, work, "a.txt", "a")
	r := ship(t, work, "--message", "a", "--files", "a.txt", "--dry-run")
	if r.code != output.ExitUnsafeGit || !strings.Contains(r.stderr, "behind") {
		t.Fatalf("behind: %+v", r)
	}
	testutil.Git(t, work, "add", "a.txt")
	testutil.Git(t, work, "commit", "--quiet", "--message", "local")
	testutil.Write(t, work, "b.txt", "b")
	r = ship(t, work, "--message", "b", "--files", "b.txt", "--dry-run")
	if r.code != output.ExitUnsafeGit || !strings.Contains(r.stderr, "ahead and") {
		t.Fatalf("diverged: %+v", r)
	}
}

func TestShipWithoutTerminalRefusesToConfirm(t *testing.T) {
	work, bare := repoWithConfig(t, nil)
	testutil.Write(t, work, "a.txt", "a")
	r := ship(t, work, "--message", "a", "--files", "a.txt")
	if r.code != output.ExitDeclined {
		t.Fatalf("%+v", r)
	}
	unchanged(t, work, bare)
}

// A path that does not exist and is not tracked is a mistake, not a
// deletion, and must be refused before anything is shown as ready.
func TestShipRefusesUnknownPath(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	r := ship(t, work, "--message", "x", "--files", "no-such-file.txt", "--dry-run")
	if r.code != output.ExitInvalidUsage || strings.Contains(r.stdout, "READY TO SHIP") {
		t.Fatalf("%+v", r)
	}
}

func TestShipShowsIntentionalDeletion(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	os.Remove(filepath.Join(work, "README.md"))
	r := ship(t, work, "--message", "remove readme", "--files", "README.md", "--dry-run")
	if r.code != 0 || !strings.Contains(r.stdout, "README.md  (deletion)") {
		t.Fatalf("%+v", r)
	}
}

// Relative paths are read from the directory the command runs in, the way
// git itself reads them.
func TestShipResolvesPathsFromWorkingDirectory(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	testutil.Write(t, work, "docs/guide.md", "g")
	r := ship(t, filepath.Join(work, "docs"), "--message", "g", "--files", "guide.md", "--dry-run")
	if r.code != 0 || !strings.Contains(r.stdout, "docs/guide.md") {
		t.Fatalf("%+v", r)
	}
}

func TestShipRequiresApprovedValidation(t *testing.T) {
	work, bare := repoWithConfig(t, func(c *config.Config) {
		c.Verify.Steps = []config.Step{{Name: "test", Command: []string{"go", "version"}}}
	})
	testutil.Write(t, work, "a.txt", "a")
	r := ship(t, work, "--message", "a", "--files", "a.txt", "--dry-run")
	if r.code != output.ExitDeclined || !strings.Contains(r.stderr, "not approved") {
		t.Fatalf("%+v", r)
	}
	unchanged(t, work, bare)
}

// The confirmed path stages exactly the listed files, commits with the
// supplied message, and pushes without force.
func TestShipConfirmedStagesOnlyListedFiles(t *testing.T) {
	work, bare := repoWithConfig(t, nil)
	testutil.Write(t, work, "a.txt", "a")
	testutil.Write(t, work, "b.txt", "b")
	testutil.Write(t, work, "unlisted.txt", "u")

	orig := confirm
	confirm = func(string) (bool, error) { return true, nil }
	defer func() { confirm = orig }()
	wd, _ := os.Getwd()
	os.Chdir(work)
	defer os.Chdir(wd)
	stdout := os.Stdout
	devnull, _ := os.Open(os.DevNull)
	os.Stdout = devnull
	err := cmdShip(context.Background(), []string{"--message", "feat: a and b", "--files", "a.txt", "b.txt"})
	os.Stdout = stdout
	if err != nil {
		t.Fatal(err)
	}

	files := testutil.Git(t, work, "show", "--name-only", "--format=", "HEAD")
	if files != "a.txt\nb.txt" {
		t.Fatalf("committed %q", files)
	}
	if msg := testutil.Git(t, work, "log", "-1", "--format=%B"); msg != "feat: a and b" {
		t.Fatalf("message %q", msg)
	}
	if testutil.Git(t, bare, "rev-parse", "work") != testutil.Git(t, work, "rev-parse", "HEAD") {
		t.Fatal("not pushed")
	}
	if s := testutil.Git(t, work, "status", "--porcelain"); s != "?? unlisted.txt" {
		t.Fatalf("working tree %q", s)
	}
}

func TestShipDeclinedChangesNothing(t *testing.T) {
	work, bare := repoWithConfig(t, nil)
	testutil.Write(t, work, "a.txt", "a")
	orig := confirm
	confirm = func(string) (bool, error) { return false, nil }
	defer func() { confirm = orig }()
	wd, _ := os.Getwd()
	os.Chdir(work)
	defer os.Chdir(wd)
	stdout := os.Stdout
	devnull, _ := os.Open(os.DevNull)
	os.Stdout = devnull
	err := cmdShip(context.Background(), []string{"--message", "a", "--files", "a.txt"})
	os.Stdout = stdout
	if ze, ok := err.(*output.Error); !ok || ze.Code != output.ExitDeclined {
		t.Fatalf("err %v", err)
	}
	unchanged(t, work, bare)
}

func TestShipStagesSymlinkItselfNotItsTarget(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	testutil.Write(t, work, "docs/v2.md", "v2")
	testutil.Git(t, work, "add", "docs/v2.md")
	testutil.Git(t, work, "commit", "--quiet", "--message", "v2")
	testutil.Git(t, work, "push", "--quiet")
	if err := os.Symlink("v2.md", filepath.Join(work, "docs", "latest.md")); err != nil {
		t.Skip("symbolic links unavailable")
	}
	r := ship(t, work, "--message", "link", "--files", "docs/latest.md", "--dry-run")
	if r.code != 0 || !strings.Contains(r.stdout, "docs/latest.md") || strings.Contains(r.stdout, "  docs/v2.md") {
		t.Fatalf("%+v", r)
	}
}

func TestShipHandlesNonASCIINames(t *testing.T) {
	work, _ := repoWithConfig(t, nil)
	name := "café notes.md"
	testutil.Write(t, work, name, "n")
	testutil.Git(t, work, "add", "--", name)
	r := ship(t, work, "--message", "notes", "--files", name, "--dry-run")
	if r.code != 0 || !strings.Contains(r.stdout, "READY TO SHIP") {
		t.Fatalf("a staged file named in the command was reported as unrelated: %+v", r)
	}
}
