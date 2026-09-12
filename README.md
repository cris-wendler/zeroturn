# ZeroTurn

[![Tests on Linux, macOS, and Windows](https://github.com/cris-wendler/zeroturn/actions/workflows/ci.yml/badge.svg)](https://github.com/cris-wendler/zeroturn/actions/workflows/ci.yml)

Know when a coding session is under pressure. Keep routine development work local.

ZeroTurn shows context, usage windows, session duration, and subagent activity when the coding harness provides them. It can request approval before additional delegation and can run validation and safe Git workflows directly on your machine.

ZeroTurn works with coding harnesses. It is not another coding harness.

![Terminal recording. The ZeroTurn status line shows context at 82 percent, five hour usage at 81 percent, seven day usage at 47 percent, a session of 3 hours 12 minutes, 2 active subagents, and the word ask. zeroturn policy check shows the decision ask because context, five hour usage, and active subagents are past their thresholds. The credential guard then stops a file that holds an aws access key id from being read, and, with the prompt guard switched on, stops a message carrying the same key from being sent. zeroturn verify passes two checks, and zeroturn ship with dry run prints READY TO SHIP. The values are sample data.](docs/demo/zeroturn.svg)

> [!NOTE]
> The recording is real output from the executable, made with [docs/demo/record.sh](docs/demo/record.sh) against a sample project. The session values are sample data, not a real account. The last word of the status line says what happens to the next subagent. Here it is `ask`, so the harness asks you before starting another one, with a short reason such as "New subagent requires approval. Context is 82% and five hour usage is 81%."

<details>
<summary>Recording as text</summary>

```text
$ zeroturn status --stdin --harness claude < session.json
ZT  ctx 82%  5h 81%  7d 47%  session 3h12m  agents 2  ask
$ zeroturn policy check
ZEROTURN POLICY CHECK
mode       confirm
level      confirm
decision   ask
thresholds crossed:
  context          Context is 82% (limit 80)
  fiveHour         Five hour usage is 81% (limit 75)
  activeSubagents  2 subagents are active (limit 2)

# the model asks to read a file that holds a key
$ zeroturn event --event PreToolUse < read.json \
    | jq -r .hookSpecificOutput.permissionDecisionReason
Reading this file would put a credential into the conversation. deploy.env 
line 2 looks like aws access key id. Approving means the value is shared 
and should be rotated.
# the prompt guard is off by default, this session switched it on
$ zeroturn event --event UserPromptSubmit < message.json \
    | jq -r .reason
Your message was not sent. It contains what looks like aws access key id, 
and sending it would mean rotating the value. Remove it and send the 
message again. Turn this check off with zeroturn policy set 
guard.credentials.prompts off.

$ zeroturn verify
ZEROTURN VERIFY

PASS  vet        0.3s
PASS  test       0.3s

Result: 2 checks passed in 0.7s

$ zeroturn ship --message "docs: add release notes" --files NOTES.md --dry-run
ZEROTURN SHIP
files:
  NOTES.md
message:  docs: add release notes
branch:   docs-update
remote:   origin  ../remote.git

PASS  vet        0.3s
PASS  test       0.3s

Result: 2 checks passed in 0.6s

READY TO SHIP
Dry run finished. Nothing was staged, committed, or pushed.
```

</details>

## Why it exists

I kept reaching coding session limits during longer development sessions. The useful work was mixed with large context, repeated delegation, background activity, and routine validation and Git operations.

ZeroTurn was built to make that behavior easier to see and easier to control.

It shows the condition of the current session, asks before more delegated work when configured limits have been reached, and handles predictable development commands locally.

The coding tool should solve the problem. It should not spend the rest of the session narrating commands the computer already knows how to run. You should not need another agent to manage work that never needed an agent in the first place.

ZeroTurn does not promise to remove session limits, and it does not estimate savings.

## How it works

![Diagram in two lanes. Session Guard: the coding harness sends events from its status line and hooks to ZeroTurn, which checks your thresholds, counts subagents, and keeps local records, then returns a decision for the next subagent: allow, ask you, or deny. The harness applies it before the subagent starts. Direct Lane: you run zeroturn verify or zeroturn ship, ZeroTurn runs only approved commands with a credential scan and safe Git rules, and acts on your project's tests, lint, build, commit, and push, on your machine without a model turn.](docs/img/how-it-works.svg)

## Session Guard

**Session Guard reads what the harness already reports:** context use, the five hour and seven day usage windows, and session duration. It adds one thing the harness does not report, the number of subagents started and currently running, counted from the harness's own start and stop events.

Before a new subagent starts, the gate compares those values with your thresholds. Each measurement is checked against its own limit. Percentages from different measurements are never added together.

A value the harness did not send is left out. It is never shown as zero.

```text
ZT  ctx 82%  5h 81%  7d 47%  session 3h12m  agents 2  ask
     |        |       |       |             |         |
     |        |       |       |             |         what happens to the next subagent
     |        |       |       |             subagents running now, counted by ZeroTurn
     |        |       |       how long this session has been open
     |        |       seven day usage window
     |        five hour usage window
     context used in this conversation
```

The status line and the gate each take about 8 ms on the machine they were measured on, including process start, and neither makes a network request or starts a background process. The method and the numbers are in [docs/benchmarks.md](docs/benchmarks.md).

## Credential guard

![Two guards. Files the model reads, on by default: before a file is opened ZeroTurn scans it and the harness asks you, with modes ask, deny, and off. Messages you send, off by default: switch it on and a message holding a key is stopped before it is sent, which means ZeroTurn reads your messages in that repository in memory, and a stopped message is erased. Both checks run on your machine, nothing is stored, and the explanation names the file, the line, and the kind, never the value.](docs/img/credential-guard.svg)

Before the model reads a file, ZeroTurn scans that file. If it holds something shaped like a credential, the harness asks you first:

```text
Reading this file would put a credential into the conversation. deploy.env line 2 looks like
an aws access key id. Approving means the value is shared and should be rotated.
```

You decide. Approving reads the file as usual; declining means the value never leaves your machine, and there is nothing to rotate.

| Setting | Behavior |
| --- | --- |
| `ask` | The default. The harness asks before that file is read |
| `deny` | The read is refused |
| `off` | No scanning |

```sh
zeroturn policy set guard.credentials.mode deny
```

> [!IMPORTANT]
> This reads the file the model was about to open, and only that file. It never reads your prompts. The message names the file, the line, and the kind of credential, never the value.

> [!NOTE]
> It looks for high confidence patterns, so it catches a common mistake rather than every possible one. A file larger than 1 MB is skipped, because a credential lives in a small file and a larger one is almost always data, and a credential in an unusual format can pass. It is a guard, not a guarantee.

### Messages you send

A key you paste into a message is the other way one reaches the model. ZeroTurn can check for that too, and it is **off by default**, because switching it on changes what ZeroTurn reads.

```sh
zeroturn policy set guard.credentials.prompts block
```

The command explains both consequences and asks you to confirm. With it on, in that repository only:

- Every message you send passes through ZeroTurn first. It is read in memory, checked, and never stored, logged, or sent anywhere.
- A message carrying a credential is stopped before it leaves your machine, with an explanation that never repeats the value.

> [!WARNING]
> The harness erases a stopped message rather than handing it back, so a long message is lost. That is the cost of catching the key before it is sent.

With it off, ZeroTurn never reads what you write, and the hook that would do so is not even installed.

## Direct Lane

**`zeroturn verify`** runs the checks listed in `.zeroturn.json` directly, without a shell and without a model turn. It prints one line per step and keeps the full output under `.git/zeroturn/logs/`.

**`zeroturn ship`** stages only the files you name, scans them for credentials, runs the checks, fetches, refuses unsafe branch states, asks for confirmation, then commits with your message and pushes without force. `--dry-run` stops before anything changes.

## Installation

ZeroTurn is one executable with no runtime dependencies. Building it needs Go 1.17 or newer.

```sh
git clone https://github.com/cris-wendler/zeroturn.git
cd zeroturn
go build -o zeroturn ./cmd/zeroturn
```

Put the `zeroturn` executable somewhere on your `PATH`.

Release archives for macOS, Linux, and Windows are built by `scripts/build-release.sh` and attached to each release with their checksums. `go install` and a Homebrew formula follow the first public release. The steps are in [docs/release.md](docs/release.md).

## Five minute setup

From inside a Git repository:

```sh
zeroturn init                      # detect the project and propose .zeroturn.json
zeroturn integrate claude --plan   # show what would change, change nothing
zeroturn integrate claude --apply  # write it after you confirm
zeroturn doctor                    # check the installation
```

> [!TIP]
> Start a new coding session after `--apply`. The settings are read when a session starts, so a running session does not pick them up.

`init` proposes validation steps only for commands the project actually declares, and only when the executable is installed. It recognises Go, JavaScript and TypeScript through `package.json` scripts, Python with pytest, ruff, mypy, poetry and uv, Rust, Maven and Gradle, .NET, Ruby, and a `Makefile`, which it reads for the targets it declares rather than guessing. The file it writes looks like this:

```json
{
  "version": 1,
  "guard": {
    "mode": "observe",
    "context": { "warn": 70, "confirm": 80, "critical": 90 },
    "limits": { "fiveHourWarn": 75, "sevenDayWarn": 75 },
    "session": { "durationWarnMinutes": 240, "activeSubagentsWarn": 2, "subagentStartsWarn": 4 },
    "credentials": { "mode": "ask", "prompts": "off" }
  },
  "verify": {
    "steps": [
      { "name": "vet", "command": ["go", "vet", "./..."] },
      { "name": "test", "command": ["go", "test", "./..."] }
    ]
  },
  "git": { "remote": "origin", "protectedBranches": ["main", "master"] }
}
```

The thresholds are starting points. They were chosen for a first run, not measured, and you should adjust them to how you work.

## Policy modes

![Three modes side by side. observe, the default, in green: shows the session condition and counts subagents, never blocks or asks. confirm, in yellow: asks before a new subagent once a threshold is crossed, you decide each time. strict, in red: denies a new subagent at critical context, other thresholds ask, and you approve it first.](docs/img/modes.svg)

```sh
zeroturn policy show
zeroturn policy check                      # evaluate the current session, change nothing
zeroturn policy set guard.mode confirm
zeroturn policy set guard.context.confirm 85
zeroturn policy reset
```

> [!WARNING]
> Strict mode is never switched on by a file alone. `zeroturn policy set guard.mode strict` shows the exact policy, explains what can be blocked and how to turn it off, checks the installed harness, runs a compatibility test, and asks you to confirm. A repository that commits `"mode": "strict"` gets Confirm behavior on every machine where nobody has approved Strict.

## Privacy

![Two columns. Recorded, on your machine: session identifier, harness and version, context percent and window size, five hour and seven day usage, session duration, subagent starts, stops and active count, gate decisions and command counts, and a hash of the repository rather than its path. Never recorded: prompts and responses, source code and diffs, subagent instructions, transcript contents, credentials and environment values, repository paths, and remote URLs holding credentials.](docs/img/privacy.svg)

> [!IMPORTANT]
> The harness sends a transcript path with most events. ZeroTurn discards it and never opens the file. A test checks that no prompt, response, or path reaches its records.

Two things are read without being recorded, and only to look for credentials: the file the model is about to open, and, if you switch the prompt guard on, the message you are about to send. Both are scanned in memory and nothing from either is stored.

Records stay on your machine, in `~/Library/Application Support/zeroturn` on macOS, `$XDG_DATA_HOME/zeroturn` or `~/.local/share/zeroturn` on Linux, and `%LOCALAPPDATA%\zeroturn` on Windows. Records older than seven days are removed when a new session starts. `zeroturn report purge --all` removes every ZeroTurn record and nothing else.

ZeroTurn makes no network requests of its own, calls no model, and runs no background process. `zeroturn ship` contacts your Git remote because pushing requires it.

## Commands

**Session Guard**

| Command | Purpose |
| --- | --- |
| `zeroturn status` | The session condition, or repository status outside a session |
| `zeroturn policy show \| check \| set \| reset` | Read or change the guard thresholds |
| `zeroturn report current \| day \| week \| purge` | Summarise locally observed events, `--json` for machine output |
| `zeroturn integrate claude --plan \| --apply \| --remove` | Show, install, or remove the harness integration |

**Direct Lane**

| Command | Purpose |
| --- | --- |
| `zeroturn verify` | Run the approved validation steps, `--approve` to review them first |
| `zeroturn ship --message ... --files ...` | Stage named files, check, commit, and push, `--dry-run` to stop before any change |

**Setup and support**

| Command | Purpose |
| --- | --- |
| `zeroturn init` | Detect the project and write `.zeroturn.json` after confirmation |
| `zeroturn doctor` | Check the installation, including whether the hooks still point at this executable. `--compat` for guard decisions, `--compat --live` for a real session |
| `zeroturn capabilities --json` | Describe what this build supports |
| `zeroturn version` | Print the version |
| `zeroturn event` | The adapter entry point. Harness hooks call this, you do not |

> [!NOTE]
> Every report ends with the line "Based only on events observed locally by ZeroTurn on this machine." The counts are events, never tokens, cost, or a saving.

## Claude Code

Supported. The full guide, including what each hook does and which payload fields are read, is in [docs/integrations/claude-code.md](docs/integrations/claude-code.md). ZeroTurn uses these official interfaces:

- the status line, for context, usage windows, and session duration
- `PreToolUse` with the exact matcher `Agent`, the only hook that can allow, ask, or deny a subagent
- `PreToolUse` with the exact matcher `Read`, for the credential guard
- `UserPromptSubmit`, installed only if you switch the prompt guard on
- `SubagentStart` and `SubagentStop`, to count subagents
- `Stop`, to see whether background tasks are still running
- `SessionEnd`

`zeroturn integrate claude` writes to `.claude/settings.local.json` in the repository, the settings file that applies to you only, because the entries contain the path of your local executable. `--user` writes to your user settings instead, and only after the same plan and confirmation. `--remove` deletes ZeroTurn's own entries and leaves everything else as it was.

Tested on Claude Code 2.1.265. A denial was honoured in a real session and no subagent started. An allow let the subagent start. The ask decision is accepted by the harness, but the interactive approval prompt has not yet been observed, so Confirm mode is not yet described as tested end to end. Run `zeroturn doctor --compat --live` to repeat the denial test on your machine. It starts one short session and spends a small amount of usage.

## GitHub Copilot CLI

Not supported in this version. No Copilot hook payload carries usage, context utilisation, or session duration, so Session Guard has nothing to show there. Custom commands need an interface that GitHub labels experimental. An adapter is planned once those values are exposed.

## Harness contract

Other harnesses can send events in a normalized JSON form with `zeroturn event --harness normalized`. Each event names the contract `zeroturn.event/1`, and an event that names a different contract is refused with a clear message. `zeroturn capabilities --json` lists the commands, event types, recorded fields, and exit codes.

[docs/harness-contract.md](docs/harness-contract.md) states what ZeroTurn promises an adapter and what an adapter must do in return: process invocation, event shapes, decisions, mutation classification, exit codes, cancellation, redaction, and how versions change. [docs/adapter-authoring.md](docs/adapter-authoring.md) is the practical guide, and [integrations/template/](integrations/template) holds a worked example you can copy.

## Safety

![Two columns. ship always: stages only the files you name, scans them for credentials, runs your approved checks, reads the branch state first, asks before the first change, and pushes without force. ship refuses: a protected branch, a path outside the repository, files staged that you did not name, a branch behind or diverged, force push, rebase, reset, branch delete, and skipping your Git hooks.](docs/img/safety.svg)

- Validation commands are argument arrays in `.zeroturn.json`, never shell strings. A repository's commands do not run until you approve them on your machine, and changing any command withdraws the approval.
- Output from validation steps is scanned for credentials before it is printed or logged.
- The gate fails open. If ZeroTurn cannot read an event or its state, the harness behaves as if ZeroTurn were not installed.
- Every error states what stopped, why, and the smallest safe next step.

| Exit code | Meaning |
| --- | --- |
| 0 | success |
| 1 | validation or policy failure |
| 2 | invalid invocation or configuration |
| 3 | unsafe Git state |
| 4 | required executable unavailable |
| 5 | internal failure |
| 6 | confirmation declined |
| 7 | incompatible contract version |
| 8 | integration unavailable |

## JSON schemas

`status`, `policy show`, `policy check`, `report`, `verify`, `doctor`, and `capabilities` accept `--json`. The schemas are published in [schemas/](schemas), including the repository configuration file and the normalized event an adapter sends.

```sh
go test ./conformance                                                      # every output still follows its schema
go run ./conformance/validate schemas/normalized-event.schema.json e.json  # check one document
```

The validator is part of the project and has no dependencies. A schema using a keyword it does not support is reported rather than skipped, so a contract can never look checked when it is not.

## Contributing

Useful areas: harness adapters, event normalization, project detection, validation presets, platform support, security and redaction tests, terminal rendering, documentation, and conformance fixtures.

Contributions that add any of the following will be redirected: a model SDK or model calls, model selection, a chat interface, a complete coding harness, transcript collection, prompt inspection, remote telemetry, a hosted dashboard, output compression, automatic handoff, automatic compaction or clearing, shell strings in configuration, automatic force pushing or rebasing, credential bypasses, silent changes to global settings, Python packaging, or containers without a demonstrated need.

Before contributing, check that the feature is not already there and search the existing issues. Then open a bug report or feature proposal on GitHub, and a pull request once the approach is agreed. The steps, and how to build and test, are in [CONTRIBUTING.md](CONTRIBUTING.md). Security problems go through [SECURITY.md](SECURITY.md), not a public issue. The tests use temporary repositories and local bare remotes and never contact a real remote.

## Limitations

> [!CAUTION]
> ZeroTurn shows and gates what the harness reports. It cannot see usage the harness does not send, and it does not promise to remove session limits.

- Session values appear only when the harness sends them. Some accounts receive no usage window data.
- Subagent counts cover only subagents started while ZeroTurn was installed. One that never reports stopping is cleared when the turn ends.
- The background task count is as current as the last `Stop` event.
- The interactive approval prompt for Confirm mode has not yet been observed.
- The test suite runs on Linux, macOS, and Windows in continuous integration. On Windows, tests that need a POSIX shell are skipped. The harness integration has been used on macOS only.
- The status line has been checked in the terminal interface of the harness only.

## Roadmap

Current: everything described above, including both credential guards, plus contributor documents, the harness contract with published schemas and a conformance suite, and continuous integration on Linux, macOS, and Windows. A run against this repository, showing each behavior with real output, is in [docs/dogfood.md](docs/dogfood.md).

Planned:

- `go install` and a Homebrew formula after the first public release, following [docs/going-public.md](docs/going-public.md)
- a Copilot adapter once Copilot exposes session values to hooks

Not planned: `zeroturn sync`. The reasoning is in [docs/decisions.md](docs/decisions.md). The research behind the product boundary is in [docs/product-boundary.md](docs/product-boundary.md).

## How this was built

The research came before the code, the differentiator was tested before the rest was written, and the constraints were kept rather than worked around. [docs/how-this-was-built.md](docs/how-this-was-built.md) is the record: what was examined and rejected, what would have stopped the project, the defects its own tests found, and what is still unproven.

## License

ZeroTurn is free software licensed under GNU GPL version 3. You may use, study, modify, and distribute it under the terms of that license.

![GPL version 3 in plain words. You may: run it for any purpose including at work, read and change the source, share copies changed or not, charge money for doing so, and keep your private changes private. If you share it you must: keep the license and the notices, say what you changed, give people the source code, use this same license for it, and expect no warranty. This is a summary for orientation, not the license. The text in LICENSE is what governs.](docs/img/license.svg)

| | |
| --- | --- |
| SPDX identifier | `GPL-3.0-only` |
| Full text | [LICENSE](LICENSE). [COPYING](COPYING), the traditional GNU name, points to it |
| Contributions | accepted under the same license, with no contributor license agreement |
| Dependencies | none, see [docs/dependency-licenses.md](docs/dependency-licenses.md) |

> [!NOTE]
> The card is a summary, not the license, and it grants nothing. The text in `LICENSE` is the Free Software Foundation's, unmodified, because the license itself forbids changing that document and because tools identify the license by matching it exactly.
