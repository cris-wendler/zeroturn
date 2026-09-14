# Decisions

Each entry records what was decided, the evidence behind it, and what it changes. The research behind the early entries is in [product-boundary.md](product-boundary.md).

## 1. The gate is the product, and sync is not built

Date: 2026-09-11

Decision: Session Guard and the subagent gate are the product. `verify` and `ship` stay because they pair with the gate. `zeroturn sync` is not built.

Evidence: the competitive review found three things.

- Session status display is a crowded category. Several maintained status line projects already render the same official context and usage window fields. The ZeroTurn status line is not better than those. It exists to carry the gate and the subagent counts.
- The direct commands are conveniences. `verify` runs configured project commands, `ship` stages named files, commits, and pushes with a credential scan, and `sync` would be a fast forward only pull. A developer already has each of these. Once the unsupported claim that routine Git commands cause session limits is removed, the measurable benefit of this half is small.
- Nothing examined asks for approval before delegation specifically, using session pressure as the input. That gap is narrow and real.

Consequence: the command list is `init`, `integrate`, `status`, `policy`, `report`, `verify`, `ship`, `capabilities`, `doctor`, and `version`, plus `event`, the adapter entry point. Documentation must not list `sync`. It can be reconsidered if users ask for it, and it would follow the sequence and refusals already specified for `ship`.

## 2. The gate was proven before the rest was written

Date: 2026-09-11

Decision: implementation beyond the gate waited for a live test of the harness response.

Method: one real session of the supported harness, run against a temporary settings file passed on the command line, with a hook that returned a fixed decision for the `Agent` tool. The user's own settings were not changed.

| Decision | Result |
| --- | --- |
| deny | Honoured. The subagent did not start. |
| allow | Honoured. The subagent started, and the start and stop events arrived with an agent identifier. |
| ask | Accepted by the harness. In an unattended run the harness refused the call, which is the safe outcome. The interactive approval prompt itself was not observed. |

The payload confirmed that the tool name is exactly `Agent`, so the matcher is an exact string. It also confirmed that the payload carries the subagent prompt, which ZeroTurn never binds to a variable.

Follow up, 2026-09-11: `zeroturn doctor --compat --live` repeated the deny test through ZeroTurn itself, using the real gate, a temporary repository, and temporary ZeroTurn state. On Claude Code 2.1.265 the harness honoured the denial and no subagent started.

Open item: the interactive approval prompt for `ask` has still not been observed. Until it is, Confirm mode is described as returning the native ask decision, not as tested end to end. The interactive check was not run because answering the harness workspace trust dialog for a temporary folder would write an entry into the user's global configuration.

## 3. No `zt` alias

Date: 2026-09-10

Decision: the executable is `zeroturn` only.

Evidence: ZeroTurnaround publishes libraries named `zt-zip`, `zt-exec`, and `zt-process-killer`, and `zt` is occupied on npm and PyPI. See product-boundary.md, question 5.

## 4. The Copilot adapter is deferred

Date: 2026-09-11. Superseded in part by entry 19, which corrects the evidence.

Decision: version one ships no Copilot adapter. The README lists Copilot as planned and explains why.

Evidence: no Copilot hook payload carries usage, context utilisation, or session duration, so Session Guard has no pressure values to show there. Custom local commands require the experimental extension interface. Gating through `preToolUse` on the `task` tool looks possible but has not been tested.

Consequence: the harness contract, the normalized event format, and `zeroturn event --harness normalized` remain, so an adapter can be added without changing the core.

## 5. Go 1.17 and the standard library only

Date: 2026-09-11

Decision: the module targets Go 1.17 and has no dependencies.

Evidence: Go 1.17 was the toolchain available when the build started. Using only the standard library leaves no dependency licenses to audit and no dependency updates to follow.

Consequence: current editor tooling (gopls) does not support Go 1.17. Raising the `go` directive is a separate decision and would not add dependencies.

## 6. Repository location and dogfood target

Date: 2026-09-11

Decision: the module path is `github.com/cris-wendler/zeroturn`, because the GitHub account `zeroturn` belongs to an inactive user. The GitHub repository exists and nothing has been pushed to it. The dogfood example uses a second project on this machine read only. That project is not modified, committed to, or pushed.

## 7. What Strict mode can deny

Date: 2026-09-11

Decision: Strict mode denies a new subagent only when context use reaches the critical threshold. Every other threshold asks.

Evidence: context critical is the only configured hard limit. The usage windows and session duration have a single warning threshold each, and background task counts are only reported at the end of a turn, so they can be stale by the time a subagent is proposed.

## 8. Strict mode is approved per machine

Date: 2026-09-11

Decision: `guard.mode` set to `strict` in a repository's `.zeroturn.json` does not deny anything until the person on this machine approves it with `zeroturn policy set guard.mode strict`. Until then the gate asks instead of denying. Approval is stored in ZeroTurn's local state, bound to the repository hash, and withdrawn when the mode is changed or reset.

