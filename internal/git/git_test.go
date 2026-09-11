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
	}
	for in, want := range cases {
		if got := StripCredentials(in); got != want {
			t.Errorf("StripCredentials(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRemoteDisplayHidesCredentials(t *testing.T) {
	testutil.Isolate(t)
	work, _ := testutil.Remote(t)
	testutil.Git(t, work, "remote", "add", "creds", "https://someone:hunter2hunter2@example.invalid/r.git")
	r := open(t, work)
	if got := r.RemoteDisplay(ctx, "creds"); strings.Contains(got, "hunter2") {
		t.Fatalf("display leaked %q", got)
	}
}
