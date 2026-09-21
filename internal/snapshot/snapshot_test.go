package snapshot

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/cris-wendler/zeroturn/internal/git"
	"github.com/cris-wendler/zeroturn/internal/testutil"
)

const plan = "plan-digest"

func repoAt(t *testing.T) (git.Repo, string) {
	t.Helper()
	testutil.Isolate(t)
	work, _ := testutil.Remote(t)
	repo, err := git.Open(context.Background(), work)
	if err != nil {
		t.Fatal(err)
	}
	return repo, work
}

func take(t *testing.T, repo git.Repo) Snapshot {
	t.Helper()
	s, err := Compute(context.Background(), repo, plan)
	if err != nil {
		t.Fatal(err)
	}
	if s.Digest == "" {
		t.Fatal("the snapshot has no digest")
	}
	return s
}

// porcelain is the output the obvious cheap implementation would have
// hashed. Tests below compare it as well as the digest, so the reason the
// digest has to be taken over content is proved here rather than asserted
// in a comment.
func porcelain(t *testing.T, dir string) string {
	t.Helper()
	return testutil.Git(t, dir, "status", "--porcelain", "--untracked-files=all")
}

func TestTheSameRepositoryProducesTheSameDigest(t *testing.T) {
	repo, _ := repoAt(t)
	first := take(t, repo)
	second := take(t, repo)
	if first.Digest != second.Digest {
		t.Fatalf("two readings of one repository disagree:\n%s\n%s", first.Digest, second.Digest)
	}
	if !first.Clean {
		t.Error("a repository with nothing changed is not reported clean")
	}
}

// This is the defect the digest exists to avoid.
//
// Edit a file that is already modified and Git's porcelain output does
// not move: the path is the same path and the category is still modified.
// An implementation that hashed that output would report the second
// contents as the state the first contents were validated against, and a
// developer would be shown a passing run for code that was never run.
//
// The test asserts both halves, so it fails if the digest stops covering
// content and it also records that the porcelain output really is
// identical across the change.
func TestEditingAnAlreadyModifiedFileChangesTheDigest(t *testing.T) {
	repo, work := repoAt(t)
	testutil.Write(t, work, "a.txt", "one\n")
	testutil.Git(t, work, "add", "a.txt")
	testutil.Git(t, work, "commit", "--quiet", "--message", "add a")

	testutil.Write(t, work, "a.txt", "two\n")
	before, beforeStatus := take(t, repo), porcelain(t, work)

	testutil.Write(t, work, "a.txt", "three\n")
	after, afterStatus := take(t, repo), porcelain(t, work)

	if beforeStatus != afterStatus {
		t.Fatalf("this test no longer demonstrates anything: the porcelain output changed\n%q\n%q",
			beforeStatus, afterStatus)
	}
	if before.Digest == after.Digest {
		t.Fatal("the contents of a modified file changed and the digest did not, " +
			"so validation made against the earlier contents would be reported as current")
	}
}

func TestStagingAChangeChangesTheDigest(t *testing.T) {
	repo, work := repoAt(t)
	testutil.Write(t, work, "b.txt", "staged\n")
	untracked := take(t, repo)

	testutil.Git(t, work, "add", "b.txt")
	staged := take(t, repo)

	if untracked.Digest == staged.Digest {
		t.Fatal("staging a file did not change the digest")
	}
	if untracked.UntrackedFiles != 1 {
		t.Errorf("an untracked file was not counted: %d", untracked.UntrackedFiles)
	}
	if staged.TrackedChanges != 1 {
		t.Errorf("a staged file was not counted as a tracked change: %d", staged.TrackedChanges)
	}
}

// A file staged and then edited again in the working tree differs from
// the index as well as from the commit. Both halves have to reach the
// digest, or the state that is on disk is not the state recorded.
func TestEditingAfterStagingChangesTheDigest(t *testing.T) {
	repo, work := repoAt(t)
	testutil.Write(t, work, "c.txt", "first\n")
	testutil.Git(t, work, "add", "c.txt")
	staged := take(t, repo)

	testutil.Write(t, work, "c.txt", "second\n")
	edited := take(t, repo)

	if staged.Digest == edited.Digest {
		t.Fatal("a staged file was edited in the working tree and the digest did not change")
	}
}