Evidence: the configuration file is committed and shared. Without this rule, a repository could switch on denial for everyone who clones it, which the requirement that Strict is never enabled automatically forbids.

## 9. The integration writes to the per user project settings file

Date: 2026-09-11

Decision: `zeroturn integrate claude` writes to `.claude/settings.local.json` unless `--user` is given. The plan warns when that file is not ignored by Git. Removal deletes ZeroTurn's own hook commands one by one, so a command someone added to the same entry survives.

Evidence: the entries contain the absolute path of the local `zeroturn` executable. In the shared `.claude/settings.json` that path would be committed and would be wrong on every other machine.

## 10. Retention is applied when a session starts

Date: 2026-09-11

Decision: session records older than the seven day default are removed when ZeroTurn first sees a new session. `zeroturn report purge` remains for manual removal.

Evidence: the status line runs on every repaint, so retention there would repeat the same directory scan many times a minute. Once per session keeps the store bounded at negligible cost.

## 11. Release archives are built with the Go toolchain, not goreleaser

Date: 2026-09-11

Decision: `scripts/build-release.sh` cross compiles the five release archives and writes `SHA256SUMS`. There is no `.goreleaser.yml`, which the original plan named.

Evidence: the whole build is `go build` with `CGO_ENABLED=0` and `-trimpath`, repeated for five targets. goreleaser would add a tool that every maintainer has to install, and a configuration format to track, for a build that the Go toolchain already does in twenty seconds. The project takes the same position on build tools as on dependencies.

Consequence: the release workflow runs the script, attaches the archives to a draft release, and never publishes. The Homebrew formula is generated from the checksums by `scripts/homebrew-formula.sh` and lives in a separate tap repository.

## 12. Considered: warn before a credential reaches the model

Date: 2026-09-11. Status: proposed, not decided, not built.

The idea: ZeroTurn already detects credentials in content that `ship` is about to stage. The same detection could run earlier, before a credential reaches the model at all, so that nobody has to rotate a key that was pasted into a session by mistake.

Two forms, which differ in what they cost.

**Before a file is read.** `PreToolUse` fires for the file reading tools with the path the model is about to open. ZeroTurn could scan that file and answer ask or deny, with the file and the category and never the value. This reads only a path that the harness already hands to a hook, and the file is one the model was about to read anyway. It does not touch prompts, so the privacy promise holds as written. Limits: a credential can still reach the model through a command whose output is not scanned, and gating every command would mean reading tool arguments in general, which the scope forbids.

**Before your message is sent.** `UserPromptSubmit` fires with the text of the prompt and can refuse it. This is the only place that catches a credential typed or pasted into the session. It also means ZeroTurn reads prompts, which version one refuses to do, and which several statements in the README and the privacy section depend on. The scan would be local, would store nothing, and would report only a category, but the promise changes from "never reads prompts" to "reads prompts locally to look for credential patterns, and keeps nothing".

Honest limits either way: the detection is a list of high confidence patterns. It will miss credentials in unusual formats, so it reduces a common mistake rather than preventing a class of them. It must never claim more than that.

The decision needed: whether the second form is worth changing the privacy promise, or whether only the first form is built. Until that is decided, neither is implemented.

## 13. Context thresholds stay where they are, although the harness also warns

Date: 2026-09-11

Decision: the context defaults stay as they are: warn at 70, ask at 80, and deny at the critical threshold of 90. They are not raised to sit above the harness's own context warning.

Evidence: Claude Code tracks the context window itself, shows how much is left before it summarises the conversation, and then summarises automatically. That warning is about compacting. ZeroTurn's question is different: is it sensible to start another subagent now. Asking that while there is still room to act is the point, so the threshold sits below the harness warning on purpose.

Consequence: at high context a developer may hear about context from both tools. The overlap is documented in the Claude Code guide so it does not look like a fault, and every threshold is configurable with `zeroturn policy set`. The measurements the harness does not report at all, the usage windows, session duration, and subagent counts, are the ones only ZeroTurn watches.

## 14. One copy of the license text, under two names

Date: 2026-09-12

Decision: `LICENSE` holds the complete official GPL version 3 text, unmodified. `COPYING` stays, because that is the name the GNU project uses, but it names the license, gives the SPDX identifier, and points to `LICENSE` instead of repeating the text.

Evidence: with the full text in both files, GitHub detected two licenses and showed two identical "GPL-3.0 license" tabs on the repository page, which reads as a mistake. The plan asked for both files and for the official text without modification. Both still hold: the text is present once, unmodified, and both file names exist for anyone looking for either.

## 15. Every change goes through a branch and a pull request

Date: 2026-09-12

Decision: no more commits directly on `main`. Each change goes on a branch named for its kind, `feat/`, `fix/`, `docs/`, `test/`, or `chore/`, and reaches `main` through a pull request that is squashed into one commit. The branch is deleted on merge. The repository allows squash merges only.

Evidence: the first days of work were committed straight to `main`, which is reasonable for a prototype nobody can see. Once the repository is public, that history shows no review and no trace of why anything changed. A pull request per change gives each one a description, a place for review comments, and a run of the checks before it lands, and it costs a minute.

