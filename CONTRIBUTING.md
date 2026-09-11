# Contributing

Thank you for considering a contribution. This file explains what fits the project, how to work on it, and what a pull request needs.

## What fits

ZeroTurn works with coding harnesses. It is not another coding harness. It shows session pressure, asks before more delegated work, and runs routine validation and Git workflows locally.

Useful areas:

- coding harness adapters that follow the event contract
- event normalization and new fixtures
- project detection for `zeroturn init`
- validation presets for common toolchains
- platform support, especially Linux and Windows testing
- security and redaction tests
- terminal rendering and accessibility
- documentation and examples
- conformance fixtures

## What does not fit

Pull requests that add any of the following will be closed or redirected, however well they are written:

- a model SDK, model calls, or model selection
- a chat interface or a complete coding harness
- transcript collection or prompt inspection
- remote telemetry or a hosted dashboard
- generic output compression
- automatic handoff generation, automatic compaction, or automatic session clearing
- shell strings in configuration
- automatic force pushing or automatic rebasing
- a way to bypass credential detection
- silent changes to global settings
- Python packaging or a Python wrapper
- a container image without a demonstrated need

If you are unsure, open an issue describing the change before writing it.

## Working on the code

Requirements: Go 1.17 or newer and Git. No other dependency is used, and a pull request that adds one needs a reason and an entry in the dependency record.

```sh
go build -o zeroturn ./cmd/zeroturn
go vet ./...
go test ./...
scripts/lint-copy.sh
```

The tests build the executable, create temporary repositories with local bare remotes, and point Git and ZeroTurn at temporary configuration. They never contact a real remote or change your own settings. The full run takes about a minute.

Layout:

| Path | Contents |
| --- | --- |
| `cmd/zeroturn` | commands, flags, and output |
| `internal/policy` | turns session values into a gate decision |
| `internal/state` | local session records, locking, retention |
| `internal/events` | reads harness payloads and discards everything not permitted |
| `internal/git` | the only Git commands ZeroTurn runs |
| `internal/security` | credential detection and redaction |
| `internal/trust` | approval of repository commands and of Strict mode |
| `fixtures` | harness payloads used by tests |
| `docs/demo` | the README recording and its renderer |

## Writing rules

These apply to code comments, CLI output, error messages, and documentation.

- Describe what happens. Leave out promotional words and claims that are not measured.
- Do not use em dashes or en dashes.
- Comment only a security decision, a Git safety rule, a compatibility limit, a public contract, or a choice that is not obvious from the code.
- Every error states what stopped, why, and the smallest safe next step. Use `output.Errorf`.
- No emoji in default output and no decorative banners.
- Do not describe event counts as savings.

`scripts/lint-copy.sh` checks most of these.

## Pull requests

- Keep a pull request to one change.
- Add or update tests. A fix should come with a test that fails without it.
- Update the README or the relevant document when behavior changes.
- Exit codes, the event contract, and JSON output are public. Changing their meaning needs a major contract version.
- Do not add generated attribution, tool trailers, or prompt text to commits or files.

## License

ZeroTurn is licensed under `GPL-3.0-only`. By submitting a contribution you agree that it is licensed under the same terms. There is no contributor license agreement.