func TestAnUntrackedFileChangesTheDigest(t *testing.T) {
	repo, work := repoAt(t)
	clean := take(t, repo)

	testutil.Write(t, work, "new_test.go", "package app\n")
	added := take(t, repo)
	if clean.Digest == added.Digest {
		t.Fatal("an untracked file appeared and the digest did not change")
	}
	if added.Clean {
		t.Error("a repository holding an untracked file is reported clean")
	}

	// Its contents matter as much as its presence. A new test file that
	// is rewritten is a different test file.
	testutil.Write(t, work, "new_test.go", "package app\n\nfunc TestX(t *testing.T) {}\n")
	rewritten := take(t, repo)
	if added.Digest == rewritten.Digest {
		t.Fatal("the contents of an untracked file changed and the digest did not")
	}
}

func TestDeletingAFileChangesTheDigest(t *testing.T) {
	repo, work := repoAt(t)
	before := take(t, repo)

	if err := os.Remove(filepath.Join(work, "README.md")); err != nil {
		t.Fatal(err)
	}
	removed := take(t, repo)
	if before.Digest == removed.Digest {
		t.Fatal("a tracked file was deleted and the digest did not change")
	}

	testutil.Git(t, work, "rm", "--quiet", "--cached", "README.md")
	staged := take(t, repo)
	if removed.Digest == staged.Digest {
		t.Fatal("the deletion was staged and the digest did not change")
	}
}

func TestRenamingAFileChangesTheDigest(t *testing.T) {
	repo, work := repoAt(t)
	testutil.Write(t, work, "old.txt", "same contents\n")
	testutil.Git(t, work, "add", "old.txt")
	testutil.Git(t, work, "commit", "--quiet", "--message", "add old")
	before := take(t, repo)

	// The contents are unchanged by a rename, so only the path can carry
	// the difference. This is what the length prefix on every path in the
	// digest is there to keep unambiguous.
	testutil.Git(t, work, "mv", "old.txt", "new.txt")
	after := take(t, repo)
	if before.Digest == after.Digest {
		t.Fatal("a file was renamed and the digest did not change")
	}
}

func TestCommittingChangesTheDigest(t *testing.T) {
	repo, work := repoAt(t)
	testutil.Write(t, work, "d.txt", "committed\n")
	testutil.Git(t, work, "add", "d.txt")
	before := take(t, repo)

	testutil.Git(t, work, "commit", "--quiet", "--message", "add d")
	after := take(t, repo)

	if before.Digest == after.Digest {
		t.Fatal("a commit was made and the digest did not change")
	}
	if before.Head == after.Head {
		t.Fatal("the recorded head did not move across a commit")
	}
	if !after.Clean {
		t.Error("a repository with everything committed is not reported clean")
	}
}

// The validation steps are part of the state a result is evidence about.
// Running the same code through different checks is a different claim.
func TestChangingTheValidationPlanChangesTheDigest(t *testing.T) {
	repo, _ := repoAt(t)
	one, err := Compute(context.Background(), repo, "plan-one")
	if err != nil {
		t.Fatal(err)
	}
	two, err := Compute(context.Background(), repo, "plan-two")
	if err != nil {
		t.Fatal(err)
	}
	if one.Digest == two.Digest {
		t.Fatal("the validation plan changed and the digest did not")
	}
	if got := one.Differences(two); len(got) != 1 || got[0] != "the validation steps changed" {
		t.Errorf("the difference is not named as the plan: %v", got)
	}
}

// Undoing a change puts the repository back, and the digest says so. This
// is correct rather than a limitation, and it is written down because a
// reader could otherwise expect a digest to count edits.
func TestUndoingAChangeRestoresTheDigest(t *testing.T) {
	repo, work := repoAt(t)
	before := take(t, repo)
	testutil.Write(t, work, "README.md", "changed\n")
	if take(t, repo).Digest == before.Digest {
		t.Fatal("a change did not move the digest")
	}
	testutil.Write(t, work, "README.md", "fixture\n")
	if take(t, repo).Digest != before.Digest {
		t.Fatal("the change was undone and the digest did not return to what it was")
	}
}