Consequence: `main` keeps one commit per change. The detail of how a change was built stays in its pull request rather than in a string of small commits.

## 16. The credential guard reads files, not prompts

Date: 2026-09-12

Decision: the first form proposed in entry 12 is built. `PreToolUse` with the exact matcher `Read` gives ZeroTurn the path of a file the model is about to open. ZeroTurn scans that file with the detection `ship` already uses and answers ask or deny, naming the file, the line, and the category, never the value. The default is ask, and `guard.credentials.mode` can set deny or off.

The second form, scanning what you type or paste, is still not built. It is the only way to catch a credential pasted into a message, and it would mean ZeroTurn reading prompts, which every privacy statement here rules out. That remains a decision for later.

Evidence: a credential that reaches the model has to be rotated, and the most common way for one to arrive is a file the model reads on its own, an environment file or a key. The scan happens locally, the content is never stored, and only a count of warnings is kept.

Consequence: ZeroTurn now declares one field of `tool_input`, `file_path`, for that one tool. Everything else a tool carries still has no field in the decoder. The contract version moves to 1.1.0, and the integration installs a second `PreToolUse` entry with an exact matcher.

Limits, stated wherever the feature is described: high confidence patterns only, files over 1 MB skipped, and a credential reaching the model by another route, a command's output for example, is not caught.

## 17. The prompt guard is built, and it is off by default

Date: 2026-09-12

Decision: the second form from entry 12 is built. With `guard.credentials.prompts` set to `block`, a `UserPromptSubmit` hook hands ZeroTurn each message before it is sent, ZeroTurn scans it for credentials in memory, and stops one that carries a value. The default is `off`, the hook is not installed while it is off, and switching it on asks for confirmation after stating both costs.

Evidence: a key pasted into a message reaches the model exactly as surely as one in a file, and the file guard cannot see it. The harness supports only block or allow for a message, so there is no ask.

The costs, stated wherever this is described:

- ZeroTurn reads messages in that repository while the guard is on. It keeps nothing, but the promise changes from "never reads what you write" to "reads it in memory, in this repository, when you asked for it".
- A blocked message is erased by the harness rather than handed back, so a long message is lost.

Consequence: the privacy section now describes both guards. The normalized contract is unchanged: there is no prompt event in it, and an adapter never sends message text.

## 18. Thresholds are proposed from behavior, not guessed

Date: 2026-09-12

Decision: ZeroTurn records each gate decision with the measurements behind it, and whether a subagent started afterwards. `zeroturn policy tune` reads those records and proposes thresholds. It never changes one.

Evidence: the defaults were chosen for a first run and are described that way. Nothing else in the tool tells a developer whether they are right, and a guard that asks too often is switched off rather than tuned. The harness reports the one outcome that matters: a subagent starting after an ask means the developer approved it. That is enough to tell an early threshold from one that is doing its job, and it needs nothing about the work itself.

Consequence: a session record holds up to twenty outcomes, each a decision, a time, a list of threshold names, and the measurements at that moment. No text, and nothing a person wrote. The suggestion is printed as the exact command to run, and it stays a suggestion: five observations are required before one appears, and a threshold that is often declined is reported as working rather than changed.

The limit, stated in the output: approval is inferred. A subagent that starts for another reason after an ask would read as an approval.

## 19. The reason Copilot was cut was wrong for the shipped version

Date: 2026-09-12

Finding: entry 4 deferred Copilot because no Copilot hook payload carried usage, context, or session duration. That was taken from the documentation. Reading the installed package, version 1.0.83, shows the CLI ships hook events for `preToolUse`, `subagentStart`, `subagentStop`, `sessionStart`, and `sessionEnd`; a pre tool decision of allow, deny, or ask with a reason, the same shape Claude Code uses; quota snapshots with entitlement, used requests, and reset date; context window token counts; a local telemetry file exporter for token usage; and its own session budget through `--max-ai-credits`. Hooks can be configured as files, without the experimental extension interface.

Decision: the claim is corrected in [product-boundary.md](product-boundary.md) with the file each capability was verified in, and an adapter moves from refused to planned. What ZeroTurn says about Copilot support stays at nothing until an adapter exists and has been run against a real session, because the rule is that support is claimed only where it has been observed.

Still unverified, and therefore still not claimed: the schema of a `.github/hooks` file, whether a hook defined as a command receives the same input as an extension callback and may answer with a permission decision, and whether usage values reach a hook at all rather than only the account interface and the event stream.

The lesson worth keeping: a capability check has a shelf life. This one was three days old. Anything the scope depends on should be re-read from the installed software before it is repeated.

## 20. The gate asks about the rate of use, not only the level

Date: 2026-09-12

Finding: a fixed percentage is a poor question. Seventy five percent of the five hour window is fine at the end of the window and a problem at the start of it, and the two cases are indistinguishable to a threshold. A developer already sees the percentage in the harness; repeating it in a gate adds nothing they did not know.

