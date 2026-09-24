# ZeroTurn

[![Tests on Linux, macOS, and Windows](https://github.com/cris-wendler/zeroturn/actions/workflows/ci.yml/badge.svg)](https://github.com/cris-wendler/zeroturn/actions/workflows/ci.yml)
[![Go 1.17+](https://img.shields.io/badge/go-1.17%2B-00ADD8?logo=go&logoColor=white)](go.mod)
![No dependencies](https://img.shields.io/badge/dependencies-none-success)
[![License GPL-3.0-only](https://img.shields.io/badge/license-GPL--3.0--only-blue)](LICENSE)

Your tests passed. Then the code changed. ZeroTurn records which state of the code the checks passed for, and tells you when that is no longer the code on disk.

It is for developers working with a coding agent, where files keep moving between the moment the tests pass and the moment somebody reads the diff.

![Three steps. First, zeroturn verify passes and records evidence for repository state cab46d1b8898. Second, the code changes, for example an agent edits a file. Third, zeroturn report current says the validation is stale, because the checks passed for code that is no longer there.](docs/img/stale-evidence.svg)

One executable. No dependencies, no network, no model calls. It never reads your prompts, your messages, or your code.

## What it does

![Diagram in two lanes. Session Guard: the coding harness sends events from its status line and hooks to ZeroTurn, which checks your thresholds, counts subagents, and keeps local records, then returns a decision for the next subagent: allow, ask you, or deny. The harness applies it before the subagent starts. Direct Lane: you run zeroturn verify or zeroturn ship, ZeroTurn runs only approved commands with a credential scan and safe Git rules, and acts on your project's tests, lint, build, commit, and push, on your machine without a model turn.](docs/img/how-it-works.svg)

| Part | What you get | Read more |
| --- | --- | --- |
| **Validation evidence** | `verify` runs your checks and records the state of the code they passed for. `report current` says when that state is gone | [docs/validation-evidence.md](docs/validation-evidence.md) |
| **Session Guard** | A status line with context, usage windows, session time and running subagents, and a gate that can ask before another subagent starts | [docs/session-guard.md](docs/session-guard.md) |
| **Credential guard** | Before the model reads a file that holds a key, the harness asks you first | [docs/credential-guard.md](docs/credential-guard.md) |
| **Ship** | `ship` stages only the files you name, scans them, runs the checks, and pushes without force. `--dry-run` changes nothing | `zeroturn ship --help` |

## Install

**With Go 1.17 or newer:**

```sh
go install github.com/cris-wendler/zeroturn/cmd/zeroturn@latest
```

If `zeroturn` is not found afterwards, your Go bin directory is not on your `PATH`. Add `export PATH="$(go env GOPATH)/bin:$PATH"` to your shell profile.

**Without Go:** take an archive for macOS, Linux, or Windows from the [latest release](https://github.com/cris-wendler/zeroturn/releases/latest), check it against the `SHA256SUMS` beside it, and put the executable on your `PATH`. There is no Homebrew tap yet.

## Set it up

From inside a Git repository:

```sh
zeroturn init                      # detect the project and propose .zeroturn.json
zeroturn integrate claude --plan   # show what would change, change nothing
zeroturn integrate claude --apply  # write it after you confirm
zeroturn doctor                    # check the installation
```

`doctor` is the command to run whenever something looks wrong. It checks the install, the `PATH`, the harness entries, and whether the guard is receiving measurements.

Start a new coding session after `--apply`. This writes to `.claude/settings.local.json` in the repository. `--user` writes to your user settings, and `--remove` deletes ZeroTurn's own entries and leaves everything else alone. The full guide is [docs/integrations/claude-code.md](docs/integrations/claude-code.md).

### Or as a Claude Code plugin

```
/plugin marketplace add cris-wendler/zeroturn
/plugin install zeroturn@zeroturn
```

The plugin installs the hooks only. It carries no status line and no executable, so `go install` first, and use `integrate` if you want the measurements. See [integrations/claude-plugin/](integrations/claude-plugin).

### The configuration file

`init` proposes validation steps only for commands the project declares, and only when the executable is installed. It recognises Go, JavaScript and TypeScript, Python, Rust, Maven and Gradle, .NET, Ruby, and `Makefile` targets.

<details>
<summary>What .zeroturn.json looks like</summary>

The file it writes looks like this:

```json
{
  "version": 1,
  "guard": {
    "mode": "observe",
    "context": { "warn": 70, "confirm": 80, "critical": 90 },
    "limits": { "fiveHourWarn": 75, "sevenDayWarn": 75, "projection": "on" },
    "session": { "durationWarnMinutes": 240, "activeSubagentsWarn": 2, "subagentStartsWarn": 4 },
    "credentials": { "mode": "ask", "prompts": "off" }
  },
  "report": { "retentionDays": 7 },
  "verify": {
    "steps": [
      { "name": "vet", "command": ["go", "vet", "./..."] },
      { "name": "test", "command": ["go", "test", "./..."] }
    ]
  },
  "git": { "remote": "origin", "protectedBranches": ["main", "master"] }
}
```

The thresholds are starting points, not measurements. Adjust them to how you work.

</details>

## Use it

```sh
zeroturn verify --approve   # the first time: review the commands, then run them
zeroturn verify             # every time after that
zeroturn report current     # say whether that answer still covers the code
```

The first run asks. A repository's validation commands are whatever that repository put in `.zeroturn.json`, and they run directly on your machine, so `verify` prints them and waits for you to agree once. Changing any of them asks again. Without that approval `verify` runs nothing and says so.

`verify` then prints one line per step and ends with the state the run answered for. `report current` compares that state with the code on disk and says `STALE` when they differ.

![Terminal recording. The ZeroTurn status line shows context at 82 percent, five hour usage at 81 percent, seven day usage at 47 percent, a session of 3 hours 12 minutes, 2 active subagents, and the word ask. zeroturn policy check shows the decision ask because context, five hour usage, and active subagents are past their thresholds. The credential guard then stops a file that holds an aws access key id from being read, and, with the prompt guard switched on, stops a message carrying the same key from being sent. zeroturn verify passes two checks and records evidence for the repository state it ran against, and zeroturn ship with dry run prints READY TO SHIP. The values are sample data.](docs/demo/zeroturn.svg)

Real output from the executable, recorded with [docs/demo/record.sh](docs/demo/record.sh) against a sample project. The session values are sample data.

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

PASS  vet        0.1s
PASS  test       0.1s

Result: 2 checks passed in 0.1s
Evidence 19d72ae794c5e681 recorded for repository state 54a926e9cc78

$ zeroturn ship --message "docs: add release notes" --files NOTES.md --dry-run
ZEROTURN SHIP
files:
  NOTES.md
message:  docs: add release notes
branch:   docs-update
remote:   origin  ../remote.git

PASS  vet        0.1s
PASS  test       0.1s

Result: 2 checks passed in 0.1s

READY TO SHIP
Dry run finished. Nothing was staged, committed, or pushed.
```

</details>

## Where each part works

Session Guard's measurements arrive through the harness status line and through nothing else. An editor extension draws no status line, and a plugin cannot install one.

| | Terminal, with `integrate` | Editor extension | Plugin only |
| --- | --- | --- | --- |
| Validation evidence, `verify`, `ship` | yes | yes | yes |
| Credential guard | yes | yes | yes |
| Subagent counts and the gate on them | yes | yes | yes |
| Context, usage windows, session time | yes | no | no |

`zeroturn doctor` and `zeroturn policy check` both say which case you are in, and name the values that were not measured. They do not print an all clear for a threshold nothing could check.

## Validation evidence

```text
$ zeroturn verify
PASS  vet        0.4s
PASS  test     102.2s
Evidence 91ef35e32bc212a8 recorded for repository state cab46d1b8898

$ vim internal/policy/policy.go

$ zeroturn report current
Validation  STALE
  Validation is stale because the working tree changed after the last successful run.
```

The state is a digest of the commit, everything staged, every file that differs from Git, and the validation steps themselves. It is taken over file content, so editing a file that was already modified still counts.

It says the checks passed for that code, on your machine, at that moment. It does not say the code is correct. The digest does not cover:

- Files Git ignores. A change to an ignored build input does not move the digest.
- Contents inside a submodule. Git reports that a submodule changed, and that is recorded, but the files within it are not hashed.
- Anything outside the repository: installed dependencies, environment variables, toolchain versions, services the tests reach.
- A change that was made and then undone. The digest returns to its earlier value, because the code did too.

### Recorded without being asked

`verify` records evidence because it ran the steps itself. Steps are also recorded when a coding agent happens to run them: ZeroTurn compares each command against the plan, and a step that passes is written down.

This watches what the agent runs, and only that. It is a harness hook, so it sees the commands a coding session makes through its own tools. A command you type in your terminal never reaches it, because the harness was not involved. That is the case it is for: the evidence here went stale across fifteen merged changes because an agent ran the tests directly and nobody ran `verify` afterwards.

Installing it needs a new coding session. Settings are read when a session starts, so `integrate` says so and a session already open does not pick it up.

A record says `passed` only when every configured step has passed against the same state of the code. Until then it says `partial` and names what is outstanding. A narrower run, `go test ./internal/policy` where the step is `go test ./...`, records nothing, because it did less work than the step claims.

More in [docs/validation-evidence.md](docs/validation-evidence.md).

### Why not just git

Git tells you what changed. It does not record when your checks passed, so on its own it cannot say whether they covered the code in front of you.

| What you might reach for | Why it does not answer |
| --- | --- |
| `git rev-parse HEAD` | Does not move when you edit a file, and the working tree is where an agent does its work |
| `git status --porcelain` | Prints the same line for the first and the second edit of a file, so a digest built on it reports new code as the code that was tested |
| `git stash create` | Comes closest, and does not see a new file at all |
| `git blame` | Answers who last touched a line, which is a different question |

Writing down the moment the checks passed is the part Git does not have, and it is most of what `zeroturn verify` does.

## Session Guard

```text
ZT  ctx 82%  5h 81%  7d 47%  session 3h12m  agents 2  ask
```

The last word says what happens to the next subagent. With `ask`, the harness asks you first and shows a reason such as "New subagent requires approval. Context is 82% and five hour usage is 81%."

![Three modes side by side. observe, the default, in green: shows the session condition and counts subagents, never blocks or asks. confirm, in yellow: asks before a new subagent once a threshold is crossed, you decide each time. strict, in red: denies a new subagent at critical context, other thresholds ask, and you approve it first.](docs/img/modes.svg)

```sh
zeroturn policy set guard.mode confirm   # observe is the default
zeroturn policy check                    # what would the gate do right now
```

The gate also looks at how fast the five hour window is being used, and `zeroturn policy tune` proposes thresholds from what you actually approved. Both are in [docs/session-guard.md](docs/session-guard.md).

## Credential guard

![Two guards. Files the model reads, on by default: before a file is opened ZeroTurn scans it and the harness asks you, with modes ask, deny, and off. Messages you send, off by default: switch it on and a message holding a key is stopped before it is sent, which means ZeroTurn reads your messages in that repository in memory, and a stopped message is erased. Both checks run on your machine, nothing is stored, and the explanation names the file, the line, and the kind, never the value.](docs/img/credential-guard.svg)

On by default for files, in `ask` mode. Off by default for messages you send, because switching it on means ZeroTurn reads them. It is a guard, not a guarantee: it looks for high confidence patterns and skips files over 1 MB. Details in [docs/credential-guard.md](docs/credential-guard.md).

## Commands

**Session Guard**

| Command | Purpose |
| --- | --- |
| `zeroturn status` | The session condition, or repository status outside a session |
| `zeroturn policy show \| check \| set \| reset \| tune \| migrate` | Read or change the guard thresholds |
| `zeroturn report current \| day \| week \| month \| purge` | Summarise events observed in this repository and whether its validation still covers the current code, `--all-repositories` for the machine, `--json` for machine output |
| `zeroturn integrate claude --plan \| --apply \| --remove` | Show, install, or remove the harness integration |

**Direct Lane**

| Command | Purpose |
| --- | --- |
| `zeroturn verify` | Run the approved validation steps and record what state they ran against, `--approve` to review them first |
| `zeroturn ship --message ... --files ...` | Stage named files, check, commit, and push, `--dry-run` to stop before any change |

**Setup and support**

| Command | Purpose |
| --- | --- |
| `zeroturn init` | Detect the project and write `.zeroturn.json` after confirmation |
| `zeroturn doctor` | Check the installation, including whether the hooks still point at this executable. `--compat` for guard decisions, `--compat --live` for a real session |
| `zeroturn uninstall` | List everything ZeroTurn put on this machine, `--apply` to remove it |
| `zeroturn capabilities --json` | Describe what this build supports |
| `zeroturn version` | Print the version |
| `zeroturn event` | The adapter entry point. Harness hooks call this, you do not |

Every command has `--help`. `status`, `policy show`, `policy check`, `report`, `verify`, `doctor`, and `capabilities` accept `--json`, and the schemas are in [schemas/](schemas).

<details>
<summary>Exit codes</summary>

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

</details>

## Privacy and safety

- Nothing leaves your machine. No network requests, no model calls, no background process. `zeroturn ship` contacts your Git remote because pushing requires it.
- The harness sends a transcript path with most events. ZeroTurn discards it and never opens the file, and a test checks that no prompt, response, or path reaches its records.
- Commands a session runs are read, and not kept. Each one is compared with the validation steps in `.zeroturn.json` so that a step passing can be recorded without anybody remembering to run `verify`. What reaches a record is the name of the step it matched, or nothing. The command itself is never stored, logged, or sent.
- The credential guard scans in memory and stores nothing from what it scans.
- Validation evidence stores digests, step names and results. It stores no file contents, no file paths and no step output.
- Validation commands are argument arrays, never shell strings. They do not run until you approve them on your machine, and changing a command withdraws the approval.
- The gate fails open. If ZeroTurn cannot read an event or its state, the harness behaves as if ZeroTurn were not installed.
- Session records are removed after `report.retentionDays`, seven by default. `zeroturn report purge --all` removes every record, and `zeroturn uninstall` lists and removes everything ZeroTurn put on the machine.

Records live in `~/Library/Application Support/zeroturn` on macOS, `$XDG_DATA_HOME/zeroturn` or `~/.local/share/zeroturn` on Linux, and `%LOCALAPPDATA%\zeroturn` on Windows.

## Harnesses

Claude Code is supported, tested on 2.1.265, 2.1.270 and 2.1.277. Allow, ask and deny were each observed in a real session. Run `zeroturn doctor --compat --live` to repeat the denial test on your machine. It starts one short session and spends a small amount of usage.

GitHub Copilot CLI is not supported yet. No adapter ships, and one will be called supported only after it has run against a real session. The notes are in [docs/decisions.md](docs/decisions.md).

Any other harness can send events as normalized JSON with `zeroturn event --harness normalized`. The contract is the [schemas/](schemas) plus the suite in [conformance/](conformance), and [integrations/template/](integrations/template) is a worked example to copy.

## Limitations

- Session values appear only when the harness sends them. Some accounts receive no usage window data.
- The approval prompt can be switched off from inside itself: the harness offers to stop asking for that tool in that directory, and ZeroTurn is not told when that is chosen.
- Subagent counts cover only subagents started while ZeroTurn was installed.
- Validation evidence has not been used long enough to say how often it goes stale in real work, or whether the warning changes what anybody does.
- The test suite runs on Linux, macOS and Windows for every change. The harness integration has been used on macOS only.

## Roadmap

- A published Homebrew tap. The formula is generated already.
- A Copilot adapter.
- Being explored, and not built: a repository handoff record that an engineer can review after an agent has worked, holding the repository state, what was done, the validation evidence, and what still needs a person. This is a file you ask for and read. It is not an automatic session handoff.

Not planned: `zeroturn sync`. The reasoning is in [docs/decisions.md](docs/decisions.md).

## Contributing

Start with [CONTRIBUTING.md](CONTRIBUTING.md). Security problems go through [SECURITY.md](SECURITY.md), not a public issue.

Useful areas: harness adapters, project detection, validation presets, platform support, security and redaction tests, and documentation. Changes that add model calls, a chat interface, transcript collection, prompt inspection, remote telemetry, a hosted dashboard, or shell strings in configuration will be redirected. CONTRIBUTING has the full list.

## How this was built

The research came before the code, and the project keeps a record of what was rejected, what its own tests found, and what is still unproven: [docs/how-this-was-built.md](docs/how-this-was-built.md).

The finding that outlived its feature: six defects here had the same shape, a description kept by hand beside the thing it described, drifting away from it. The tests now derive those descriptions from the code. Every drift with such a check was caught automatically, and every drift without one was caught by luck. That is entry 33 in [docs/decisions.md](docs/decisions.md).

## License

`GPL-3.0-only`. The full text is in [LICENSE](LICENSE), and [COPYING](COPYING) is the traditional GNU name for the same file. Contributions are accepted under the same license, with no contributor license agreement.