// An ignored file is outside what the digest covers. The documentation
// says so, and this holds the documentation to it: if ignored files ever
// start moving the digest, the stated limit is wrong.
func TestAnIgnoredFileDoesNotChangeTheDigest(t *testing.T) {
	repo, work := repoAt(t)
	testutil.Write(t, work, ".gitignore", "build/\n")
	testutil.Git(t, work, "add", ".gitignore")
	testutil.Git(t, work, "commit", "--quiet", "--message", "ignore build")
	before := take(t, repo)

	testutil.Write(t, work, "build/output.bin", "artifact\n")
	if take(t, repo).Digest != before.Digest {
		t.Fatal("an ignored file moved the digest, so the documented limit is wrong")
	}
}

func TestASymbolicLinkIsRecordedByItsTarget(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("creating a symbolic link on Windows needs a privilege tests do not have")
	}
	repo, work := repoAt(t)
	if err := os.Symlink("one", filepath.Join(work, "link")); err != nil {
		t.Fatal(err)
	}
	first := take(t, repo)

	if err := os.Remove(filepath.Join(work, "link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("two", filepath.Join(work, "link")); err != nil {
		t.Fatal(err)
	}
	if take(t, repo).Digest == first.Digest {
		t.Fatal("a symbolic link was repointed and the digest did not change")
	}
}

// The digest must not be reachable by a path name alone. Two repositories
// whose changed paths concatenate to the same text have to differ.
func TestPathsCannotBeRunTogether(t *testing.T) {
	repoA, workA := repoAt(t)
	testutil.Write(t, workA, "ab", "")
	testutil.Write(t, workA, "c", "")
	first := take(t, repoA)

	repoB, workB := repoAt(t)
	testutil.Write(t, workB, "a", "")
	testutil.Write(t, workB, "bc", "")
	second := take(t, repoB)

	if first.Digest == second.Digest {
		t.Fatal("two different sets of paths produced one digest")
	}
}

// Nothing the digest is taken over may be written down. The counts say
// how much differed; the paths and the contents do not survive.
func TestTheSnapshotHoldsNoPathsOrContents(t *testing.T) {
	repo, work := repoAt(t)
	testutil.Write(t, work, "secret_filename.txt", "AKIAIOSFODNN7EXAMPLE\n")
	testutil.Git(t, work, "add", "secret_filename.txt")
	snap := take(t, repo)

	b, err := ioutil.ReadFile(filepath.Join(work, "secret_filename.txt"))
	if err != nil {
		t.Fatal(err)
	}
	encoded := marshal(t, snap)
	for _, forbidden := range []string{"secret_filename", string(b), "AKIAIOSFODNN7EXAMPLE"} {
		if contains(encoded, forbidden) {
			t.Errorf("the snapshot carries %q", forbidden)
		}
	}
	if snap.TrackedChanges != 1 {
		t.Errorf("the change was not counted: %d", snap.TrackedChanges)
	}
}

// A repository with no commits has no HEAD. Asking for one fails the
// whole git command even though the branch name is known, so this is the
// case that would otherwise leave a snapshot with neither.
func TestARepositoryWithNoCommitsStillHasASnapshot(t *testing.T) {
	testutil.Isolate(t)
	dir := testutil.Canonical(t, t.TempDir())
	testutil.Git(t, dir, "init", "--quiet", "--initial-branch=main", ".")
	repo, err := git.Open(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}

	empty := take(t, repo)
	if empty.Head != "" {
		t.Errorf("a repository with no commits reports head %q", empty.Head)
	}
	if empty.Branch != "main" {
		t.Errorf("branch is %q, want main", empty.Branch)
	}

	testutil.Write(t, dir, "first.txt", "content\n")
	if take(t, repo).Digest == empty.Digest {
		t.Fatal("the first file in an empty repository did not change the digest")
	}
}