Decision: the gate also measures how fast the window is being used. The first five hour reading of the window is kept, with the time it was taken and the reset time it was taken under. The rate is the rise since then per hour, and the projection is where the window lands when it resets. When that is one hundred percent or more, the gate asks, whatever the reading is now. A heavy session that will still finish inside the window says nothing.

Consequence: three fields were added to the session record, which moves the contract version to 1.2.0, all measurements, none of them anything a person wrote. The setting is `guard.limits.projection`, on by default, and `off` restores the old behavior exactly. The projection is skipped once the fixed threshold has already fired, so a gate never states the same thing twice.

The guards on the estimate matter more than the estimate. A rate is ignored when it has been measured over less than fifteen minutes, when the baseline was taken under a different reset time, when usage is not rising, and when the reset time has passed. A baseline is also replaced when a reading falls below it, because a window that rolled over without a reset time is otherwise indistinguishable from a quiet session.

The limit: this is an estimate, and it is treated as one. It never denies, even in Strict mode, because a projection is not a fact. It also assumes the next hour looks like the last one, which a session that is about to stop does not.

## 21. What Copilot exposes, read from the hook engine rather than the schemas

Date: 2026-09-12

Finding: entry 19 corrected the Copilot research but left four questions open, because the schemas did not answer them. They were answered by reading the native hook engine in the installed package, version 1.0.83, and the answers change what an adapter can be.

A hook can be an external command. A definition carries one of `command`, `bash`, `powershell`, `exec`, `url`, or `prompt`; the process's standard output is parsed as JSON, and exit code 2 denies the tool call. `preToolUse` answers with `permissionDecision` of `allow`, `deny`, or `ask` and a `permissionDecisionReason`, which are the same words ZeroTurn already produces.

No hook payload carries usage, quota, token counts, or session duration. That part of the original research was right, and it was checked directly rather than assumed: the hook input types carry only the session, the time, the working directory, and the tool. The quota structures sit in the account client, a different module.

Session pressure is still reachable, through a different door. `statusLine.command` runs a child process and hands it the session status as JSON on standard input, including context window use, the window size, premium requests, and credits. That is the same arrangement ZeroTurn already uses on Claude Code.

Decision: the adapter is worth writing, and it uses two entry points rather than one, a `preToolUse` hook for the gate and the status line process for pressure. The findings are recorded in [integrations/copilot.md](integrations/copilot.md) with the file each one came from. What ZeroTurn claims about Copilot stays at nothing until an adapter has run against a real session, which is the same bar the Claude Code gate had to clear.

Two spellings would have failed silently if an adapter had guessed them: the prompt event is `userPromptSubmitted`, not `userPromptSubmit`, and the end of turn event is `agentStop`, with no plain `stop`.

The lesson, again: the answer was in the binary, not in the schemas. Entry 19 said a capability check has a shelf life. This one adds that a capability check has a depth, and stopping at the published schema is not the bottom.

## 22. The state lock answers, whatever is wrong with the directory

Date: 2026-09-12

Finding: an architecture review found that `Lock` could not fail. Its retry loop treated every failure to create the lock file as a lock somebody else was holding. When the entry looked stale it removed it and repeated, without checking the deadline and without sleeping. If the removal could not succeed, the loop spun on a processor and never ended.

It was reproduced through the real executable, not only in a test. With `state.lock` present as a directory rather than a file, `zeroturn event` ran at one whole processor and never exited. A read only state directory did the same. That contradicts the fail open rule in [harness-contract.md](harness-contract.md), which promises that an unreadable state directory ends in silence and exit 0. A hook that never returns is worse than one that fails: it holds the harness until its timeout and then keeps burning a processor after it.

Decision: only a lock another process holds is waited for. Any other failure is returned at once, the deadline is checked on every pass, and a lock left behind by a dead process is cleared once rather than repeatedly. The same scenario now exits 0 in the time the deadline allows.

Consequence: `Lock` can return an error that is not `ErrLocked`. Every caller already treats a failure to lock as a reason to do nothing, and `zeroturn event` still exits 0, so no behavior changes for a healthy installation. The acquisition deadline became a package variable so a test can shorten it instead of waiting five seconds for each case.

Two attempts at this fix were wrong, and Windows failed both. The first retried only when the error said the file already existed: a lock another process holds is reported there as access denied rather than as an existing file, so sixty concurrent writers lost ten of their updates. The second looked at whether the lock entry was there, which is right in principle, but a lock in the middle of being released is briefly neither there nor gone, and a writer that arrived in that moment gave up.

What works is to treat the entry staying absent as the signal, rather than one reading of it. A directory nothing can be created in shows an absent entry every time, and gives up in a few milliseconds. A release race shows it once and keeps waiting.

The lesson: the retry loop was written for contention and tested for contention, and both tests passed. Nothing asked what happened when the operation could not succeed at all. A loop that retries needs a test for the case that never succeeds, not only for the case that succeeds late. The second lesson is that the fix for it was wrong on a system nobody ran it on, and continuous integration on three systems is what said so.

