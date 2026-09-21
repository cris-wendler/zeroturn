package git

import (
	"context"
	"sort"
	"strings"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/testutil"
)

var ctx = context.Background()

func open(t *testing.T, dir string) Repo {
	t.Helper()
	r, err := Open(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestOpenOutsideRepository(t *testing.T) {
	testutil.Isolate(t)
	if _, err := Open(ctx, t.TempDir()); err != ErrNotRepo {
		t.Fatalf("got %v, want ErrNotRepo", err)
	}
}

func TestOpenFromSubdirectoryFindsRoot(t *testing.T) {
	testutil.Isolate(t)
	work, _ := testutil.Remote(t)
	testutil.Write(t, work, "a/b/c.txt", "x")
	r := open(t, work+"/a/b")
	if r.Root != work {
		t.Fatalf("root %q, want %q", r.Root, work)
	}
}

func TestBranchUpstreamRemotes(t *testing.T) {
	testutil.Isolate(t)
	work, _ := testutil.Remote(t)
	r := open(t, work)
	if b, _ := r.Branch(ctx); b != "main" {
		t.Fatalf("branch %q", b)
	}
	if u := r.Upstream(ctx); u != "origin/main" {
		t.Fatalf("upstream %q", u)
	}
	if !r.HasRemote(ctx, "origin") || r.HasRemote(ctx, "upstream") {
		t.Fatal("remote detection")
	}
	testutil.Git(t, work, "checkout", "--quiet", "-b", "local-only")
	if u := r.Upstream(ctx); u != "" {
		t.Fatalf("branch without upstream reported %q", u)
	}
}

func TestStatusClassifiesChanges(t *testing.T) {
	testutil.Isolate(t)
	work, _ := testutil.Remote(t)
	r := open(t, work)
	if clean, _ := r.IsClean(ctx); !clean {
		t.Fatal("fresh clone is not clean")
	}
	testutil.Write(t, work, "new.txt", "n")
	testutil.Write(t, work, "staged.txt", "s")
	testutil.Git(t, work, "add", "staged.txt")
	testutil.Git(t, work, "rm", "--quiet", "README.md")

	ch, err := r.Status(ctx)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]Change{}
	for _, c := range ch {
		got[c.Path] = c
	}
	if c := got["new.txt"]; c.Staged || c.Deleted {
		t.Errorf("untracked: %+v", c)
	}
	if c := got["staged.txt"]; !c.Staged {
		t.Errorf("staged: %+v", c)
	}
	if c := got["README.md"]; !c.Deleted || !c.Staged {
		t.Errorf("deleted: %+v", c)
	}
	staged, _ := r.StagedPaths(ctx)
	sort.Strings(staged)
	if strings.Join(staged, ",") != "README.md,staged.txt" {
		t.Errorf("staged paths %v", staged)
	}
}

func TestDivergenceStates(t *testing.T) {
	testutil.Isolate(t)
	work, bare := testutil.Remote(t)
	r := open(t, work)

	d, _ := r.Divergence(ctx)
	if !d.UpToDate || d.Diverged {
		t.Fatalf("fresh clone: %+v", d)
	}

	testutil.Write(t, work, "local.txt", "l")
	testutil.Git(t, work, "add", "local.txt")
	testutil.Git(t, work, "commit", "--quiet", "--message", "local")
	d, _ = r.Divergence(ctx)
	if !d.OnlyAhead || d.Ahead != 1 {
		t.Fatalf("ahead: %+v", d)
	}

	testutil.PushRemoteCommit(t, bare, "main", "remote.txt")
	if err := r.Fetch(ctx, "origin"); err != nil {
		t.Fatal(err)
	}
	d, _ = r.Divergence(ctx)
	if !d.Diverged || d.Ahead != 1 || d.Behind != 1 {
		t.Fatalf("diverged: %+v", d)
	}
	// The three flags answer different questions and ship refuses on two
	// of them, so each one is asserted in every state rather than only in
	// the state it is named for.
	if d.UpToDate || d.OnlyAhead {
		t.Fatalf("diverged and also up to date or only ahead: %+v", d)
	}

	// Behind and not ahead is the fourth state, and the one that has to
	// leave all three flags clear.
	work2 := testutil.Clone(t, bare)
	r2 := open(t, work2)
	testutil.PushRemoteCommit(t, bare, "main", "second.txt")
	if err := r2.Fetch(ctx, "origin"); err != nil {
		t.Fatal(err)
	}
	d, _ = r2.Divergence(ctx)
	if d.Behind != 1 || d.Ahead != 0 {
		t.Fatalf("behind: %+v", d)
	}
	if d.UpToDate || d.OnlyAhead || d.Diverged {
		t.Fatalf("behind by one and yet %+v", d)
	}
}

func TestFastForwardRefusesDivergentHistory(t *testing.T) {
	testutil.Isolate(t)
	work, bare := testutil.Remote(t)
	r := open(t, work)

	testutil.PushRemoteCommit(t, bare, "main", "remote.txt")
	r.Fetch(ctx, "origin")
	if err := r.FastForward(ctx, "origin/main"); err != nil {
		t.Fatalf("behind only must fast forward: %v", err)
	}

	testutil.Write(t, work, "local.txt", "l")
	testutil.Git(t, work, "add", "local.txt")
	testutil.Git(t, work, "commit", "--quiet", "--message", "local")
	testutil.PushRemoteCommit(t, bare, "main", "remote2.txt")
	r.Fetch(ctx, "origin")
	head := testutil.Git(t, work, "rev-parse", "HEAD")
	if err := r.FastForward(ctx, "origin/main"); err == nil {
		t.Fatal("divergent fast forward reported success")
	}
	if testutil.Git(t, work, "rev-parse", "HEAD") != head {
		t.Fatal("a refused fast forward moved HEAD")
	}
}

// A file whose name looks like a flag must be staged as a file.
func TestAddTreatsNamesAsPaths(t *testing.T) {
	testutil.Isolate(t)
	work, _ := testutil.Remote(t)
	r := open(t, work)
	testutil.Write(t, work, "--all", "x")
	testutil.Write(t, work, "other.txt", "y")
	if err := r.Add(ctx, []string{"--all"}); err != nil {
		t.Fatal(err)
	}
	staged, _ := r.StagedPaths(ctx)
	if len(staged) != 1 || staged[0] != "--all" {
		t.Fatalf("staged %v", staged)
	}
}

func TestCommitAndPush(t *testing.T) {
	testutil.Isolate(t)
	work, bare := testutil.Remote(t)
	r := open(t, work)
	testutil.Write(t, work, "a.txt", "a")
	r.Add(ctx, []string{"a.txt"})
	sha, err := r.Commit(ctx, "add a")
	if err != nil || len(sha) != 40 {
		t.Fatalf("sha %q err %v", sha, err)
	}
	if _, err := r.Push(ctx, "origin", "main"); err != nil {
		t.Fatal(err)
	}
	if testutil.Git(t, bare, "rev-parse", "main") != sha {
		t.Fatal("remote did not receive the commit")
	}
	if msg := testutil.Git(t, work, "log", "-1", "--format=%B"); msg != "add a" {
		t.Fatalf("message rewritten to %q", msg)
	}
}

func TestPushNeverForces(t *testing.T) {
	testutil.Isolate(t)
	work, bare := testutil.Remote(t)
	r := open(t, work)
	testutil.PushRemoteCommit(t, bare, "main", "remote.txt")
	remoteHead := testutil.Git(t, bare, "rev-parse", "main")

	testutil.Write(t, work, "local.txt", "l")
	r.Add(ctx, []string{"local.txt"})
	r.Commit(ctx, "local")
	if _, err := r.Push(ctx, "origin", "main"); err == nil {
		t.Fatal("non fast forward push reported success")
	}
	if testutil.Git(t, bare, "rev-parse", "main") != remoteHead {
		t.Fatal("remote history was overwritten")
	}
}

func TestStripCredentials(t *testing.T) {
	cases := map[string]string{
		"https://user:pass@github.com/o/r.git":  "https://github.com/o/r.git",
		"https://token@github.com/o/r.git":      "https://github.com/o/r.git",
		"https://github.com/o/r.git":            "https://github.com/o/r.git",
		"git@github.com:o/r.git":                "git@github.com:o/r.git",
		"https://github.com/o/r@v1.git":         "https://github.com/o/r@v1.git",
		"ssh://git@example.com:22/srv/repo.git": "ssh://example.com:22/srv/repo.git",
		"/srv/local/remote.git":                 "/srv/local/remote.git",
		// The degenerate shapes. Each one sits on a boundary the
		// function compares against, and none of the ordinary cases
		// above reaches any of them.
		"://user@example.com/r.git":   "://example.com/r.git",
		"https://@example.com/r.git":  "https://example.com/r.git",
		"https:///srv/repo@v1.git":    "https:///srv/repo@v1.git",
		"https://example.com/@v1.git": "https://example.com/@v1.git",
	}
	for in, want := range cases {
		if got := StripCredentials(in); got != want {
			t.Errorf("StripCredentials(%q) = %q, want %q", in, got, want)
		}
	}
}

// Checking only that the credential is absent passed for a function that
// failed and returned the bare remote name, which carries no credential
// and no URL either. What it has to return is the destination, with only
// the credential gone.
func TestRemoteDisplayHidesCredentials(t *testing.T) {
	testutil.Isolate(t)
	work, _ := testutil.Remote(t)
	testutil.Git(t, work, "remote", "add", "creds", "https://someone:hunter2hunter2@example.invalid/r.git")
	r := open(t, work)

	got := r.RemoteDisplay(ctx, "creds")
	if want := "https://example.invalid/r.git"; got != want {
		t.Fatalf("RemoteDisplay is %q, want %q", got, want)
	}
}

// Git's own message is the specific one, so it is used whenever git
// printed anything. The fallback exists for a failure that printed
// nothing, which the wrappers around it cannot arrange on demand.
func TestAFailedGitCallKeepsGitsOwnMessage(t *testing.T) {
	if err := gitError("fatal: no such remote", "git fetch returned a failure"); err == nil ||
		err.Error() != "fatal: no such remote" {
		t.Errorf("got %v, want git's own message", err)
	}
	if err := gitError("", "git fetch returned a failure"); err == nil ||
		err.Error() != "git fetch returned a failure" {
		t.Errorf("got %v, want the fallback", err)
	}
}

// Each of these answers a question ship asks before it changes anything,
// and each was only ever asserted in one of its two answers.
func TestIgnoredTrackedAndStaged(t *testing.T) {
	testutil.Isolate(t)
	work, _ := testutil.Remote(t)
	testutil.Write(t, work, ".gitignore", "build/\n")
	testutil.Write(t, work, "build/out.bin", "x")
	testutil.Write(t, work, "kept.txt", "x")
	testutil.Git(t, work, "add", ".gitignore", "kept.txt")
	testutil.Git(t, work, "commit", "--quiet", "--message", "add")
	r := open(t, work)

	if !r.IsIgnored(ctx, "build/out.bin") {
		t.Error("an ignored path is reported as not ignored")
	}
	if r.IsIgnored(ctx, "kept.txt") {
		t.Error("a tracked path is reported as ignored")
	}
	if !r.IsTracked(ctx, "kept.txt") {
		t.Error("a committed path is reported as untracked")
	}
	if r.IsTracked(ctx, "never.txt") {
		t.Error("a path that was never added is reported as tracked")
	}

	staged, err := r.HasStagedChanges(ctx)
	if err != nil || staged {
		t.Fatalf("staged %v after a commit, err %v", staged, err)
	}
	testutil.Write(t, work, "kept.txt", "changed")
	testutil.Git(t, work, "add", "kept.txt")
	staged, err = r.HasStagedChanges(ctx)
	if err != nil || !staged {
		t.Fatalf("staged %v with one path staged, err %v", staged, err)
	}
}

// ShortSHA answers with git's own abbreviation, which is what a person
// sees everywhere else. Truncating the input is the fallback for a
// revision git cannot resolve, and returning it for one git can resolve
// would print a name no other tool uses.
func TestShortSHAUsesGitsAbbreviation(t *testing.T) {
	testutil.Isolate(t)
	work, _ := testutil.Remote(t)
	r := open(t, work)
	full, _, err := r.git(ctx, "rev-parse", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	want, _, err := r.git(ctx, "rev-parse", "--short", full)
	if err != nil {
		t.Fatal(err)
	}
	if got := r.ShortSHA(ctx, full); got != want {
		t.Errorf("ShortSHA is %q, want git's %q", got, want)
	}
	// A revision git cannot resolve, longer than the fallback bound.
	const unknown = "0123456789abcdef"
	if got := r.ShortSHA(ctx, unknown); got != unknown[:8] {
		t.Errorf("an unresolvable revision gave %q", got)
	}
	// And one no longer than the bound comes back whole.
	if got := r.ShortSHA(ctx, "0123456"); got != "0123456" {
		t.Errorf("a short unresolvable revision gave %q", got)
	}
}
