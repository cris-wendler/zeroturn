package main

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/cris-wendler/zeroturn/internal/config"
	"github.com/cris-wendler/zeroturn/internal/git"
	"github.com/cris-wendler/zeroturn/internal/output"
	"github.com/cris-wendler/zeroturn/internal/security"
	"github.com/cris-wendler/zeroturn/internal/state"
	"github.com/cris-wendler/zeroturn/internal/trust"
	"github.com/cris-wendler/zeroturn/internal/verify"
)

type shipArgs struct {
	message string
	files   []string
	dryRun  bool
	skipVfy bool
	asJSON  bool
}

const shipUsage = `zeroturn ship --message "<text>" --files <path> [<path> ...] [--dry-run]

  --message     Commit message, written by you
  --files       The exact files to stage, listed explicitly
  --dry-run     Inspect, scan, validate, and fetch without staging or pushing
  --no-verify-steps
                Skip the configured validation steps for this run only
`

// parseShip reads the ship flags. The flag package cannot express a
// repeated trailing value list, so --files consumes arguments until the
// next flag.
func parseShip(args []string) (shipArgs, error) {
	var a shipArgs
	bad := func(detail, fix string) error {
		return output.Errorf(output.ExitInvalidUsage, "zeroturn ship changed nothing", detail, fix)
	}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--message", "-m":
			if i+1 >= len(args) {
				return a, bad("--message was given without any text", "supply a commit message you wrote")
			}
			i++
			a.message = args[i]
		case "--files", "-f":
			for i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				i++
				a.files = append(a.files, args[i])
			}
			if len(a.files) == 0 {
				return a, bad("--files was given without any paths", "list the exact files to stage")
			}
		case "--dry-run":
			a.dryRun = true
		case "--no-verify-steps":
			a.skipVfy = true
		case "--json":
			a.asJSON = true
		case "--help", "-h":
			fmt.Print(shipUsage)
			os.Exit(output.ExitOK)
		default:
			return a, bad("flag "+args[i]+" is not one zeroturn ship accepts", "run zeroturn ship --help")
		}
	}
	if strings.TrimSpace(a.message) == "" {
		return a, bad("no commit message was supplied", "add --message with the text you want on the commit")
	}
	if len(a.files) == 0 {
		return a, bad("no files were supplied",
			"add --files with the exact paths to stage, ZeroTurn never stages everything for you")
	}
	return a, nil
}

type shipPath struct {
	Rel     string
	Abs     string
	Deleted bool
}

