# How this was built

A record of the decisions behind ZeroTurn, written for someone reviewing the work rather than installing the tool. Everything here is checkable against the repository.

## The question came before the code

The first work was not a prototype. It was a search for a reason not to build this.

Sixteen maintained projects were examined: usage reporters, status lines, context guards, budget gates, Git safety wrappers, and local validation runners. Each one was recorded with its purpose, maintainer activity, license, install method, overlap, and difference, with the date each claim was checked.

Six stop conditions were written down first, and a decision to abandon the project was the expected outcome if any of them held:

| Condition | What the research found |
| --- | --- |
| A maintained project already does this | No. The three functions exist in three separate categories, and nothing spans them |
| The distinction is only wording | No. The gate input, the delegation specific trigger, and the locally maintained subagent counts are checkable in code |
| The feature set cannot stay coherent | The reservation was recorded rather than dismissed, and the pairing was kept on the condition that either half can be removed |
| The platform interfaces are missing | Present for Claude Code, absent for Copilot, so Copilot support was cut rather than faked |
| Subagent control cannot be tested | Unknown at the time, and therefore the next thing tested |
| The name has a material conflict | The `zt` alias was dropped because it matches an unrelated company's naming |

The honest result of that research was that most of the idea already existed. The status line is a crowded category, and the local Git and validation commands are conveniences a developer already has. One thing was missing everywhere: asking before delegation, using session pressure as the input.

## The differentiator was proven before the rest was written

Everything depended on one unknown: whether a hook can stop a subagent, and whether the harness honours the answer.

That was tested with one real session, against a temporary settings file, before the remaining code existed. The findings changed the design:

- `SubagentStart` cannot block. It accepts only text added to model context, which this project refuses to use.
- `PreToolUse` with the tool name `Agent` can allow, ask, or deny, and the harness honours a denial.
- The payload carries the subagent's prompt, which ZeroTurn therefore never binds to a variable.

The test is part of the product: `zeroturn doctor --compat --live` repeats it on any machine, in a throwaway repository, and reports what happened. If it had failed, the honest outcome would have been to keep only the observation mode and say so.

## The constraints were chosen, and kept

One executable, the Go standard library only, no daemon, no network, no model calls, no database.

Those constraints cost something, and the cost was paid rather than avoided:

| Needed | Usual answer | What was done |
| --- | --- | --- |
| Validate JSON against published schemas | A schema library | A small validator in `internal/jsonschema`, which reports any keyword it cannot check rather than skipping it |
| An animated terminal recording | A recording tool | `docs/demo/render`, which turns a real transcript into an animated SVG with CSS keyframes |
| Release archives for five platforms | A release tool | `scripts/build-release.sh`, twenty lines around `go build` |
| Enforce the writing rules | A prose linter | `scripts/lint-copy.sh`, which fails the build on a banned phrase |

The dependency list has one row: the Go standard library.

## Refusals are part of the design

A list of things this project will not do is kept in [decisions.md](decisions.md) and in the contributing guide: no model calls, no transcript reading, no telemetry, no output compression, no automatic compaction or clearing, no shell strings in configuration, no automatic force pushing or rebasing, no credential bypass, no silent changes to global settings.

Two refusals shaped the code more than any feature:

- **Never read private content.** The event decoder declares only permitted fields, so anything else the harness sends cannot be bound to a variable. When the credential guard needed a file path, exactly one field was added, and the reason is recorded.
- **Never claim enforcement that was not observed.** Confirm mode is documented as accepted by the harness but not seen end to end, because nobody has watched the approval prompt appear.

## Evidence, not assertions

| Claim | Where it is checked |
| --- | --- |
| It behaves as documented | 391 tests across 63 files, run on Linux with both supported Go releases, on macOS, and on Windows |
| Output matches the published contract | `conformance/`, which runs the real executable against 9 schemas |
| It is fast enough to sit in a hook | `scripts/bench`: status line 7.5 ms, gate 7.7 ms, measured over 50 runs |
| A change the tests would not notice | `scripts/mutate` alters one operator at a time and runs that package's tests; on `internal/policy` it makes 56 changes and 55 are noticed |
| It keeps nothing private | Tests that walk the state directory after each kind of event |

## The tests found real defects

These were found by the project's own tests and continuous integration, not by a user:

- The status line started a Git process on every repaint, which the specification forbids.
- A `.zeroturn.json` committed with Strict mode could deny subagents on a machine where nobody had approved it.
- `ship` read relative paths from the repository root rather than the working directory, accepted a mistyped path as an intentional deletion, and staged a symbolic link's target instead of the link.
- File names with non ASCII characters could be reported as unrelated staged files.
- `doctor --compat --live` could never have passed, because its temporary folder was not a repository and its session had no context value.
- A test that checked for private paths matched a two character directory name and failed for no reason on one runner.

The performance work came from measurement as well: the first numbers were 31 ms and 53 ms, and two changes brought both to about 8 ms.

