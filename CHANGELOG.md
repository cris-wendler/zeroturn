# Changelog

Notable changes, newest first. The project follows semantic versioning from the first release.

## Unreleased

First prototype. Nothing has been released yet.

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
