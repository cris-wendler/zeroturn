// Package snapshot identifies the exact state of a repository that a
// validation run was made against.
//
// The point of it is to answer one question later: is the code here now
// the code that was tested? A name for the state is not enough, because
// two different states must never share one. So the digest is taken over
// content rather than over a description of content.
//
// The obvious cheap answer, hashing the output of git status, is wrong,
// and wrong in the direction that matters. That output names paths and
// the category each one is in. Edit an already modified file again and
// every path and every category stays exactly as it was, so the digest
// would not move and validation made against the earlier contents would
// still be presented as current. A digest that fails to change is worse
// than no digest, because it is believed.
//
// What is hashed instead: the commit, the content Git holds for every
// staged path, the content of every path the working tree disagrees with
// Git about, and the definition of the validation steps themselves.
//
// What it cannot see is written down in Limits, and in the documentation,
// because a reader has to be able to tell what the digest is promising.
package snapshot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cris-wendler/zeroturn/internal/git"
)

// Version is the digest format. A change to what is hashed, or to how it
// is written, changes this, so that evidence recorded by an older build is
// treated as describing a different state rather than silently compared
// against a digest that was computed a different way.
const Version = 1

// Snapshot is one reading of a repository, with the digest that identifies
// it and the small amount of description needed to explain it to a person.
//
// Nothing here holds file contents, path names from the repository, or
// anything a person wrote. The counts say how much was different; the
// digest says whether it is the same.
type Snapshot struct {
	Version int    `json:"snapshotVersion"`
	Digest  string `json:"digest"`
	// Head is the commit the snapshot was taken at, empty in a repository
	// with no commits. It is stored so a later comparison can say that the
	// commit changed rather than only that something did.
	Head string `json:"head,omitempty"`
	// PlanDigest identifies the validation steps. It is inside Digest as
	// well, so changing a step makes earlier evidence stale, and it is kept
	// beside it so the reason can be named.
	PlanDigest string `json:"planDigest"`
	Branch     string `json:"branch,omitempty"`
	Clean      bool   `json:"clean"`
	// TrackedChanges counts paths Git tracks whose state differs from the
	// commit; UntrackedFiles counts files Git does not track and does not
	// ignore. DirtySubmodules counts submodules Git reports as changed.
	TrackedChanges  int `json:"trackedChanges"`
	UntrackedFiles  int `json:"untrackedFiles"`
	DirtySubmodules int `json:"dirtySubmodules,omitempty"`
}

// Limits is what the digest does not cover.
//
// It is the one description of them. The README states the same list for
// a reader, and a test requires the README to carry each sentence in
// this one word for word, because a promise about what a check does not
// see is exactly the kind that is quietly outgrown by the code it
// describes.
var Limits = []string{
	"Files Git ignores. A change to an ignored build input does not move the digest.",
	"Contents inside a submodule. Git reports that a submodule changed, and that is recorded, but the files within it are not hashed.",
	"Anything outside the repository: installed dependencies, environment variables, toolchain versions, services the tests reach.",
	"A change that was made and then undone. The digest returns to its earlier value, because the code did too.",
}

// Compute reads the repository and returns its snapshot. planDigest
// identifies the validation steps and comes from the configuration, so
// that editing a step is a change of state like any other.
func Compute(ctx context.Context, repo git.Repo, planDigest string) (Snapshot, error) {
	entries, err := repo.IndexEntries(ctx)
	if err != nil {
		return Snapshot{}, err
	}
	changes, err := repo.Status(ctx)
	if err != nil {
		return Snapshot{}, err
	}

	submodules := map[string]bool{}
	for _, e := range entries {
		if e.IsSubmodule() {
			submodules[e.Path] = true
		}
	}

	head, branch := repo.Head(ctx)
	snap := Snapshot{
		Version:    Version,
		Head:       head,
		Branch:     branch,
		PlanDigest: planDigest,
		Clean:      len(changes) == 0,
	}

	h := sha256.New()
	fmt.Fprintf(h, "zeroturn.snapshot/%d\n", Version)
	writeField(h, "head", snap.Head)
	writeField(h, "plan", planDigest)

	// The index carries the content of everything staged, as a hash Git
	// has already computed. Hashing it here costs nothing and covers
	// staged edits, staged deletions, staged renames, and file modes.
	fmt.Fprintf(h, "index %d\n", len(entries))
	for _, e := range entries {
		fmt.Fprintf(h, "%s %s %s ", e.Mode, e.Blob, e.Stage)
		writeField(h, "path", e.Path)
	}

	// Only paths the working tree disagrees with Git about need reading.
	// Anything else is byte for byte what the index already hashed above,
	// so this reads the changed set, which is small, rather than the
	// repository, which is not.
	states, counts, err := worktreeStates(repo.Root, changes, submodules)
	if err != nil {
		return Snapshot{}, err
	}
	snap.TrackedChanges = counts.tracked
	snap.UntrackedFiles = counts.untracked
	snap.DirtySubmodules = counts.submodules

	fmt.Fprintf(h, "worktree %d\n", len(states))
	for _, s := range states {
		fmt.Fprintf(h, "%s ", s.kind)
		writeField(h, "path", s.path)
		writeField(h, "content", s.content)
	}

	snap.Digest = hex.EncodeToString(h.Sum(nil))
	return snap, nil
}