## The same defect, six times

One failure kept coming back, and it was not noticed as a pattern until the sixth time. A description of something was kept by hand beside the thing it described, and the two drifted apart.

| What drifted | From what |
| --- | --- |
| The list of fields a session record may store | the `Session` type |
| The shape an adapter can send | the `Event` type the gate reads |
| The table of policy keys, in three copies | the `Guard` type |
| The counts in this document | the repository |
| The platform claims in the README | what the harness actually delivers |
| The required checks in the branch ruleset | the jobs the workflow defines |

Each one was written up on its own as a new lesson. Read together they are one lesson: a description maintained beside a thing will drift, and the answer is to derive it from the thing instead. Nine tests in this repository now do that. They compare against a type by reflection, against the other implementation of a contract, or against the files on disk, and they fail in both directions, so neither a missing entry nor a stale one survives.

The evidence for whether that works is in what happened next. Every drift with a derived check was caught automatically: three unlisted fields in the stored record, a contract version an adapter could not send, `--help` broken on seven commands, and the counts in this document twice within an hour of the check being written. Every drift without one was caught by luck: a test count out by a hundred, platform claims in three documents, and a branch rule requiring a check that no longer exists, which would have blocked every merge once it was applied.

## Why this matters more with an agent writing the code

A person changes one thing and drifts slowly enough that review catches it. This project was built with a coding agent, and the drift arrived faster than anyone reads.

In a single afternoon: changing the continuous integration matrix left three documents stating the old one, and changing it back left them stating that; adding two decision entries made the count in this document wrong twice; and changing a workflow silently invalidated the branch rule that depended on its job names. None of that was carelessness. An agent holds no memory of the parallel lists scattered through a repository, and it makes more changes per day than a person, so every hand-maintained description rots faster and is read less often.

That is the argument for deriving rather than maintaining, and it is stronger now than it was before agents wrote code. A check that reads the type is the only kind that cannot be forgotten, because there is nothing to remember.

## Two ways this kind of guardrail fails

Found by running the guard against Claude Code 2.1.257, 2.1.265 and 2.1.270, not by reading the documentation. The interface works as documented; neither is a defect in the harness. They are the difference between what a hook can decide and what a person ends up experiencing, and anyone building a control on this interface inherits both.

**A decision of `ask` can be disabled from inside its own prompt.** The harness renders the reason and three answers, not two. The second is "Yes, and don't ask again for this tool in this directory". It records a permanent permission, it appears at the one moment the control is inconvenient and the person is most willing to take it, and the hook is never told. The guard then cannot tell a session where its thresholds were never crossed from one where it was switched off in the first hour. Both look like nothing happened.

The consequence for anyone building this: a control that must hold cannot be built on `ask`, because `ask` is a request the person can permanently withdraw. A guard that stays on `ask` is advisory, should say so, and must not report an absence of prompts as an absence of risk.

**A control that reads session state fails open, silently, outside a terminal.** Context use and the usage windows are carried by no hook payload. The only carrier is the status line, which the harness invokes to draw a line at the bottom of a terminal. An editor extension draws none and never invokes it. Every hook still fires, so the integration looks correct from every other angle, and the policy engine evaluates against measurements it never received. It cannot cross a threshold it cannot read, so it allows, quietly, for as long as the session runs.

That was found by reading three days of stored records and noticing that every one held a harness name, subagent counts and credential warnings, and none held a context value, a usage window, a duration or a model. Seven sessions across two machines. Feeding the same code a recorded status line payload produced all of them, which ruled out the code.

The fix was not to the logic. `doctor` now compares the sessions it recorded and reports how many carried measurements, which turns a silent failure into a visible one:

```
WARN  session data   6 of 7 recent sessions for this repository carried no
                     context or usage values.
```

What the two have in common is that both are failures of a control's assumptions about its environment rather than of its logic, and neither is visible from inside the control. The first assumes an answer stays given. The second assumes an input will arrive. In both cases the guard reports success, because from where it sits nothing went wrong. A guardrail built on an agent harness needs to state which of its inputs are optional, and to report when one it depends on has never arrived.

## What is still unproven

Written in the README, not buried:

- The interactive approval prompt was observed on 2026-09-14, on Claude Code 2.1.270, and the refusal was honoured.
- The integration has been used on macOS only, although the tests run on three systems.
- The credential guards match high confidence patterns, so they reduce a common mistake rather than eliminate a class of them.
- Full gate testing is still in progress. The default thresholds are starting points, and `zeroturn policy tune` suggests better ones from what the gate asked and what you answered.

## How decisions are recorded

51 entries in [decisions.md](decisions.md), each with the decision, the evidence, and the consequence. They include the ones that cut scope: `sync` dropped, Copilot deferred, goreleaser refused, and the license text left untouched. A decision that turns out to be wrong is meant to be replaced there, with its reason, rather than quietly reversed.
