# Going public

The repository is private while the gate is being tried in real sessions. These are the steps for the day it becomes public, and what each one is for.

## Before

| Check | Why |
| --- | --- |
| `go test ./...`, `go vet ./...`, `scripts/lint-copy.sh` pass on `main` | Nothing broken on the first day |
| `zeroturn doctor --compat --live` passes | The one claim that matters is still true on the current harness |
| The README says what is unproven | The interactive approval prompt, and use on Linux and Windows |
| No private paths, names, or addresses | `git log --format='%an <%ae>' \| sort -u` and a read through `docs/` |
| The recording matches the current output | Rerun `docs/demo/record.sh` and the renderer |

> [!CAUTION]
> Making a repository public is effectively permanent. Forks and caches survive turning it private again, and so does everything in the history.

## The switch

```sh
gh repo edit cris-wendler/zeroturn --visibility public --accept-visibility-change-consequences
```

## Straight after

**Protect the branch.** Rulesets need a public repository or a paid plan, which is why the rule is kept here as a file:

```sh
gh api -X POST repos/cris-wendler/zeroturn/rulesets --input .github/rulesets/main.json
```

It requires a pull request for `main`, requires the four checks that run on a pull request to pass, allows squash merges only, and refuses deletion and force pushes. macOS is deliberately not among them: it runs weekly in its own workflow, so requiring it here would block every pull request on a check that never reports.

**Switch on private vulnerability reporting**, which [SECURITY.md](../SECURITY.md) tells people to use:

```sh
gh api -X PUT repos/cris-wendler/zeroturn/private-vulnerability-reporting
```

**Check that installation works from a clean machine:**

```sh
go install github.com/cris-wendler/zeroturn/cmd/zeroturn@latest
zeroturn version
```

If that works, add the command to the README installation section, which currently describes building from source.

## Then, when there is something to release

Follow [release.md](release.md). The tag opens a draft release; publishing it stays a deliberate step.