// writeField writes one value with its length in front of it. Without the
// length, a path ending in a newline, or one holding the word the next
// field starts with, could be read as two fields, and two different
// repositories would produce one digest.
func writeField(h hash.Hash, name, value string) {
	fmt.Fprintf(h, "%s:%d:%s\n", name, len(value), value)
}

// pathState is what the working tree holds for one changed path.
type pathState struct {
	path    string
	kind    string
	content string
}

type counts struct{ tracked, untracked, submodules int }

const (
	kindFile      = "file"
	kindExec      = "exec"
	kindSymlink   = "symlink"
	kindAbsent    = "absent"
	kindSubmodule = "submodule"
	kindOpaque    = "opaque"
)

func worktreeStates(root string, changes []git.Change, submodules map[string]bool) ([]pathState, counts, error) {
	var c counts
	out := make([]pathState, 0, len(changes))
	seen := make(map[string]bool, len(changes))
	for _, ch := range changes {
		if seen[ch.Path] {
			continue
		}
		seen[ch.Path] = true
		if ch.Code == "??" {
			c.untracked++
		} else {
			c.tracked++
		}
		if submodules[ch.Path] {
			c.submodules++
			// Git reports that the submodule moved or is dirty and this
			// records that it did. What changed inside it is not read.
			out = append(out, pathState{ch.Path, kindSubmodule, ch.Code})
			continue
		}
		st, err := readPath(filepath.Join(root, filepath.FromSlash(ch.Path)))
		if err != nil {
			return nil, counts{}, err
		}
		st.path = ch.Path
		out = append(out, st)
	}
	// Git lists paths in its own order. Sorting makes the digest depend on
	// what is there rather than on the order it was reported in.
	sort.Slice(out, func(i, j int) bool { return out[i].path < out[j].path })
	return out, c, nil
}

func readPath(abs string) (pathState, error) {
	info, err := os.Lstat(abs)
	if os.IsNotExist(err) {
		// A deleted path is part of the state. Recording it as absent
		// distinguishes a repository with the file removed from one that
		// never had it.
		return pathState{kind: kindAbsent}, nil
	}
	if err != nil {
		return pathState{}, err
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		// The link is the state, not whatever it points at. Following it
		// could leave the repository or run in a circle.
		target, rerr := os.Readlink(abs)
		if rerr != nil {
			return pathState{kind: kindOpaque, content: "unreadable-link"}, nil
		}
		return pathState{kind: kindSymlink, content: filepath.ToSlash(target)}, nil
	case info.IsDir():
		// With untracked files listed individually, the only directory Git
		// reports is one holding a repository of its own that the index
		// does not record as a submodule.
		return pathState{kind: kindOpaque, content: "directory"}, nil
	case !info.Mode().IsRegular():
		// A socket or a device has no contents to hash and reading one
		// could block. It is recorded as present and opaque.
		return pathState{kind: kindOpaque, content: info.Mode().String()}, nil
	}

	sum, err := hashFile(abs)
	if err != nil {
		// A file that cannot be read still counts as being there. Failing
		// the whole snapshot over one unreadable file would mean no
		// evidence at all rather than evidence with a gap in it.
		return pathState{kind: kindOpaque, content: "unreadable"}, nil
	}
	kind := kindFile
	// The executable bit changes what a validation step does with a file.
	// It reads differently on Windows, which does not matter: a snapshot
	// is only ever compared with another taken on the same machine.
	if info.Mode().Perm()&0100 != 0 {
		kind = kindExec
	}
	return pathState{kind: kind, content: sum}, nil
}

// hashFile reads the file in fixed size pieces, so a large file costs the
// same memory as a small one.
func hashFile(abs string) (string, error) {
	f, err := os.Open(abs)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Short is the digest as it is shown to a person. The full value is what
// is compared; nothing reads the short form back.
func (s Snapshot) Short() string {
	if len(s.Digest) < 12 {
		return s.Digest
	}
	return s.Digest[:12]
}

// Differences names what changed between the snapshot evidence was
// recorded against and the one taken now, in the words a person would
// use. It is built from what is stored rather than from the contents,
// which is why the working tree is named as a whole.
func (s Snapshot) Differences(now Snapshot) []string {
	if s.Digest == now.Digest {
		return nil
	}
	var out []string
	if s.Head != now.Head {
		out = append(out, "the commit changed")
	}
	if s.PlanDigest != now.PlanDigest {
		out = append(out, "the validation steps changed")
	}
	if s.Version != now.Version {
		out = append(out, "ZeroTurn now records repository state differently")
	}
	if len(out) == 0 {
		out = append(out, "the working tree changed")
	}
	return out
}

// Describe writes the differences as one sentence.
func Describe(reasons []string) string {
	switch len(reasons) {
	case 0:
		return ""
	case 1:
		return reasons[0]
	case 2:
		return reasons[0] + " and " + reasons[1]
	default:
		return strings.Join(reasons[:len(reasons)-1], ", ") + ", and " + reasons[len(reasons)-1]
	}
}
