# Changelog

Notable changes, newest first. The project follows semantic versioning from the first release.

## Unreleased

## 0.2.0, 2026-09-15

**Breaking**

- `policy show --json` prints the whole configuration. It printed the guard, so a reader took `.mode` from the top level and now takes `.guard.mode`. Retention is a setting outside the guard, and a document calling itself the policy while omitting a policy setting was wrong. The contract version is 2.0.0 for this.

**Added**

- `zeroturn uninstall` lists everything ZeroTurn put on this machine and, with `--apply`, removes it. Settings files are edited rather than deleted, each is copied aside first, and the executable is never deleted.
- `zeroturn policy migrate` rewrites `.zeroturn.json` in the format this build writes, with `--plan` to see it first. It is never required: a file is read whether or not it has been migrated.
- `report month` and `report --since`, which reads a date, a number of days, or a duration. `--all-repositories` counts the machine.
- `report.retentionDays`, so how long records are kept is a setting rather than a constant. The default is the seven days it has always been.
- `git.remote` can be set by name, like every guard setting.
- A report carries `scope`, saying whether it counted one repository or the machine, and `windowStart`, saying what the window covered.
- `doctor` has a third level. A warning is something you can act on, a note is something true that you cannot.

**Fixed**

- `report` counted every repository on the machine while saying "This repository" underneath. It now covers the repository it was run in.
- `verify` and `ship` did not run at all in a linked worktree or a submodule, where `.git` is a file rather than a directory.
- The Gradle wrapper step could never run on any platform, because the proposal named it without a path and `exec.LookPath` searched `PATH`. The Maven wrapper is honoured the same way now.
- The hand install file was missing the credential guard entirely, so anyone following the integration document lost it with nothing to tell them.
- A configuration file older than the build reading it now loads, with settings it does not carry taking the value a new file would have. The advice for a file from a newer build is to upgrade ZeroTurn rather than to run `init`, which would have overwritten the policy.
- Counted nouns across nine commands: "Result: 1 checks passed", "1 commits behind", "1 of 1 items", and more.
- `policy tune --days 30` printed a thirty day heading over records that are deleted after seven.
- `init --yes` printed the whole file it was about to write, which is the answer to a question nobody asked.
- Test isolation set only `HOME`, so on Windows every test that reached the user wide settings file wrote into the real home directory.

**Internal**

- Twelve tests that could not fail were rewritten, six of which survived the whole suite.
- `scripts/mutate` changes one operator at a time and reports what the tests accept. It runs in continuous integration.
- The test suite runs under the race detector and in a shuffled order.

## 0.1.0, 2026-09-14

First release.

**Session Guard**

- Status line showing context, five hour and seven day usage, session duration, and active subagents.
- Subagent gate through `PreToolUse` with the exact `Agent` matcher, in observe, confirm, and strict modes.
- Subagent counting from harness start and stop events, which no harness reports itself.
- Strict mode requires approval on the machine, so a committed configuration cannot deny subagents for everyone.
- Rate projection: the gate also asks when the rate of use implies the five hour window runs out before it resets, whatever the reading is now. A heavy session that will still finish inside the window stays quiet. Setting `guard.limits.projection`, on by default.

**Credential guard**

- 31 patterns, covering the services whose keys turn up most often, with a corpus of ordinary repository content that must not trigger it.
- Scanning runs at about 27 MB/s on ordinary source, after literal anchors were added so most lines never reach a regular expression.

- Before the model reads a file, that file is scanned and the harness asks, or denies, when it holds something shaped like a credential. Modes: `ask`, the default, `deny`, and `off`.
- The message names the file, the line, and the category, never the value.

- Optional prompt guard, off by default: with `guard.credentials.prompts` set to `block`, a message carrying a credential is stopped before it is sent. The hook is installed only when the guard is on.

**Learning from its own decisions**

- `zeroturn policy tune` proposes thresholds from what the gate asked and whether a subagent followed, which is how approval is inferred. Counts and measurements only, five observations before it says anything, and it never changes a setting.

**Direct Lane**

- `init` recognises Python, Maven, Gradle, .NET and Ruby projects as well as Go, JavaScript and Rust, and reads a Makefile for the targets it declares.

- `zeroturn verify` runs approved commands as argument arrays, without a shell, keeping full logs and printing a redacted excerpt.
- `zeroturn ship` stages only named files, scans them for credentials, refuses unsafe Git states, and pushes without force.

**Keeping the integration working**

- `doctor` reports when the settings point at an executable that has moved or is no longer there, since hooks in that state fail in silence.
- `integrate claude --apply` repairs those entries, and leaves commands it does not own untouched.

**Research corrections**

- The reason Copilot support was cut turned out to be wrong for the shipped CLI, which does expose hooks for tool use and subagents, a pre tool decision, quota snapshots, and context token counts. The claim is corrected and an adapter is planned, with support claimed only once it has run against a real session.

**Contract and safety**

- Published JSON schemas and a conformance suite.
- Local records only, with a seven day retention and a purge command.
- Claude Code integration with plan, apply, and remove, writing to the per user project settings file.
