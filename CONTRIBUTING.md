# Contributing

Thank you for considering a contribution. Follow the four steps below, in order.

1. **Check it is not already there.** Read the Commands section of the
   [README](README.md), run `zeroturn <command> --help`, and run
   `zeroturn capabilities --json`. Check [CHANGELOG.md](CHANGELOG.md). It
   may already exist under another name.

2. **Search the issues**, open and closed. If one matches, add your
   details there rather than opening a second one for the same problem.

3. **Open an issue.** A bug report for something that does not work as
   documented, with the command, the output, `zeroturn version` and
   `zeroturn doctor`. A feature proposal for something new, describing
   the problem before the change. The reply tells you whether it fits
   before you spend time on it.

4. **Open a pull request** from a fork, on its own branch, referencing
   the issue, for example `Fixes #12`. Review is faster when the reason
   is already agreed.

Small fixes such as a typo or a broken link can go straight to step 4.

> [!IMPORTANT]
> Never open a public issue for a security problem. Follow [SECURITY.md](SECURITY.md), which explains how to report it privately.

## What fits

ZeroTurn works with coding harnesses. It is not another coding harness. It shows session pressure, asks before more delegated work, and runs routine validation and Git workflows locally.

Useful areas:

- adapters for a harness ZeroTurn does not support yet, following the schemas in [schemas/](schemas) and the worked example in [integrations/template/](integrations/template)
- events: normalization, new fixtures, better handling of missing fields
- detection: project detection for `zeroturn init`, validation presets for common toolchains
- platforms: real use on Linux and Windows, where only the tests have run so far
- safety: security and redaction tests, credential patterns that matter in practice
- presentation: terminal rendering, accessibility, documentation, examples

## What does not fit

These are closed or redirected however well they are written. The reasons are in [docs/decisions.md](docs/decisions.md).

A model SDK, model calls, or model selection are out, because ZeroTurn
never calls a model to decide whether a model call should happen. So are a
chat interface or a complete coding harness, since it works with harnesses
rather than replacing them.

Transcript collection and prompt inspection are out because it never reads
what you or the model wrote, and remote telemetry or a hosted dashboard
because everything stays on the machine. Output compression, automatic
session handoff, compaction and clearing all change the session behind
your back. A handoff record written into a repository for a person to
read is a different thing and is not covered by this.

The rest are safety rules. Commands are argument arrays, so shell strings
in configuration cannot smuggle in shell syntax. Git history is the
developer's to rewrite, so nothing force pushes or rebases on its own. A
way around credential detection makes the check worthless. Global settings
are never changed silently: every change is planned, confirmed, and backed
up first. Python packaging and containers stay out without a demonstrated
need, because it is one executable with no runtime to install.

## Branches and pull requests

Nothing goes straight to `main`. Every change, including documentation, goes on a branch and through a pull request, so the history shows what changed and why.

| Prefix | For |
| --- | --- |
| `feat/` | A new behavior |
| `fix/` | A correction to existing behavior |
| `docs/` | Documentation, images, and the recording |
| `test/` | Tests without a behavior change |
| `chore/` | Build, tooling, continuous integration |

```sh
git checkout -b docs/short-description
git commit ...
git push --set-upstream origin docs/short-description
gh pr create --fill
```

Pull requests are **squashed** into one commit, so `main` carries one commit per change with the detail in the pull request. The branch is deleted when it merges. The checks in [.github/workflows/ci.yml](.github/workflows/ci.yml) run on every pull request, and a red check blocks the merge. They cover Linux with both supported Go versions, macOS, and Windows.

## Working on the code

Requirements: **Go 1.17 or newer** and **Git**. Nothing else. A pull request that adds a dependency needs a reason good enough to change that.

| Command | What it does |
| --- | --- |
| `go build -o zeroturn ./cmd/zeroturn` | Builds the executable |
| `go test ./...` | Runs every test, including the conformance suite, in about a minute |
| `go vet ./...` | The standard Go checks |
| `scripts/lint-copy.sh` | The writing rules below |
| `scripts/check-commit-messages.sh` | Refuses a commit message that credits a coding tool |
| `go run ./scripts/bench ./zeroturn 50` | Measures the commands that run inside a session |

The tests build the executable, create temporary repositories with local bare remotes, and point Git and ZeroTurn at temporary configuration. They never contact a real remote or change your own settings.

| Path | Contents |
| --- | --- |
| `cmd/zeroturn` | Commands, flags, and output |
| `internal/policy` | Turns session values into a gate decision |
| `internal/state` | Local session records, locking, retention |
| `internal/events` | Reads harness payloads and discards everything not permitted |
| `internal/git` | The only Git commands ZeroTurn runs |
| `internal/security` | Credential detection and redaction |
| `internal/trust` | Approval of repository commands and of Strict mode |
| `schemas`, `conformance` | The published contract, and the suite that checks output against it |
| `fixtures` | Harness payloads used by tests |
| `docs/demo` | The README recording and its renderer |

## Writing rules

These apply to code comments, CLI output, error messages, and documentation. `scripts/lint-copy.sh` checks most of them.

- **Describe what happens.** No promotional words, no claims that were not measured.
- **No em dashes or en dashes.**
- **Comment only** a security decision, a Git safety rule, a compatibility limit, a public contract, or a choice that is not obvious from the code.
- **Errors state three things:** what stopped, why, and the smallest safe next step. Use `output.Errorf`.
- **No emoji in default output**, and no decorative banners.
- **Never call event counts savings.**

## Derive a description, do not maintain one

The most common defect in this repository has been a description kept by hand beside the thing it describes, drifting away from it. A list of permitted fields beside a type, a table of settings beside a struct, a count in a document beside the repository, required checks beside the jobs that produce them.

So: when a change adds something that is named or counted in a second place, add a test that reads the second place from the first, rather than updating both and hoping.

- Compare against the type, with reflection, not against a sample value. A sample only shows what it happens to hold.
- Fail in both directions. An entry that is missing and an entry that no longer exists are both drift.
- Say what to correct. `doccount_test.go` names the sentence and the number, so fixing it takes one edit.

Nine tests here do this. They have caught unlisted fields in the stored record, a contract version no adapter could send, broken help on seven commands, and stale counts in the documentation. The drift that had no such check was found by accident instead, once by a pre flight read through that happened to notice a branch rule requiring a check that no longer existed.

This matters more when a coding agent is writing the changes, because it holds no memory of the parallel lists and makes more changes per day than a person does.

## Pull requests

- [ ] One change per pull request.
- [ ] Tests added or updated. A fix comes with a test that fails without it.
- [ ] Documentation updated where behavior changed.
- [ ] `go test ./...`, `go vet ./...`, and `scripts/lint-copy.sh` pass.
- [ ] No generated attribution, tool trailers, or prompt text in commits or files.

Exit codes, the event contract, and the JSON output are public. Changing what any of them means needs a major contract version, which `zeroturn capabilities --json` reports.

## License

ZeroTurn is licensed under `GPL-3.0-only`. By submitting a contribution you agree that it is licensed under the same terms. There is no contributor license agreement.
