// Package git wraps the Git commands ZeroTurn is allowed to run.
//
// Every call passes an explicit argument array to the git executable. No
// shell is involved. The mutating set is deliberately small: fetch, a fast
// forward merge, staging named paths, commit, and a plain push. Reset,
// stash, rebase, force push, branch deletion, and --no-verify have no
// implementation here, so they cannot be reached by any code path.
package git

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

type Repo struct{ Root string }

var ErrNotRepo = errors.New("not inside a Git repository")

func run(ctx context.Context, dir string, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	// A pager would block a non interactive run.
	cmd.Env = append(envWithout("GIT_PAGER"), "GIT_PAGER=cat", "GIT_TERMINAL_PROMPT=0")
	err := cmd.Run()
	return strings.TrimRight(out.String(), "\n"), strings.TrimSpace(errb.String()), err
}

// gitError reports a failed git call. Git's own message is used when it
// printed one, because it is more specific than anything written here,
// and the fallback covers a failure that printed nothing at all.
func gitError(errText, fallback string) error {
	if errText == "" {
		return errors.New(fallback)
	}
	return errors.New(errText)
}

func (r Repo) git(ctx context.Context, args ...string) (string, string, error) {
	return run(ctx, r.Root, args...)
}

// Open finds the repository that contains dir.
func Open(ctx context.Context, dir string) (Repo, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return Repo{}, errors.New("the git executable was not found on PATH")
	}
	out, _, err := run(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil || out == "" {
		return Repo{}, ErrNotRepo
	}
	abs, err := filepath.Abs(out)
	if err != nil {
		return Repo{}, err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		abs = resolved
	}
	return Repo{Root: abs}, nil
}

// FindRoot locates the repository containing dir by looking for a .git
// entry, without starting a git process. The status line repaints often,
// and the specification forbids a Git command on every repaint. The result
// matches Open for ordinary repositories, worktrees, and submodules.
func FindRoot(dir string) (string, bool) {
	if dir == "" {
		return "", false
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", false
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		abs = resolved
	}
	for {
		if _, err := os.Lstat(filepath.Join(abs, ".git")); err == nil {
			return abs, true
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			return "", false
		}
		abs = parent
	}
}