func cmdShip(ctx context.Context, args []string) error {
	a, err := parseShip(args)
	if err != nil {
		return err
	}
	repo, err := openRepo(ctx)
	if err != nil {
		return err
	}
	c, err := loadConfig(repo.Root)
	if err != nil {
		return err
	}

	branch, berr := repo.Branch(ctx)
	if berr != nil {
		return output.Errorf(output.ExitUnsafeGit, "zeroturn ship changed nothing",
			"the current branch could not be resolved", "check that the repository has at least one commit")
	}
	if c.IsProtected(branch) {
		return output.Errorf(output.ExitUnsafeGit, "zeroturn ship changed nothing",
			"branch "+branch+" is protected by "+config.FileName,
			"switch to a working branch, or remove it from git.protectedBranches")
	}

	wd, err := os.Getwd()
	if err != nil {
		return output.Errorf(output.ExitInternal, "zeroturn ship changed nothing",
			"the working directory could not be read", "run the command again from inside the repository")
	}
	paths, err := resolveShipPaths(ctx, repo, wd, a.files)
	if err != nil {
		return err
	}

	if err := refuseUnrelatedStaged(ctx, repo, paths); err != nil {
		return err
	}

	findings, err := scanShipPaths(paths)
	if err != nil {
		return err
	}
	if len(findings) > 0 {
		fmt.Fprintln(os.Stderr, "Credential indicators were found in the files you asked to ship:")
		for _, f := range findings {
			fmt.Fprintf(os.Stderr, "  %s\n", f.String())
		}
		return output.Errorf(output.ExitPolicyFailure,
			"zeroturn ship changed nothing",
			"the proposed content contains "+strings.Join(security.Categories(findings), ", "),
			"remove the credential from the file and rotate it, then run zeroturn ship again")
	}

	remote := c.Git.Remote
	if !repo.HasRemote(ctx, remote) {
		return output.Errorf(output.ExitUnsafeGit, "zeroturn ship changed nothing",
			"remote "+remote+" is not configured in this repository",
			"add the remote, or change git.remote in "+config.FileName)
	}

	fmt.Println("ZEROTURN SHIP")
	fmt.Println("files:")
	for _, p := range paths {
		if p.Deleted {
			fmt.Printf("  %s  (deletion)\n", p.Rel)
			continue
		}
		fmt.Printf("  %s\n", p.Rel)
	}
	fmt.Printf("message:  %s\n", a.message)
	fmt.Printf("branch:   %s\n", branch)
	fmt.Printf("remote:   %s  %s\n", remote, repo.RemoteDisplay(ctx, remote))
	fmt.Println()

	if !a.skipVfy && len(c.Verify.Steps) > 0 {
		st, serr := openStore()
		if serr != nil {
			return serr
		}
		if ts := trust.Check(st, repo.Root, c); !ts.Trusted {
			return output.Errorf(output.ExitDeclined, "zeroturn ship changed nothing",
				"the configured validation commands are not approved: "+ts.Reason,
				"run zeroturn verify --approve, or run ship with --no-verify-steps")
		}
		if missing := verify.MissingExecutables(c); len(missing) > 0 {
			return output.Errorf(output.ExitMissingExec, "zeroturn ship changed nothing",
				"these executables were not found on PATH: "+strings.Join(missing, ", "),
				"install them, or run ship with --no-verify-steps")
		}
		res, rerr := verify.Run(ctx, c, verify.Options{RepoRoot: repo.Root, Progress: os.Stdout})
		if rerr != nil {
			return output.Errorf(output.ExitInternal, "zeroturn ship changed nothing",
				rerr.Error(), "check that the repository .git directory is writable")
		}
		printVerify(res)
		if res.Failed > 0 || res.Cancelled {
			return output.Errorf(output.ExitPolicyFailure, "zeroturn ship changed nothing",
				"validation did not pass", "fix the failure above, then run zeroturn ship again")
		}
		fmt.Println()
	}

	if ferr := repo.Fetch(ctx, remote); ferr != nil {
		return output.Errorf(output.ExitUnsafeGit, "zeroturn ship changed nothing",
			"the fetch from "+remote+" failed: "+git.StripCredentials(ferr.Error()),
			"check your network and remote access, then run zeroturn ship again")
	}
	div, derr := repo.Divergence(ctx)
	if derr != nil {
		return output.Errorf(output.ExitUnsafeGit, "zeroturn ship changed nothing",
			derr.Error(), "check the branch state with git status")
	}
	if err := refuseUnsafeDivergence(div, branch, remote); err != nil {
		return err
	}

	if a.dryRun {
		fmt.Println("READY TO SHIP")
		fmt.Println("Dry run finished. Nothing was staged, committed, or pushed.")
		return nil
	}

	ok, cerr := confirm(fmt.Sprintf("Stage %d file(s), commit, and push to %s/%s?", len(paths), remote, branch))
	if cerr != nil {
		return cerr
	}
	if !ok {
		return output.Errorf(output.ExitDeclined, "zeroturn ship changed nothing",
			"confirmation was declined", "run zeroturn ship again when you are ready")
	}

	rels := make([]string, 0, len(paths))
	for _, p := range paths {
		rels = append(rels, p.Rel)
	}
	if aerr := repo.Add(ctx, rels); aerr != nil {
		return output.Errorf(output.ExitUnsafeGit, "zeroturn ship stopped after staging failed",
			aerr.Error(), "inspect the index with git status, nothing was committed")
	}

	staged, sterr := repo.StagedPaths(ctx)
	if sterr != nil {
		return output.Errorf(output.ExitUnsafeGit, "zeroturn ship stopped before committing",
			sterr.Error(), "inspect the index with git status")
	}
	if extra := difference(staged, rels); len(extra) > 0 {
		return output.Errorf(output.ExitUnsafeGit,
			"zeroturn ship stopped before committing",
			"the index contains files you did not list: "+strings.Join(extra, ", "),
			"unstage them with git restore --staged, then run zeroturn ship again")
	}
	if len(staged) == 0 {
		return output.Errorf(output.ExitPolicyFailure, "zeroturn ship stopped before committing",
			"the listed files contain no changes to commit",
			"check the files with git status, nothing was committed")
	}

	sha, coerr := repo.Commit(ctx, a.message)
	if coerr != nil {
		return output.Errorf(output.ExitUnsafeGit, "zeroturn ship stopped after staging",
			coerr.Error(), "the files are staged and nothing was pushed, inspect with git status")
	}

	if _, perr := repo.Push(ctx, remote, branch); perr != nil {
		return output.Errorf(output.ExitUnsafeGit,
			"zeroturn ship committed but did not push",
			git.StripCredentials(perr.Error()),
			"commit "+repo.ShortSHA(ctx, sha)+" is on your local branch, push it when the remote is reachable")
	}

	if st, serr := openStore(); serr == nil {
		if sess, found := recentSession(st, repo.Root); found {
			st.Update(sess.SessionID, "", func(s *state.Session) { s.DirectGitOps++ })
		}
	}

	fmt.Printf("Pushed %s to %s/%s\n", repo.ShortSHA(ctx, sha), remote, branch)
	return nil
}