Still open, and recorded rather than fixed here: the lock has no ownership mark, so a lock judged stale and removed while its holder is merely slow can be removed twice; and the staleness limit is longer than the acquisition deadline, so one invocation cannot recover from a holder that died moments ago.

## 23. The privacy tests are checked against the type, not against a sample

Date: 2026-09-12

Finding: the two tests that guard the promise this project rests on could not fail.

The first checks the stored record against a list of permitted fields. It built one record, set three values on it, and inspected the keys that appeared. Almost every field is omitted when it holds no value, so a field the sample did not set never appeared, and a field nobody meant to store passed unnoticed. The three fields added by the rate projection were missing from the permitted list and the test still passed.

The second asserts that no transcript path survives parsing. No payload in its table carried one.

Decision: the record is compared against the type. Every name `Session` can write, including the names inside a gate outcome, must be on the permitted list, and a name on the list that no longer exists fails too, so the list cannot go stale in either direction. A second test keeps the check against what is actually written, because the type says what can be stored and only a written record says what is. The event payloads now carry a marker in a transcript path, a subagent prompt, a typed message, an assistant reply, and file content, and the test asserts the marker does not survive.

Both were confirmed to fail before being relied on. Adding a `lastPromptText` field to the record fails the first. Binding an assistant reply in the decoder fails the second.

The second test recorded an exception rather than hiding it. A message on its way out is bound on the prompt event, because the prompt guard cannot scan a message it has not read. It stays in memory, it is never stored, and the test clears that one field and checks everything else, so the exception is visible in the code rather than implied by a passing test.

The lesson: a test that asserts an absence has to be shown failing. An absence is the default state of an empty object, and a test that never sees the thing it forbids is indistinguishable from one that works.

## 24. Asking for help is not a failure

Date: 2026-09-12

Finding: the top level usage told a reader to run `zeroturn <command> --help`. Doing so printed the flag package's own listing, in a format the rest of the program does not use, followed by an error saying the flags could not be read, and exited 2. Every command that takes flags behaved this way. `ship`, which parses its arguments by hand, was the only one that did not.

The exit code made it more than untidy. Two is invalid usage here, and it is also the code a harness reads as a blocking error.

Two worse cases turned up while fixing it. `zeroturn integrate --help` reported that `--help` was not a flag it accepts. `zeroturn policy reset --help` restored the default thresholds: the command took no notice of its arguments, so asking it what it does made it do it.

Decision: one helper reads the flags for every command. A request for help prints that command's own usage and the flags it accepts, and stops with success. Commands that read a word before their flags, which are `report`, `policy`, and `integrate`, answer it before that word is interpreted. Usage text was written for the six commands that had none.

`policy reset` now refuses any argument it does not understand rather than ignoring it. A command that writes should not treat an unrecognised word as permission to proceed.

`zeroturn event` answers help too, and still exits 0 on every other failure, because a gate that fails must not stop a session.

Consequence: a test runs `--help` against every command and subcommand and requires success, the command's own name in the output, no flag package listing, and no error text. A second test keeps an unknown flag an error, with the wording and the code it had before.

The exit codes gained the test they never had. Every assertion in the suite used the named constant, so renumbering one would have left the suite green and broken every adapter. The numbers are now written out once, and the capabilities document is checked against them.

## 25. An adapter can send what the gate measures

Date: 2026-09-12

Finding: the rate projection was built on the five hour reset time, the contract version was raised to 1.2.0 for it, and the adapter facing event had no field for a reset time. The schema declares `additionalProperties: false`, so an adapter could not even send one legally. `Project` returns nothing without it, so the feature was silently unavailable to every harness except Claude Code.

Nothing caught it because nothing compared the two parsers. Each was tested against its own payloads, and each passed.

Decision: `fiveHourResetsAt` and `sevenDayResetsAt` are part of the normalized event and the published schema, and the contract document tells an adapter to send them and says what is lost without them.

Two tests hold the two paths together. The first parses the same session from a Claude payload and from a normalized one and requires the resulting events to be equal. The second walks the event type and fails when a field exists that the adapter facing shape has no way to carry, with the deliberate exceptions named in the code. Both were confirmed to fail against the shape that shipped this morning.

The lesson: a contract with two implementations needs a test that compares them. Testing each against its own fixtures proves only that each is self consistent, which is exactly what a divergence looks like from the inside.

## 26. The fold and the Strict downgrade are behind types

Date: 2026-09-14

Finding, from the architecture review: the two rules the product rests on lived in the command package as convention.

The first is the fold, event into session record. It decides what every reading means, and every entry point depends on it agreeing, and it was a function in `package main` that no test could reach without building and running the binary.

The second is the Strict downgrade. A repository file can ask for Strict mode, but only the person at this machine can enable it, because otherwise a committed `.zeroturn.json` would deny subagents for everyone who clones it. That is a security rule, and it was applied by remembering to call one helper at five separate places. `policy.Evaluate` did not know the rule existed. A sixth call site that forgot would have reopened the hole, and nothing would have failed to compile.

