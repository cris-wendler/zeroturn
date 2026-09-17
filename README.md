# ZeroTurn

[![Tests on Linux, macOS, and Windows](https://github.com/cris-wendler/zeroturn/actions/workflows/ci.yml/badge.svg)](https://github.com/cris-wendler/zeroturn/actions/workflows/ci.yml)
[![Go 1.17+](https://img.shields.io/badge/go-1.17%2B-00ADD8?logo=go&logoColor=white)](go.mod)
![No dependencies](https://img.shields.io/badge/dependencies-none-success)
[![License GPL-3.0-only](https://img.shields.io/badge/license-GPL--3.0--only-blue)](LICENSE)

Know what your validation actually covered. ZeroTurn records the state your code was in when the checks passed, and says when that answer no longer applies to the code on disk.

It is for developers working with a coding agent, where the working tree can move between the moment the tests passed and the moment somebody reads the diff. A count of passing runs does not say which code passed.

ZeroTurn also shows context, usage windows, session duration, and subagent activity when the coding harness supplies them, and can ask for approval before further delegation. That half reads the harness status line, so it needs a terminal. The validation half does not, and works anywhere.

ZeroTurn works with coding harnesses. It is not another coding harness. One executable, no dependencies, no network, no model calls, and it never reads your prompts, your messages, or your code.

## Start here

```sh
go install github.com/cris-wendler/zeroturn/cmd/zeroturn@latest
```

Then, from inside a Git repository:

```sh
zeroturn init            # detect the project and propose .zeroturn.json
zeroturn verify          # run the checks, and record what state they ran against
zeroturn report current  # say whether that answer still covers the code
```

`go install` puts the executable in your `GOBIN`, which is not always on your `PATH`. `zeroturn doctor` checks that and names the line to add. The full instructions, including connecting it to a coding harness, are under [Installation](#installation).

![Terminal recording. The ZeroTurn status line shows context at 82 percent, five hour usage at 81 percent, seven day usage at 47 percent, a session of 3 hours 12 minutes, 2 active subagents, and the word ask. zeroturn policy check shows the decision ask because context, five hour usage, and active subagents are past their thresholds. The credential guard then stops a file that holds an aws access key id from being read, and, with the prompt guard switched on, stops a message carrying the same key from being sent. zeroturn verify passes two checks and records evidence for the repository state it ran against, and zeroturn ship with dry run prints READY TO SHIP. The values are sample data.](docs/demo/zeroturn.svg)

The recording is real output from the executable, made with [docs/demo/record.sh](docs/demo/record.sh) against a sample project. The session values are sample data, not a real account. The last word of the status line says what happens to the next subagent. Here it is `ask`, so the harness asks you before starting another one, with a short reason such as "New subagent requires approval. Context is 82% and five hour usage is 81%."

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

## Why it exists

I kept reaching coding session limits during longer development sessions. The useful work was mixed with large context, repeated delegation, background activity, and routine validation and Git operations.

ZeroTurn was built to make that behavior easier to see and easier to control.

It shows the condition of the current session, asks before more delegated work when configured limits have been reached, and handles predictable development commands locally.

The coding tool should solve the problem. It should not spend the rest of the session narrating commands the computer already knows how to run. You should not need another agent to manage work that never needed an agent in the first place.

ZeroTurn does not promise to remove session limits, and it does not estimate savings.

## How it works

Two lanes, and they are independent. The Direct Lane runs your own
commands on your machine, with no model turn involved at all, and
records what state the code was in when they passed. Session Guard
watches the session and answers the harness when it asks whether a
subagent may start. The Direct Lane works anywhere; Session Guard needs
a harness that draws a status line, which means a terminal.

![Diagram in two lanes. Session Guard: the coding harness sends events from its status line and hooks to ZeroTurn, which checks your thresholds, counts subagents, and keeps local records, then returns a decision for the next subagent: allow, ask you, or deny. The harness applies it before the subagent starts. Direct Lane: you run zeroturn verify or zeroturn ship, ZeroTurn runs only approved commands with a credential scan and safe Git rules, and acts on your project's tests, lint, build, commit, and push, on your machine without a model turn.](docs/img/how-it-works.svg)

## Direct Lane

`zeroturn verify` runs the checks listed in `.zeroturn.json` directly, without a shell and without a model turn. It prints one line per step and keeps the full output under `.git/zeroturn/logs/`.

`zeroturn ship` stages only the files you name, scans them for credentials, runs the checks, fetches, refuses unsafe branch states, asks for confirmation, then commits with your message and pushes without force. `--dry-run` stops before anything changes.

## Validation evidence

A passing run answers a question about a particular state of the code. `zeroturn verify` writes down which state that was, so a later command can say whether the answer still applies.

```text
$ zeroturn verify
ZEROTURN VERIFY

PASS  vet        0.4s
PASS  test     102.2s

Result: 2 checks passed in 102.5s
Evidence 91ef35e32bc212a8 recorded for repository state cab46d1b8898

$ vim internal/policy/policy.go

$ zeroturn report current
...
Validation  STALE
  Validation is stale because the working tree changed after the last successful run. Run zeroturn verify again for the current code.
  Evidence 91ef35e32bc212a8 recorded 2026-09-16 19:54 for repository state cab46d1b8898
```

**What is compared.** The repository state is a digest over the commit, the content Git holds for everything staged, the content of every path the working tree disagrees with Git about, and the definition of the validation steps themselves. Editing a step is a change of state like any other, so evidence recorded under different checks does not carry over.

The digest is taken over content rather than over the output of `git status`. That output names paths and the category each one is in, and it does not move when you edit a file that was already modified. A digest built from it would report the second version of the code as the state the first version was tested against.

**What it does not prove.** That the checks passed for that code on your machine, at that moment. Nothing more. It is not a claim that the code is correct, that it was reviewed, or that it will pass anywhere else. The digest does not cover:

- Files Git ignores. A change to an ignored build input does not move the digest.
- Contents inside a submodule. Git reports that a submodule changed, and that is recorded, but the files within it are not hashed.
- Anything outside the repository: installed dependencies, environment variables, toolchain versions, services the tests reach.
- A change that was made and then undone. The digest returns to its earlier value, because the code did too.

**When it is checked.** When you ask: `zeroturn verify`, and `zeroturn report` for the repository you are in. There is no background process and no notification. The status line does not compute it, because the status line repaints constantly and reading the repository there would cost more than the whole repaint budget.

`zeroturn ship` runs the same checks before it commits, and does not record evidence for them. Shipping is a decision about code you are sending somewhere, and what it should record is a question this has not answered yet. Until it does, only `zeroturn verify` writes evidence.

**What is stored.** Step names, step statuses, exit codes, counts, times, the log file names, and the digests. One record per repository, in ZeroTurn's own state directory rather than in your project. Step output is not stored, because output carries whatever the tool printed; file contents are not stored, because the digest stands in for them; and the paths of your changed files are not stored either. `zeroturn uninstall` removes it with everything else, and so does `zeroturn report purge --all`.

## Session Guard

Session Guard reads what the harness already reports: context use, the five hour and seven day usage windows, and session duration. It adds one thing the harness does not report, the number of subagents started and currently running, counted from the harness's own start and stop events.

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

The status line and the gate each take about 8 ms on the machine they were measured on, including process start, and neither makes a network request or starts a background process. Measured over 50 runs on an Apple M4: status line 7.5 ms, gate 7.7 ms, startup 6.3 ms, binary 2.5 MB.

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

This reads the file the model was about to open, and only that file. It never reads your prompts. The message names the file, the line, and the kind of credential, never the value.

It looks for high confidence patterns, so it catches a common mistake rather than every possible one. A file larger than 1 MB is skipped, because a credential lives in a small file and a larger one is almost always data, and a credential in an unusual format can pass. It is a guard, not a guarantee.

### Messages you send

A key you paste into a message is the other way one reaches the model. ZeroTurn can check for that too, and it is off by default, because switching it on changes what ZeroTurn reads.

```sh
zeroturn policy set guard.credentials.prompts block
```

The command explains both consequences and asks you to confirm. With it on, in that repository only:

- Every message you send passes through ZeroTurn first. It is read in memory, checked, and never stored, logged, or sent anywhere.
- A message carrying a credential is stopped before it leaves your machine, with an explanation that never repeats the value.

> [!WARNING]
> The harness erases a stopped message rather than handing it back, so a long message is lost. That is the cost of catching the key before it is sent.

With it off, ZeroTurn never reads what you write, and the hook that would do so is not even installed.

## Installation

> [!IMPORTANT]
> **Session Guard needs Claude Code running in a terminal.** Context use, the usage windows, and session duration reach ZeroTurn through the harness status line, and through nothing else: no hook payload carries them. An editor extension draws no status line, so it never runs the command, and the guard then has no measurements at all. Subagent counts and the credential guard still work there, because those come from hooks. `zeroturn doctor` reports which case you are in.

ZeroTurn is one executable with no runtime dependencies. Installing it needs Go 1.17 or newer.

```sh
go install github.com/cris-wendler/zeroturn/cmd/zeroturn@latest
```

That puts `zeroturn` in your `GOBIN`, or in `$(go env GOPATH)/bin` if that is not set. That directory is not always on your `PATH`, and when it is not, every command below is reported as not found. Add it in your shell profile:

```sh
export PATH="$(go env GOPATH)/bin:$PATH"
```

`zeroturn doctor` checks this and names the line to add. To build from a checkout instead:

```sh
git clone https://github.com/cris-wendler/zeroturn.git
cd zeroturn
go build -o zeroturn ./cmd/zeroturn
```

Release archives for macOS, Linux, and Windows are built by `scripts/build-release.sh` and attached to each release with their checksums. `scripts/homebrew-formula.sh` generates a Homebrew formula from a release, and no tap is published yet, so `brew install` is not an option today.

### Setting it up

From inside a Git repository:

```sh
zeroturn init                      # detect the project and propose .zeroturn.json
zeroturn integrate claude --plan   # show what would change, change nothing
zeroturn integrate claude --apply  # write it after you confirm
zeroturn doctor                    # check the installation
```

Start a new coding session after `--apply`. The settings are read when a session starts, so a running session does not pick them up.

`init` proposes validation steps only for commands the project actually declares, and only when the executable is installed. It recognises Go, JavaScript and TypeScript through `package.json` scripts, Python with pytest, ruff, mypy, poetry and uv, Rust, Maven and Gradle, .NET, Ruby, and a `Makefile`, which it reads for the targets it declares rather than guessing. The file it writes looks like this:

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

### Rate, not only level

A percentage on its own is a weak question. Seventy five percent of the five hour window is fine when the window resets in ten minutes and a problem when it resets in four hours, and a threshold cannot tell those apart. So the gate also measures the rate.

ZeroTurn keeps the first five hour reading of the window that is running now, and from it works out how fast the window is being used and where that lands when it resets:

```text
New subagent requires approval. Five hour usage is 41% and rising 19% an hour, with 3h20m left before it resets.
```

Forty one percent crosses no threshold. The trajectory does. A session that is using the window heavily but will still finish inside it says nothing at all.

The estimate is treated as an estimate. It never denies, even in Strict mode. It is ignored when the rate has been measured over less than fifteen minutes, when usage is not rising, when the window has already reset, and when the reading belongs to a window that has rolled over. Turn it off with `zeroturn policy set guard.limits.projection off`, which restores threshold only behavior.

### Thresholds from what actually happened

The defaults are starting points, not measurements. `zeroturn policy tune` reads what the gate asked and what you did next, and proposes thresholds from that.

```text
$ zeroturn policy tune
ZEROTURN POLICY TUNE
Observations from the last 7 days in this repository.

sessions observed      4
asked                  9
approved after asking  8
denied                 0

fiveHour  (guard.limits.fiveHourWarn, now 75)
  all 6 asks were approved, the highest at 81
  zeroturn policy set guard.limits.fiveHourWarn 82
```

Approval is inferred from what the harness reports: a subagent starting after an ask means you approved it. A threshold you always approve is asking too early. One you often decline is doing its job, and the command says so instead of suggesting a change. It needs five observations before it suggests anything, and it never changes a setting itself.

Strict mode is never switched on by a file alone. `zeroturn policy set guard.mode strict` shows the exact policy, explains what can be blocked and how to turn it off, checks the installed harness, runs a compatibility test, and asks you to confirm. A repository that commits `"mode": "strict"` gets Confirm behavior on every machine where nobody has approved Strict.

## Privacy

> [!IMPORTANT]
> The harness sends a transcript path with most events. ZeroTurn discards it and never opens the file. A test checks that no prompt, response, or path reaches its records.

Two things are read without being recorded, and only to look for credentials: the file the model is about to open, and, if you switch the prompt guard on, the message you are about to send. Both are scanned in memory and nothing from either is stored.

Validation evidence is read from the repository and reduced to a digest before anything is written. The contents that go into that digest are never stored, and neither are the paths of the files they came from.

Records stay on your machine, in `~/Library/Application Support/zeroturn` on macOS, `$XDG_DATA_HOME/zeroturn` or `~/.local/share/zeroturn` on Linux, and `%LOCALAPPDATA%\zeroturn` on Windows. Session records older than `report.retentionDays` in `.zeroturn.json`, seven days by default, are removed when a new session starts. Change it with `zeroturn policy set report.retentionDays 30`. Validation evidence is not aged out with them: there is one record per repository and it is replaced by the next run, because deleting it would report a repository as never validated when it had been. `zeroturn report purge --all` removes every ZeroTurn record, evidence included, and nothing else.

ZeroTurn makes no network requests of its own, calls no model, and runs no background process. `zeroturn ship` contacts your Git remote because pushing requires it.

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

Every report ends with the line "Based only on events observed locally by ZeroTurn on this machine." The counts are events, never tokens, cost, or a saving.

`status`, `policy show`, `policy check`, `report`, `verify`, `doctor`, and `capabilities` accept `--json`. The schemas are published in [schemas/](schemas), including the repository configuration file and the normalized event an adapter sends.

```sh
go test ./conformance                                                      # every output still follows its schema
go run ./conformance/validate schemas/normalized-event.schema.json e.json  # check one document
```

The validator is part of the project and has no dependencies. A schema using a keyword it does not support is reported rather than skipped, so a contract can never look checked when it is not.

## Harnesses

ZeroTurn reads events from a harness and answers it. Claude Code is
supported. Any other harness can send events in a normalized form by
writing an adapter.

### Claude Code

Supported. The full guide, including what each hook does and which payload fields are read, is in [docs/integrations/claude-code.md](docs/integrations/claude-code.md). ZeroTurn uses these official interfaces:

- the status line, for context, usage windows, and session duration
- `PreToolUse` with the exact matcher `Agent`, the only hook that can allow, ask, or deny a subagent
- `PreToolUse` with the exact matcher `Read`, for the credential guard
- `UserPromptSubmit`, installed only if you switch the prompt guard on
- `SubagentStart` and `SubagentStop`, to count subagents
- `Stop`, to see whether background tasks are still running
- `SessionEnd`

`zeroturn integrate claude` writes to `.claude/settings.local.json` in the repository, the settings file that applies to you only, because the entries contain the path of your local executable. `--user` writes to your user settings instead, and only after the same plan and confirmation. `--remove` deletes ZeroTurn's own entries and leaves everything else as it was.

Tested on Claude Code 2.1.265 and 2.1.270. A denial was honoured in a real session and no subagent started. An allow let the subagent start. The ask decision was observed interactively on 2.1.270: the harness showed the reason ZeroTurn supplied, and refusing it stopped the subagent from starting. The same prompt offers to stop asking for that tool in that directory, which turns the gate off for it, and ZeroTurn is not told when that is chosen. Run `zeroturn doctor --compat --live` to repeat the denial test on your machine. It starts one short session and spends a small amount of usage.

### GitHub Copilot CLI

Not supported yet. No adapter ships, and nothing here claims Copilot support.

The reason it was cut has changed. A re-reading of the installed CLI, version 1.0.83, found hook events for tool use and subagents, a pre tool decision of allow, deny, or ask, quota snapshots, context window token counts, and a local telemetry file for token usage. The details, and what is still unverified, are in [docs/decisions.md](docs/decisions.md).

An adapter is planned. It will be described as supported when it has been run against a real session, and not before.

### Other harnesses

Other harnesses can send events in a normalized JSON form with `zeroturn event --harness normalized`. Each event names the contract `zeroturn.event/1`, and an event that names a different contract is refused with a clear message. `zeroturn capabilities --json` lists the commands, event types, recorded fields, and exit codes.

The contract is the published schemas in [schemas/](schemas) and the suite in [conformance/](conformance) that runs the real executable against them. [integrations/template/](integrations/template) holds a worked example to copy, and `zeroturn capabilities --json` reports what a build supports, including the contract version and every exit code.

## Safety

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

## Contributing

Useful areas: harness adapters, event normalization, project detection, validation presets, platform support, security and redaction tests, terminal rendering, documentation, and conformance fixtures.

Contributions that add any of the following will be redirected: a model SDK or model calls, model selection, a chat interface, a complete coding harness, transcript collection, prompt inspection, remote telemetry, a hosted dashboard, output compression, automatic session handoff, automatic compaction or clearing, shell strings in configuration, automatic force pushing or rebasing, credential bypasses, silent changes to global settings, Python packaging, or containers without a demonstrated need.

Before contributing, check that the feature is not already there and search the existing issues. Then open a bug report or feature proposal on GitHub, and a pull request once the approach is agreed. The steps, and how to build and test, are in [CONTRIBUTING.md](CONTRIBUTING.md). Security problems go through [SECURITY.md](SECURITY.md), not a public issue. The tests use temporary repositories and local bare remotes and never contact a real remote.

## Limitations

ZeroTurn shows and gates what the harness reports. It cannot see usage the harness does not send, and it does not promise to remove session limits.

- Session values appear only when the harness sends them. Some accounts receive no usage window data.
- Subagent counts cover only subagents started while ZeroTurn was installed. One that never reports stopping is cleared when the turn ends.
- The background task count is as current as the last `Stop` event.
- The interactive approval prompt for Confirm mode was observed on Claude Code 2.1.270. It can be bypassed from the prompt itself: the harness offers to stop asking for that tool in that directory, and ZeroTurn is not told when that is chosen.
- Session Guard measurements require the terminal interface. In an editor extension the status line is never invoked, so context, the usage windows, and duration are absent and only the subagent counts and the credential guard work. Confirmed on Claude Code 2.1.257.
- Validation evidence says the checks passed for one state of one repository on this machine, and nothing else. What the digest cannot see is listed under Validation evidence above. It has not yet been used long enough to say how often evidence goes stale in real work, or whether being told changes what anybody does.
- The test suite runs on Linux with both supported Go releases, on macOS, and on Windows, for every change. On Windows, tests that need a POSIX shell are skipped. The harness integration has been used on macOS only.

## Roadmap

Current: everything described above, including both credential guards, plus contributor documents, the harness contract with published schemas and a conformance suite, and continuous integration on Linux, macOS, and Windows.

Planned:

- a published Homebrew tap, so `brew install` works. The formula is generated already; the tap is not published
- a Copilot adapter once Copilot exposes session values to hooks

**Being explored, and not built.** Validation evidence is the first piece of a larger direction: making what an agent did reviewable by the engineer who is accountable for it. The pieces below are under evaluation and none of them exist. Nothing in this repository implements them, and the commands that would carry them are not there.

- a repository handoff record, a durable checkpoint between automated implementation work and human review, holding the repository state, what was done, the decisions and assumptions behind it, the validation evidence, and what still needs a person. This is a file you ask for and read, and it is not the automatic session handoff in the list above, which changes a session behind your back
- a review or release checkpoint that records a human decision against specific evidence
- deployment evidence, which would need an adapter for each CI provider and, by the same rule the Copilot adapter follows, would be claimed only once it had run against a real one
- an editor extension, so the state is visible where the work happens

Validation evidence is being used in real work before any of that is designed around it. What is being watched: how often evidence actually goes stale, whether the warning changes what anybody does, and whether computing the digest stays fast on a large repository. Staleness on its own is infrastructure. It does not yet demonstrate the larger idea, and it is not presented as though it does.

Not planned: `zeroturn sync`. The reasoning, and the research behind the product boundary, are in [docs/decisions.md](docs/decisions.md).

## How this was built

The research came before the code, the differentiator was tested before the rest was written, and the constraints were kept rather than worked around. [docs/how-this-was-built.md](docs/how-this-was-built.md) is the record: what was examined and rejected, what would have stopped the project, the defects its own tests found, and what is still unproven.

One finding is worth naming here, because it outlived the feature it came from. Six defects in this project had the same shape: a description kept by hand beside the thing it described, drifting away from it. The permitted fields beside the record type, the adapter shape beside the event, the policy keys in three copies, the counts in the documentation, the platform claims, and a branch rule requiring a check that a change had just removed. Nine tests here now derive the description from the thing instead, by reflection over a type, by comparing one implementation of a contract with the other, or by counting what is on disk. Every drift with such a check was caught automatically. Every drift without one was caught by luck. That gap matters more when a coding agent writes the changes, because it holds no memory of the parallel lists and makes more changes in a day than a person does. The full account is in [docs/decisions.md](docs/decisions.md), entry 33.

Two findings about the harness interface itself, rather than about this project, are in [docs/how-this-was-built.md](docs/how-this-was-built.md): a decision of `ask` can be permanently disabled from inside the prompt it produces, and a control that reads session state fails open and silently outside a terminal. Both were found by running the guard rather than by reading the documentation, and both have a reproduction.

The code has also been reviewed against itself, and against its own tests. A hook that could spin on a processor forever, two privacy tests that could not fail, a published contract a second harness could not use, and twelve more tests that passed with the behavior they named removed were all found that way rather than by a user. Each one is recorded in [docs/decisions.md](docs/decisions.md) with the reproduction and the test that now fails for it.

## License

ZeroTurn is free software under `GPL-3.0-only`. The full text is in
[LICENSE](LICENSE); [COPYING](COPYING) is the traditional GNU name for the same
file. Contributions are accepted under the same license, with no contributor
license agreement, and there are no dependencies to license: the
executable is built from the Go standard library alone.