func (r Repo) Branch(ctx context.Context) (string, error) {
	out, _, err := r.git(ctx, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	return out, nil
}

// Upstream returns the configured upstream, or an empty string when the
// branch does not track one.
func (r Repo) Upstream(ctx context.Context) string {
	out, _, err := r.git(ctx, "rev-parse", "--abbrev-ref", "--symbolic-full-name", "@{u}")
	if err != nil {
		return ""
	}
	return out
}

func (r Repo) Remotes(ctx context.Context) []string {
	out, _, err := r.git(ctx, "remote")
	if err != nil || out == "" {
		return nil
	}
	return strings.Split(out, "\n")
}

func (r Repo) HasRemote(ctx context.Context, name string) bool {
	for _, x := range r.Remotes(ctx) {
		if x == name {
			return true
		}
	}
	return false
}

type Change struct {
	Path    string
	Code    string
	Staged  bool
	Deleted bool
}

// Status reports the working tree using the stable porcelain v1 format.
// The -z form is used because the line form quotes and escapes unusual
// file names, which would then fail to match the names a user typed.
func (r Repo) Status(ctx context.Context) ([]Change, error) {
	out, errText, err := r.git(ctx, "status", "--porcelain", "-z", "--untracked-files=all")
	if err != nil {
		return nil, fmt.Errorf("git status failed: %s", errText)
	}
	var ch []Change
	fields := strings.Split(out, "\x00")
	for i := 0; i < len(fields); i++ {
		entry := fields[i]
		if len(entry) < 4 {
			continue
		}
		code := entry[:2]
		// A rename or copy is followed by a second field holding the
		// original name, which is not a change of its own.
		if code[0] == 'R' || code[0] == 'C' {
			i++
		}
		ch = append(ch, Change{
			Path:    entry[3:],
			Code:    code,
			Staged:  code[0] != ' ' && code[0] != '?',
			Deleted: code[0] == 'D' || code[1] == 'D',
		})
	}
	return ch, nil
}

// Head returns the commit HEAD points at and the branch it is on, in one
// call because starting a git process costs far more than the work it
// does. A repository with no commits yet has no HEAD, which is not a
// failure: the commit is answered as an empty string so that a snapshot
// of an empty repository is still a snapshot.
func (r Repo) Head(ctx context.Context) (sha, branch string) {
	out, _, err := r.git(ctx, "rev-parse", "HEAD", "--abbrev-ref", "HEAD")
	if err != nil {
		// Before the first commit there is no HEAD to resolve, and
		// rev-parse fails whichever way it is asked, because it resolves
		// revisions and there is none. The branch is still known: it is
		// the reference HEAD points at, which symbolic-ref reads without
		// needing anything to be committed to it.
		name, _, berr := r.git(ctx, "symbolic-ref", "--short", "HEAD")
		if berr != nil {
			return "", ""
		}
		return "", name
	}
	lines := strings.Split(out, "\n")
	if len(lines) < 2 {
		return strings.TrimSpace(out), ""
	}
	return strings.TrimSpace(lines[0]), strings.TrimSpace(lines[1])
}

// IndexEntry is one line of the index: what Git has staged for a path.
//
// Blob is the hash of the content Git holds for it, so two entries with
// the same blob hold the same bytes and a change to a staged file changes
// the entry. Mode carries the executable bit and marks a submodule.
type IndexEntry struct {
	Mode  string
	Blob  string
	Stage string
	Path  string
}

// IsSubmodule reports whether the entry is a gitlink rather than a file.
// Git records the submodule's commit here and nothing about its contents.
const submoduleMode = "160000"

func (e IndexEntry) IsSubmodule() bool { return e.Mode == submoduleMode }

// IndexEntries lists what Git has staged, with the content hash of each
// path. The -z form is used for the same reason Status uses it: the line
// form quotes unusual file names, and a quoted name is not the name.
func (r Repo) IndexEntries(ctx context.Context) ([]IndexEntry, error) {
	out, errText, err := r.git(ctx, "ls-files", "--stage", "-z")
	if err != nil {
		return nil, gitError(errText, "git ls-files returned a failure")
	}
	var entries []IndexEntry
	for _, record := range strings.Split(out, "\x00") {
		if record == "" {
			continue
		}
		// <mode> <object> <stage>\t<path>
		tab := strings.IndexByte(record, '\t')
		if tab < 0 {
			continue
		}
		fields := strings.Fields(record[:tab])
		if len(fields) != 3 {
			continue
		}
		entries = append(entries, IndexEntry{
			Mode: fields[0], Blob: fields[1], Stage: fields[2], Path: record[tab+1:],
		})
	}
	return entries, nil
}

func (r Repo) IsClean(ctx context.Context) (bool, error) {
	ch, err := r.Status(ctx)
	if err != nil {
		return false, err
	}
	return len(ch) == 0, nil
}

func (r Repo) StagedPaths(ctx context.Context) ([]string, error) {
	out, errText, err := r.git(ctx, "diff", "--cached", "--name-only", "-z")
	if err != nil {
		return nil, errors.New(errText)
	}
	var paths []string
	for _, p := range strings.Split(out, "\x00") {
		if p != "" {
			paths = append(paths, p)
		}
	}
	return paths, nil
}

// IsIgnored reports whether path would be ignored by Git in this repository.
func (r Repo) IsIgnored(ctx context.Context, path string) bool {
	_, _, err := r.git(ctx, "check-ignore", "--quiet", "--", path)
	return err == nil
}

// IsTracked reports whether rel, relative to the repository root, is in
// the index. It lets ship tell an intentional deletion from a mistyped path.
func (r Repo) IsTracked(ctx context.Context, rel string) bool {
	_, _, err := r.git(ctx, "ls-files", "--error-unmatch", "--", rel)
	return err == nil
}

func (r Repo) HasStagedChanges(ctx context.Context) (bool, error) {
	p, err := r.StagedPaths(ctx)
	return len(p) > 0, err
}

func (r Repo) Fetch(ctx context.Context, remote string) error {
	_, errText, err := r.git(ctx, "fetch", "--no-tags", remote)
	if err != nil {
		return gitError(errText, "git fetch returned a failure")
	}
	return nil
}

type Divergence struct {
	Ahead     int
	Behind    int
	Upstream  string
	HasUpsrm  bool
	Diverged  bool
	UpToDate  bool
	OnlyAhead bool
}

func (r Repo) Divergence(ctx context.Context) (Divergence, error) {
	up := r.Upstream(ctx)
	if up == "" {
		return Divergence{HasUpsrm: false}, nil
	}
	out, errText, err := r.git(ctx, "rev-list", "--left-right", "--count", up+"...HEAD")
	if err != nil {
		return Divergence{}, errors.New(errText)
	}
	fields := strings.Fields(out)
	if len(fields) != 2 {
		return Divergence{}, errors.New("git rev-list produced output ZeroTurn could not read")
	}
	behind, _ := strconv.Atoi(fields[0])
	ahead, _ := strconv.Atoi(fields[1])
	d := Divergence{Ahead: ahead, Behind: behind, Upstream: up, HasUpsrm: true}
	d.Diverged = ahead > 0 && behind > 0
	d.UpToDate = ahead == 0 && behind == 0
	d.OnlyAhead = ahead > 0 && behind == 0
	return d, nil
}

// FastForward advances the branch only when the merge cannot create a
// commit. A divergent history is refused by git itself.
func (r Repo) FastForward(ctx context.Context, upstream string) error {
	_, errText, err := r.git(ctx, "merge", "--ff-only", upstream)
	if err != nil {
		return gitError(errText, "the fast forward was refused")
	}
	return nil
}

// Add stages only the paths given. The pathspec separator stops a file
// name from being read as an option.
func (r Repo) Add(ctx context.Context, paths []string) error {
	args := append([]string{"add", "--"}, paths...)
	_, errText, err := r.git(ctx, args...)
	if err != nil {
		return errors.New(errText)
	}
	return nil
}

func (r Repo) Commit(ctx context.Context, message string) (string, error) {
	_, errText, err := r.git(ctx, "commit", "--message", message)
	if err != nil {
		return "", gitError(errText, "git commit returned a failure")
	}
	sha, _, err := r.git(ctx, "rev-parse", "HEAD")
	return sha, err
}

// Push never passes --force, --force-with-lease, or --no-verify.
func (r Repo) Push(ctx context.Context, remote, branch string) (string, error) {
	out, errText, err := r.git(ctx, "push", remote, branch)
	if err != nil {
		return "", gitError(errText, "git push returned a failure")
	}
	if out == "" {
		out = errText
	}
	return out, nil
}

func (r Repo) ShortSHA(ctx context.Context, sha string) string {
	out, _, err := r.git(ctx, "rev-parse", "--short", sha)
	if err != nil {
		if len(sha) > 8 {
			return sha[:8]
		}
		return sha
	}
	return out
}

// RemoteDisplay returns a remote URL with any embedded credentials
// removed, so a report can name the destination without leaking one.
func (r Repo) RemoteDisplay(ctx context.Context, remote string) string {
	out, _, err := r.git(ctx, "remote", "get-url", remote)
	if err != nil || out == "" {
		return remote
	}
	return StripCredentials(out)
}

func StripCredentials(url string) string {
	i := strings.Index(url, "://")
	if i < 0 {
		return url
	}
	rest := url[i+3:]
	at := strings.Index(rest, "@")
	if at < 0 {
		return url
	}
	slash := strings.Index(rest, "/")
	if slash >= 0 && slash < at {
		return url
	}
	return url[:i+3] + rest[at+1:]
}

func envWithout(key string) []string {
	var out []string
	for _, kv := range environ() {
		if strings.HasPrefix(kv, key+"=") {
			continue
		}
		out = append(out, kv)
	}
	return out
}