Decision: the fold moves to `internal/session`, where `Apply` folds one event into a record and `ApplyAt` supplies the clock the five hour baseline needs. The downgrade becomes `policy.Guard`, a value that can only be built by `NewGuard`, which takes the configuration and the answer to one question: has this machine approved Strict. `Evaluate` takes a `Guard` and no longer accepts a configuration, so the check cannot be skipped by forgetting it. There is nothing to remember, because there is no other way in.

The command package keeps one helper that answers that question from the approval record. The self test in `doctor --compat` now says in its own code that it approves Strict, because it asks what the policy table does rather than what this machine allows, which was an assumption it made silently before.

Both new tests were confirmed to fail first. Removing the downgrade from `NewGuard` fails three of the guard tests. Folding a missing reading as zero fails the fold test.

The second test is only that strong because the first attempt at it was not. It checked that a measurement was still present after an event that did not carry it, and a fold that overwrote every reading with zero passed, because zero is present. It now checks the value. An absent reading is not a reading of zero, and a field that is there and wrong is worse than one that is gone.

## 27. The settings writer and the ownership rules are libraries

Date: 2026-09-14

Finding, from the architecture review: `cmd_integrate.go` was 843 lines, and roughly four hundred of them were a library that had nothing to do with being a command. Two things lived in there that the rest of the project depends on being right, and neither could be tested without building the executable and running it.

The first is the settings writer. A settings file belongs to the person using it, and ZeroTurn rewrites it. Keys keep their position, values it does not recognise keep their content, and the write is atomic, because a harness reads this file at startup and half of one would stop it starting. That is a promise, and it was being checked only by end to end tests that ran the binary and read the file back.

The second is ownership. ZeroTurn installs its own entries, repairs them when the executable moves, and removes them, and it must never touch anything else, including a command someone added to the same entry as one of ZeroTurn's.

Decision: `internal/settings` holds the file, as a `File` with `Read`, `Set`, `Delete`, `Section`, `Backup`, and `Write`. `internal/harness/claude` holds the ownership rules and the plan: what would be added, what would be repaired, what is already installed, and what belongs to someone else. The plan is worked out from values in memory and touches nothing, so the same value is what `--plan` prints and what `--apply` acts on, and the two cannot disagree.

The command keeps what a command should keep: flags, where the settings file is, the note about Git ignoring it, the printing, the confirmation, and the mapping to exit codes. It is 376 lines.

Both new packages have tests that were confirmed to fail first. Ignoring the recorded key order fails the settings test. Dropping a whole entry rather than only ZeroTurn's commands in it fails the ownership test. The ten end to end tests that drive the real binary were not changed and still pass, which is what says the behaviour is the same.

`SamePath` is exported because `doctor` applies the same rule when it checks whether the installed entries point at this executable. Comparing two paths as files rather than as text is what makes a symbolic link, which is how a package manager usually installs an executable, not read as a different program.

## 28. The tuning analysis is a library, and its published names are pinned

Date: 2026-09-14

The second half of the extraction the review asked for. `cmd_tune.go` held the analysis that reads what the gate asked and what the developer did next: which threshold was crossed, whether a subagent followed, and what that implies about where the threshold should sit. It is the only part of ZeroTurn that suggests a change to a setting, and it lived in the command package.

Decision: `internal/tune` holds it, as `Analyse` over a configuration, a list of session records, and the repository the records must belong to. The command keeps the flags, the store, and the printing. Records from another repository are ignored, as before: thresholds are per repository, and a history from elsewhere would suggest a change for work it never saw.

A dead field went with the move. `Values []string` was declared on the threshold type, never written and never read.

The six analysis tests moved with the code and are unit tests of a package now rather than of a command. The end to end test stays where it is, because what it proves is that an approval is learned from a subagent starting, which needs the real binary.

One test is new, and it was confirmed to fail first. `schemas/policy-tune.schema.json` sets `additionalProperties: false`, so a renamed field makes the output invalid for every adapter. The conformance suite validates this command's output, but only ever against a report with no thresholds in it, so the names inside a threshold were checked by nothing. They are pinned now, in both directions: a name that appears and is not in the schema fails, and a name that stops appearing fails. Renaming `suggested` fails it.

This mattered here because the move renamed the Go field that holds the list, from `Tunings` to `Thresholds`, while its JSON name stays `thresholds`. That is exactly the edit that silently breaks a contract.

## 29. One registry describes a setting, and the type says when it is missing

Date: 2026-09-14

The last of the three extractions the architecture review asked for. A guard setting was described in three places that had to agree and were checked by nobody: the switch in `policy set` that wrote it, the table in `policy show` that printed it, and the list in `Validate` that range checked it. Adding a setting meant remembering all three. The rate projection, added a day earlier, needed edits in each.

Decision: `internal/config` holds one registry. A key carries the name a person types, the label the table prints, what the value means, the values a choice accepts, and one accessor that returns the field itself. Because the accessor returns the field rather than copying it, reading and writing cannot drift apart, and the wording that refuses a bad value is written once instead of at every key.