// resolveShipPaths reads relative paths from the working directory, as git
// does, and refuses any path that leaves the repository, checked after
// symbolic links are resolved rather than by inspecting the text. A path
// that does not exist counts as a deletion only when Git tracks it, so a
// mistyped name is refused instead of being shown as ready to ship.
func resolveShipPaths(ctx context.Context, repo git.Repo, wd string, files []string) ([]shipPath, error) {
	root := repo.Root
	if resolved, err := filepath.EvalSymlinks(wd); err == nil {
		wd = resolved
	}
	var out []shipPath
	seen := map[string]bool{}
	for _, f := range files {
		abs := f
		if !filepath.IsAbs(abs) {
			abs = filepath.Join(wd, f)
		}
		abs = filepath.Clean(abs)
		info, statErr := os.Lstat(abs)
		deleted := os.IsNotExist(statErr)

		// named is the entry the user listed, with its directory resolved.
		// target follows a final symbolic link. Both must stay inside the
		// repository, but only named is staged, so a link is shipped as
		// the link rather than as the file it points to.
		named := abs
		if dir, derr := filepath.EvalSymlinks(filepath.Dir(abs)); derr == nil {
			named = filepath.Join(dir, filepath.Base(abs))
		}
		target := named
		if resolved, rerr := filepath.EvalSymlinks(named); rerr == nil {
			target = resolved
		}
		rel, inside := relInside(root, named)
		if _, targetInside := relInside(root, target); !inside || !targetInside {
			return nil, output.Errorf(output.ExitInvalidUsage,
				"zeroturn ship changed nothing",
				"path "+f+" is outside the repository",
				"list only files inside "+root)
		}
		if !deleted && info != nil && info.IsDir() {
			return nil, output.Errorf(output.ExitInvalidUsage,
				"zeroturn ship changed nothing",
				"path "+f+" is a directory",
				"list the individual files you want to ship")
		}
		rel = filepath.ToSlash(rel)
		if deleted && !repo.IsTracked(ctx, rel) {
			return nil, output.Errorf(output.ExitInvalidUsage,
				"zeroturn ship changed nothing",
				"path "+f+" does not exist and is not tracked by Git",
				"check the spelling, or list the file after creating it")
		}
		if seen[rel] {
			continue
		}
		seen[rel] = true
		out = append(out, shipPath{Rel: rel, Abs: abs, Deleted: deleted})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Rel < out[j].Rel })
	return out, nil
}

func relInside(root, p string) (string, bool) {
	rel, err := filepath.Rel(root, p)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	return rel, true
}

func refuseUnrelatedStaged(ctx context.Context, repo git.Repo, paths []shipPath) error {
	staged, err := repo.StagedPaths(ctx)
	if err != nil {
		return output.Errorf(output.ExitUnsafeGit, "zeroturn ship changed nothing",
			err.Error(), "inspect the index with git status")
	}
	want := map[string]bool{}
	for _, p := range paths {
		want[p.Rel] = true
	}
	var extra []string
	for _, s := range staged {
		if !want[s] {
			extra = append(extra, s)
		}
	}
	if len(extra) > 0 {
		return output.Errorf(output.ExitUnsafeGit,
			"zeroturn ship changed nothing",
			"the index already holds files you did not list: "+strings.Join(extra, ", "),
			"commit or unstage them first, ZeroTurn will not include files you did not name")
	}
	return nil
}

func scanShipPaths(paths []shipPath) ([]security.Finding, error) {
	var all []security.Finding
	for _, p := range paths {
		if p.Deleted {
			continue
		}
		b, err := ioutil.ReadFile(p.Abs)
		if err != nil {
			return nil, output.Errorf(output.ExitInvalidUsage,
				"zeroturn ship changed nothing",
				"file "+p.Rel+" could not be read",
				"check the path and your permissions")
		}
		f, serr := security.ScanBytes(p.Rel, b)
		if serr != nil {
			return nil, output.Errorf(output.ExitInternal,
				"zeroturn ship changed nothing",
				"file "+p.Rel+" could not be scanned",
				"the file may be unusually large, split it or remove it from this commit")
		}
		all = append(all, f...)
	}
	return all, nil
}

func refuseUnsafeDivergence(d git.Divergence, branch, remote string) error {
	if !d.HasUpsrm {
		return nil
	}
	if d.Diverged {
		return output.Errorf(output.ExitUnsafeGit,
			"zeroturn ship changed nothing",
			fmt.Sprintf("branch %s is %d ahead and %d behind %s", branch, d.Ahead, d.Behind, d.Upstream),
			"reconcile the histories yourself with the merge or rebase you intend, then run zeroturn ship again")
	}
	if d.Behind > 0 {
		return output.Errorf(output.ExitUnsafeGit,
			"zeroturn ship changed nothing",
			fmt.Sprintf("branch %s is %d commits behind %s", branch, d.Behind, d.Upstream),
			"bring the branch up to date with git merge --ff-only "+d.Upstream+", then run zeroturn ship again")
	}
	return nil
}

func difference(have, want []string) []string {
	w := map[string]bool{}
	for _, x := range want {
		w[x] = true
	}
	var out []string
	for _, x := range have {
		if !w[x] {
			out = append(out, x)
		}
	}
	return out
}