// The cost is one concern the digest has to answer for, because it is
// computed on demand by a person waiting for an answer. Almost all of it
// is starting git; the reading of changed files is the part that grows
// with the repository, so the benchmark makes every file dirty.
func BenchmarkCompute(b *testing.B) {
	dir, err := ioutil.TempDir("", "zeroturn-snapshot-bench-")
	if err != nil {
		b.Fatal(err)
	}
	defer os.RemoveAll(dir)
	for _, args := range [][]string{
		{"init", "--quiet", "--initial-branch=main", "."},
		{"config", "user.email", "bench@example.invalid"},
		{"config", "user.name", "Bench"},
	} {
		if out, err := testutil.GitErr(dir, args...); err != nil {
			b.Fatalf("%v: %s", err, out)
		}
	}
	const files = 200
	body := strings.Repeat("x", 4096)
	for i := 0; i < files; i++ {
		p := filepath.Join(dir, "src", strconv.Itoa(i%10))
		if err := os.MkdirAll(p, 0755); err != nil {
			b.Fatal(err)
		}
		if err := ioutil.WriteFile(filepath.Join(p, strconv.Itoa(i)+".txt"), []byte(body), 0644); err != nil {
			b.Fatal(err)
		}
	}
	if out, err := testutil.GitErr(dir, "add", "-A"); err != nil {
		b.Fatalf("%v: %s", err, out)
	}
	if out, err := testutil.GitErr(dir, "commit", "--quiet", "--message", "bench"); err != nil {
		b.Fatalf("%v: %s", err, out)
	}
	// Every file differs from the commit, which is the expensive shape.
	for i := 0; i < files; i++ {
		p := filepath.Join(dir, "src", strconv.Itoa(i%10), strconv.Itoa(i)+".txt")
		if err := ioutil.WriteFile(p, []byte(body+"changed"), 0644); err != nil {
			b.Fatal(err)
		}
	}

	repo, err := git.Open(context.Background(), dir)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Compute(context.Background(), repo, plan); err != nil {
			b.Fatal(err)
		}
	}
}

func marshal(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func contains(haystack, needle string) bool {
	return needle != "" && strings.Contains(haystack, needle)
}

func TestDescribeReadsAsASentence(t *testing.T) {
	cases := []struct {
		in   []string
		want string
	}{
		{nil, ""},
		{[]string{"one"}, "one"},
		{[]string{"one", "two"}, "one and two"},
		{[]string{"one", "two", "three"}, "one, two, and three"},
	}
	for _, c := range cases {
		if got := Describe(c.in); got != c.want {
			t.Errorf("Describe(%v) is %q, want %q", c.in, got, c.want)
		}
	}
}

// The mode is part of the state, because a validation step runs a file
// that is executable and does not run one that is not. Nothing about
// the contents changes when the bit does.
func TestMakingAFileExecutableChangesTheDigest(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("the executable bit is not a file mode here")
	}
	repo, work := repoAt(t)
	script := filepath.Join(work, "run.sh")
	testutil.Write(t, work, "run.sh", "#!/bin/sh\necho hello\n")
	testutil.Git(t, work, "add", "run.sh")
	testutil.Git(t, work, "commit", "--quiet", "--message", "add a script")

	before := take(t, repo)
	if err := os.Chmod(script, 0755); err != nil {
		t.Fatal(err)
	}
	if after := take(t, repo); after.Digest == before.Digest {
		t.Error("a file became executable and the digest did not move")
	}

	// And in the direction that says which is which. The digest moving
	// says only that something changed, not that the executable one is
	// the one recorded as executable.
	plain := filepath.Join(work, "plain.txt")
	testutil.Write(t, work, "plain.txt", "#!/bin/sh\necho hello\n")
	got, err := readPath(script)
	if err != nil {
		t.Fatal(err)
	}
	if got.kind != kindExec {
		t.Errorf("an executable file is recorded as %q", got.kind)
	}
	got, err = readPath(plain)
	if err != nil {
		t.Fatal(err)
	}
	if got.kind != kindFile {
		t.Errorf("a file that is not executable is recorded as %q", got.kind)
	}
}