`policy show` prints the registry. `policy set` looks a key up in it and reports what it refuses as a `ValidationError`, the same type the file validation already used. `Validate` reads the registry for both halves of its work: every choice is checked against the values that key accepts, and every percentage against its range, so a setting added to the registry is validated without being added anywhere else. A value typed at the command line and the same value committed to the file are now refused by one piece of code, in the same words, which a test checks by comparing the two. The command is 376 lines, down from 470, and the validation lost four switches that spelled out the accepted values a second time.

Two tests hold it, and both were confirmed to fail first. The first reads every setting out of the `Guard` type by reflection, using the JSON path a person would type, and compares it against the registry in both directions: a field with no key fails, and a key naming a field that is gone fails. Removing the key for `guard.session.subagentStartsWarn` fails it by name.

A third test compares the two paths into the same rule: a bad value stored in the file and the same value typed at the command line have to produce the same detail and the same fix. Giving `Check` its own wording fails it for all four choice settings.

The second test is the one that matters most. An accessor can point at the wrong field and still compile, which is the defect this shape invites. So each key is written through, and the test requires that exactly one setting changed and that it was the one the key names. Pointing `guard.context.confirm` at the critical threshold fails it with both names in the message.

The lesson is the same one as the privacy tests in 23 and the adapter parity in 25. A list maintained by hand alongside a type needs a test that derives one from the other, or it is correct only until the next person forgets.

What this buys beyond tidiness: uninstall and configuration migration both need to walk every setting, and neither can now be written against a list that is out of date.

## 30. The credential patterns are compiled when one is needed

Date: 2026-09-14

Finding: this executable starts fresh for every hook call, including every status line repaint, and the credential scanner compiled all of its expressions at package load. Thirty detectors, and thirty more loose copies of them built for redaction, plus the placeholder expression. A status line scans nothing and redacts nothing, and it was paying for all of them.

Measured at about a quarter of a millisecond, which was roughly eight percent of both startup and the status line on the machine this was found on. Small in isolation, and paid on every repaint of a line that exists to be unobtrusive.

Decision: a detector holds its pattern as text and compiles it the first time it is actually reached. What makes this more than a deferral is the anchor check that was already there: a literal string has to appear in the line before the expression is built at all, so even a real scan of an ordinary file usually compiles none of them. Redaction builds its loose copies on first use for the same reason, and the placeholder expression is only reached after a detector with a value group has matched.

The measurement is recorded in `docs/benchmarks.md` with its own conditions rather than folded into the table there, which was taken on a different machine with a different Go release. The two are not comparable, and presenting them together would suggest an improvement that was mostly a change of toolchain.

The tests caught the mistake in the first attempt, which is worth recording. Redaction reaches its expression in two branches, and only one of them was changed, so the other dereferenced a pattern that had never been compiled and the package panicked. A nil pointer is the failure this shape invites, and the existing corpus test found it immediately.

No behaviour changed. The same expressions match the same content in the same order, and every existing test in the package passes unaltered.

## 31. The guard has no measurements outside a terminal

Date: 2026-09-14

Finding: context use, the five hour and seven day windows, and session duration reach ZeroTurn through the harness status line and through nothing else. Decision 21 established that no hook payload carries any of them. An editor extension draws no status line, so it never invokes the command, and the guard runs with no measurements while every other sign says it is installed correctly.

This was found by reading the records rather than by reasoning. Every session stored on this machine, and the long build session carried over from the previous one, held a harness name, subagent counts, and credential warnings, and held no context value, no window value, no duration, and no model. Seven sessions across two machines, including the longest one this project has had, and not one measurement.

The code was never at fault. Fed a real status line payload it records everything, which was confirmed directly. The same integration then ran in a terminal and produced context 4 percent, five hour 24 percent, seven day 3 percent, a window size of one million, and the five hour reset time the rate projection needs. The difference is the interface, not the installation.

What this means is worth stating plainly rather than softening. In an extension the subagent gate can only ever act on counts ZeroTurn keeps itself, the rate projection can never fire, and thresholds on context, the windows, and duration can never be crossed. The hooks still work, so the credential guard still reads files and subagents are still counted, which is exactly why the failure is silent.

Decision: say it where someone decides whether to install, and detect it rather than describe it. The README carries it in the installation section and in the list of limits, the integration document explains it under troubleshooting, and `doctor` gains a `session data` check that compares the sessions it has recorded: every session without measurements is a failure, some without is a warning, because working in both places is the case that hides the problem behind the sessions that did work.

On this machine that check now reports six of seven recent sessions carried nothing, which is an accurate description of three days of dogfooding that measured nothing.

The lesson is the one the project already wrote down and did not apply to itself. Stop condition four in the phase 1 research was that the platform interfaces might be missing, and it was tested carefully for Copilot and taken on trust for Claude Code outside a terminal. An interface that is present in one interface of a harness is not present in all of them, and the way to find out is to read what was actually recorded.

