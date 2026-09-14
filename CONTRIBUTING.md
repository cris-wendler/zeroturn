# Contributing

Thank you for considering a contribution. Follow the four steps below, in order.

![Four steps. 1 Check: is it already there, read the commands and run zeroturn capabilities. 2 Search: look through issues, open and closed, and add to one that matches. 3 Open an issue: a bug report or a feature proposal, and wait for a reply before big work. 4 Pull request: from a fork, with tests, referencing the issue, and the checks must pass. Small fixes can go straight to step 4, and a security problem is never reported in a public issue.](docs/img/contributing.svg)

| Step | What to do | Why |
| --- | --- | --- |
| **1. Check** | Read the Commands section of the [README](README.md), run `zeroturn <command> --help`, and run `zeroturn capabilities --json`. Check [CHANGELOG.md](CHANGELOG.md). | It may already be there under another name. |
| **2. Search** | Look through the issues, open and closed. If one matches, add your details there. | Two issues for one problem split the discussion. |
| **3. Open an issue** | **Bug report** for something that does not work as documented, with the command, the output, `zeroturn version`, and `zeroturn doctor`. **Feature proposal** for something new, describing the problem before the change. | The reply tells you whether it fits, before you spend time on it. |
| **4. Pull request** | From a fork, on its own branch, referencing the issue, for example `Fixes #12`. | Review is faster when the reason is already agreed. |

> [!TIP]
> Small fixes such as a typo or a broken link can go straight to step 4.

> [!IMPORTANT]
> Never open a public issue for a security problem. Follow [SECURITY.md](SECURITY.md), which explains how to report it privately.

## What fits

ZeroTurn works with coding harnesses. It is not another coding harness. It shows session pressure, asks before more delegated work, and runs routine validation and Git workflows locally.

| Area | Examples |
| --- | --- |
| **Adapters** | A harness ZeroTurn does not support yet, following [docs/adapter-authoring.md](docs/adapter-authoring.md) |
| **Events** | Normalization, new fixtures, better handling of missing fields |
| **Detection** | Project detection for `zeroturn init`, validation presets for common toolchains |
| **Platforms** | Real use on Linux and Windows, where only the tests have run so far |
| **Safety** | Security and redaction tests, credential patterns that matter in practice |
| **Presentation** | Terminal rendering, accessibility, documentation, examples |

## What does not fit

> [!WARNING]
> These are closed or redirected however well they are written. The reasons are in [docs/product-boundary.md](docs/product-boundary.md) and [docs/decisions.md](docs/decisions.md).

| Not accepted | Why |
| --- | --- |
| A model SDK, model calls, or model selection | ZeroTurn never calls a model to decide whether a model call should happen |
| A chat interface or a complete coding harness | It works with harnesses, it does not replace them |
| Transcript collection or prompt inspection | It never reads what you or the model wrote |
| Remote telemetry or a hosted dashboard | Everything stays on the machine |
| Output compression, automatic handoff, compaction, or clearing | These change the session behind your back |
| Shell strings in configuration | Commands are argument arrays, so configuration cannot smuggle in shell syntax |
| Automatic force pushing or rebasing | Git history is the developer's to rewrite, never the tool's |
| A way around credential detection | A bypass makes the check worthless |
| Silent changes to global settings | Every change is planned, confirmed, and backed up first |
| Python packaging, or a container without a demonstrated need | One executable, no runtime to install |

If you are unsure, open an issue describing the change before writing it.

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

Pull requests are **squashed** into one commit, so `main` carries one commit per change with the detail in the pull request. The branch is deleted when it merges. The checks in [.github/workflows/ci.yml](.github/workflows/ci.yml) run on every pull request, and a red check blocks the merge. They cover Linux with both supported Go versions, and Windows. macOS runs weekly in [.github/workflows/macos.yml](.github/workflows/macos.yml), and can be started by hand from the Actions tab.

## Working on the code

Requirements: **Go 1.17 or newer** and **Git**. Nothing else. A pull request that adds a dependency needs a reason and a row in [docs/dependency-licenses.md](docs/dependency-licenses.md).

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

## Pull requests

- [ ] One change per pull request.
- [ ] Tests added or updated. A fix comes with a test that fails without it.
- [ ] Documentation updated where behavior changed.
- [ ] `go test ./...`, `go vet ./...`, and `scripts/lint-copy.sh` pass.
- [ ] No generated attribution, tool trailers, or prompt text in commits or files.

> [!NOTE]
> Exit codes, the event contract, and the JSON output are public. Changing what any of them means needs a major contract version, described in [docs/harness-contract.md](docs/harness-contract.md).

## License

ZeroTurn is licensed under `GPL-3.0-only`. By submitting a contribution you agree that it is licensed under the same terms. There is no contributor license agreement.
