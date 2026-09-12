# Changelog

Notable changes, newest first. The project follows semantic versioning from the first release.

## Unreleased

First prototype. Nothing has been released yet.

**Session Guard**

- Status line showing context, five hour and seven day usage, session duration, and active subagents.
- Subagent gate through `PreToolUse` with the exact `Agent` matcher, in observe, confirm, and strict modes.
- Subagent counting from harness start and stop events, which no harness reports itself.
- Strict mode requires approval on the machine, so a committed configuration cannot deny subagents for everyone.

**Credential guard**

- Before the model reads a file, that file is scanned and the harness asks, or denies, when it holds something shaped like a credential. Modes: `ask`, the default, `deny`, and `off`.
- The message names the file, the line, and the category, never the value.

- Optional prompt guard, off by default: with `guard.credentials.prompts` set to `block`, a message carrying a credential is stopped before it is sent. The hook is installed only when the guard is on.

**Direct Lane**

- `zeroturn verify` runs approved commands as argument arrays, without a shell, keeping full logs and printing a redacted excerpt.
- `zeroturn ship` stages only named files, scans them for credentials, refuses unsafe Git states, and pushes without force.

**Contract and safety**

- Published JSON schemas and a conformance suite.
- Local records only, with a seven day retention and a purge command.
- Claude Code integration with plan, apply, and remove, writing to the per user project settings file.