## 32. The approval prompt was observed, and it can be switched off from inside itself

Date: 2026-09-14

The last unverified claim in this project is closed. Confirm mode had been accepted by the harness and refused correctly in an unattended run, but nobody had watched a person be asked, so the README said so rather than claiming the mode worked end to end.

It was observed on Claude Code 2.1.270, in a terminal, with the five hour threshold set to 20 and the seven day threshold to 1 so that the real measurements would cross them. The harness showed:

```
Hook PreToolUse:Agent requires confirmation for this tool:
New subagent requires approval. Five hour usage is 29% and seven day usage is 4%.
```

The reason is the sentence ZeroTurn built, carried through unchanged. Refusing it stopped the subagent: nothing started. The stored record afterwards held one confirm request, one gate outcome with the decision, the two thresholds that produced it and the measurements behind them, an approval flag still false, and no subagent start. That is exactly what `policy tune` needs in order to learn that a threshold asked and the answer was no.

What the same prompt also showed is worth more than the confirmation. Under the two ordinary answers the harness offers a third: stop asking for this tool in this directory. One keypress turns the gate off for every future subagent there, permanently, and ZeroTurn is never told. Nothing in the record distinguishes a session where the gate was never crossed from one where it was switched off, and a report would show no asks in both cases.

Decision: say so. The README carries it beside the compatibility result and in the list of limits, and the integration document records the ask row as observed. The honest description of the gate is that it asks once and can be dismissed forever from inside its own prompt.

This also settles what the gate is for. A guard that a person can disable in one keypress, at the exact moment it is inconvenient, is not a budget. It is a prompt to notice something, and it is worth what noticing is worth.

Detecting the bypass was considered and not built. The harness records the exemption in its own settings, and reading them to report it is possible, but the shape of that rule was not observed here and guessing it would produce a check that quietly never fires, which is the failure this project has now made twice.

## 33. A description beside a thing will drift, so derive it

Date: 2026-09-14

Six defects in this project have now had the same shape, and it took the sixth to see it. A description of something was kept by hand beside the thing it described, and the two came apart: the permitted fields beside the record type, the adapter shape beside the event, the policy keys in three copies beside the configuration struct, the counts in the portfolio document beside the repository, the platform claims in the README beside what the harness delivers, and the required checks in the branch ruleset beside the jobs that produce them.

Each was recorded as its own lesson, in decisions 23, 24, 25, 29, 31 and in the pre flight read before publishing. Together they are one lesson, and it is now written as a rule in the contributing guide: when a change names or counts something in a second place, derive the second from the first in a test rather than maintaining both.

Nine tests here do that. They read a type by reflection, compare one implementation of a contract against the other, or count what is on disk, and they fail in both directions so that neither a missing entry nor a stale one survives.

The evidence that it works is the split between what was caught and how. Drift with a derived check was caught automatically: three unlisted fields in the stored record, a contract version no adapter could send, help broken on seven commands, and the counts in the portfolio document twice inside an hour of the check being written. Drift without one was caught by luck: a test count out by roughly a hundred, platform claims in three documents, and a branch rule requiring a check that a change in the same pull request had just removed, which would have blocked every merge to main once it was applied.

What makes this worth recording rather than filing as ordinary hygiene is what changed about who writes the code. A person drifts slowly enough for review to catch it. In one afternoon here, an agent left three documents describing the previous continuous integration matrix, made the count in the portfolio document wrong twice, and invalidated a branch rule by renaming the jobs it required. None of it was careless. An agent holds no memory of the parallel lists in a repository and makes more changes in a day than a person, so every hand maintained description rots faster and is read less often than it used to be.

A derived check is the only kind that cannot be forgotten, because there is nothing to remember.

## 34. A release removes only the lock it took

Date: 2026-09-14

The architecture review recorded two faults in the state lock and fixed neither. Both are fixed here.

The first is the serious one. The release closure removed whatever lock file was present, not the one the caller had taken. A holder judged stale has its lock removed and another process takes over, and when the first one finishes it deletes the second one's lock. A third process then acquires a lock two others believe they hold, and the mutual exclusion the file exists to provide is gone.

Each acquisition now writes a token, the process identifier with the time it acquired, and the release reads the file and removes it only while that token is still there. The process identifier alone would not do, because it is reused.

The second is that the staleness limit was longer than the deadline for acquiring. A lock left by a process that died two seconds ago could not be cleared by the next invocation, which waited out its five seconds and gave up on a lock it was entitled to take. The limit is two seconds now, shorter than the deadline, so one invocation recovers. A hook holds the lock for about a millisecond, so that is three orders of magnitude of headroom, and clearing one early is now safe in a way it was not before: the holder it was taken from can no longer delete the new lock when it returns, and every write is atomic, so the worst case is a lost update rather than a damaged record.

Both tests were confirmed to fail first. Against the old release the first reports that the first holder removed the second holder's lock. Against the old limit the second reports that the staleness limit is not shorter than the deadline.
