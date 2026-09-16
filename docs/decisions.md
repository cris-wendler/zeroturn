# Decisions

Each entry records what was decided, the evidence behind it, and what it changes.

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

Evidence: ZeroTurnaround publishes libraries named `zt-zip`, `zt-exec`, and `zt-process-killer`, and `zt` is occupied on npm and PyPI. See entry 1, question 5.

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

Decision: the claim is corrected here with the file each capability was verified in, and an adapter moves from refused to planned. What ZeroTurn says about Copilot support stays at nothing until an adapter exists and has been run against a real session, because the rule is that support is claimed only where it has been observed.

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

Decision: the adapter is worth writing, and it uses two entry points rather than one, a `preToolUse` hook for the gate and the status line process for pressure. The findings are recorded in this entry with the file each one came from. What ZeroTurn claims about Copilot stays at nothing until an adapter has run against a real session, which is the same bar the Claude Code gate had to clear.

Two spellings would have failed silently if an adapter had guessed them: the prompt event is `userPromptSubmitted`, not `userPromptSubmit`, and the end of turn event is `agentStop`, with no plain `stop`.

The lesson, again: the answer was in the binary, not in the schemas. Entry 19 said a capability check has a shelf life. This one adds that a capability check has a depth, and stopping at the published schema is not the bottom.

## 22. The state lock answers, whatever is wrong with the directory

Date: 2026-09-12

Finding: an architecture review found that `Lock` could not fail. Its retry loop treated every failure to create the lock file as a lock somebody else was holding. When the entry looked stale it removed it and repeated, without checking the deadline and without sleeping. If the removal could not succeed, the loop spun on a processor and never ended.

It was reproduced through the real executable, not only in a test. With `state.lock` present as a directory rather than a file, `zeroturn event` ran at one whole processor and never exited. A read only state directory did the same. That contradicts the fail open rule the contract publishes, which promises that an unreadable state directory ends in silence and exit 0. A hook that never returns is worse than one that fails: it holds the harness until its timeout and then keeps burning a processor after it.

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

The measurement is recorded with its own conditions rather than folded into the table there, which was taken on a different machine with a different Go release. The two are not comparable, and presenting them together would suggest an improvement that was mostly a change of toolchain.

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

## 35. The guard scans the file it opened, not the path it was given

Date: 2026-09-14

Second of the four defects the architecture review recorded without fixing. The credential guard checked the path with one call and then read it with another. Between the two the name can refer to a different file, and what would be scanned then is not what was checked: a file above the limit is read in full, or a device is opened and the read never ends, which hangs the developer waiting for it.

Decision: the reading moves into `scanTarget`, which opens the file once and takes the size and the kind back from the handle the content comes from. The read is bounded as well, whatever the size claimed, because a size is a statement about the past. The cheap check by path stays in front of it, because only an ordinary file should be opened at all: a device reports a size of zero and then never reaches the end, and a pipe blocks on being opened.

The race itself is not tested, and pretending otherwise would be worse than saying so. Winning it on demand needs the file replaced between two calls that are microseconds apart, and a test that tries would pass whether the fix were there or not. What is tested is the shape the fix gives the code: an ordinary file is read, a directory and a missing file and a file over the limit are refused, a device is refused before it is opened, and a file of exactly the limit is read in full.

That last test is in the set because of a mistake worth recording. It began as an assertion that no more than the limit is ever read, which could not fail: a file of exactly the limit reads identically with the bound and without it. It was rewritten to check the boundary it actually exercises, and rejecting a file at the limit rather than above it now fails it. The rule from decision 23 applies to the tests written for a fix as much as to the ones a fix repairs.

## 36. Two sessions can never share a record

Date: 2026-09-14

Third of the four defects the architecture review recorded without fixing. A session identifier that was missing, or that held a character the file name could not carry, became a record shared with every other one like it, and the counts of unrelated sessions added up into a session that never happened. The gate then asked or allowed on numbers belonging to work it had nothing to do with.

There were two ways in. An event with no identifier stored itself as `unknown`, and every other such event joined it. An identifier with an unusable character had it replaced by an underscore, so `a/b`, `a_b`, `a:b` and `a b` all named the same file.

Decision: it is refused at all three layers it can enter through. Both parsers reject an event that does not name its session, the store refuses to write a record for one, and the file name keeps a short digest of the original identifier whenever the name had to be changed at all, inside the same length bound as before.

The first of those closed a drift of the kind decision 33 describes. The published schema has listed `sessionId` as required since it was written, and the normalized parser checked the contract version, the event type and the harness name, but never that one. The description and the thing it described had been apart from the beginning.

Both tests were confirmed to fail first. Without the digest, four different identifiers report that they all become `a_b`. Without the refusal, a record is written for an event that names no session at all.

Two test payloads had to gain a session identifier to keep passing. They were written without one, which the contract never allowed, and no parser had been enforcing it.

## 37. The check that proves the modes work is itself checked

Date: 2026-09-14

Last of the four defects the architecture review recorded without fixing, and the one whose shape matters most.

`zeroturn doctor --compat` exists to tell somebody that each guard mode still produces the decision it claims. It is the evidence a person is offered when they ask whether the gate works. Nothing checked that it still checks. A version of it that quietly stopped asserting anything would have reported success to everybody who ran it, and the louder the claim it makes, the worse that is.

Decision: three tests. The first requires the three modes and the redaction check to be reported, each passing, each naming the decision it produced. The second requires the live test to say it was not run, rather than leaving a reader to believe the harness was asked something. The third requires the flag to add checks the ordinary run does not have, because a `--compat` returning only the ordinary checks would look like it passed.

Confirmed to fail first. A `compatFixtures` that returns nothing reports that every mode is no longer named.

This is the same rule as decisions 23 and 33, applied one level up. A test that asserts something must be shown failing, and that includes a test whose whole purpose is to assert that other things work.

## 38. A lock token cannot be built from the clock

Date: 2026-09-14

Decision 34 gave each acquisition of the state lock a token, so that a release removes only the lock it took. The token was the process identifier and the time in nanoseconds. That is not unique.

Windows reports time in coarse steps. Two acquisitions inside one step read the same instant, and since both come from the same process the identifier does not separate them either, so the two tokens are identical. The first holder's release then matches the second holder's lock and deletes it, which is the defect decision 34 was written to remove.

The test written for it did not catch this. It performs the real sequence, and on a machine with a fine grained clock the two tokens differ and it passes. It failed on Windows on the first run after the repository went public, which was also the first continuous integration run in seventeen merges.

Decision: the token carries a counter that increments once per acquisition, and the clock is read through a variable so a test can hold it still. With the clock frozen, a thousand tokens must all differ. Replacing the increment with a plain read fails it by name.

The lesson is the one this project keeps relearning from the same file. Three defects in the state lock have now been found by Windows and by nothing else: a contended lock reported as access denied rather than as an existing file, a lock in the middle of being released appearing neither present nor absent, and now a clock too coarse to separate two acquisitions. A test that depends on timing resolution is a test that passes on the machine it was written on.

## 39. A tool that installs itself has to be able to leave

Date: 2026-09-14

ZeroTurn wrote to four places and could take back one of them. `integrate claude --remove` took its entries out of a single settings file. The per repository policy file, the user wide settings file, the session records, and the trust approvals stayed, and nothing told a person where they were. Trust approvals are the worst of those to leave behind: an approval of Strict mode for a repository is a security decision that would apply again to a later install.

Asking people to read the source to find four paths is not an uninstall. Neither is a command that deletes what it thinks it wrote, because a settings file also holds entries written by the person and by other tools.

Decision: `zeroturn uninstall` surveys every location, prints what it found with a count beside each item, and changes nothing until `--apply`. It edits settings files rather than deleting them, keeping every entry ZeroTurn did not write, and it copies each one to a temporary directory first. It never deletes the executable: a running program cannot be deleted on Windows, and a binary placed by a package manager belongs to that manager, so the plan prints the path and the command instead.

Each location comes from the package that writes there, not from a list in the uninstall code. The test that matters walks the disk after a full install and removal and fails if any file or directory still names ZeroTurn, so a location added later is covered without anyone remembering this file. Four separate breakages were introduced to confirm the tests fail: a removal that deletes nothing, one that deletes settings files whole, a plan that acts without `--apply`, and a declined answer that is ignored.

Adding the command exposed the drift this project keeps finding. A command name is written in four places: the dispatch in `main.go`, the help text, the README table, and the `capabilities` document an adapter reads. Nothing compared them. The capabilities list was the one that mattered, because an adapter reads it and a person does not, and it was already missing a command before anyone looked. All three descriptions are now read back against the dispatch switch through the parser, in both directions. The count of tests stated in how-this-was-built.md was stale for the same reason and is now counted from the repository, which is what the paragraph in that document about hand-maintained lists was already saying.

## 40. The test isolation hid the home directory on two platforms out of three

Date: 2026-09-14

`testutil.Isolate` exists so that no test can read or change the settings of the person running it. It set `HOME`. `os.UserHomeDir` reads `HOME` on Linux and macOS and `USERPROFILE` on Windows, so on Windows it hid nothing, and every test that reached the user wide settings file wrote into the real home directory of whoever ran the suite.

This had been true since the isolation was written. It surfaced only when the uninstall work added the first test to install a user wide integration, and it surfaced on Windows continuous integration, which is now the fourth defect in this repository found there and nowhere else.

Decision: `Isolate` sets both names, and a test in `internal/testutil` checks that the home directory after `Isolate` is not the real one and that both names agree with it. That test runs on every platform, so the next variable of this kind is caught on a developer's machine rather than by a lucky run on a runner. Removing either line fails it by name.

The lesson is narrower than the Windows lock defects and worth separating from them. Those were about timing. This one is about a single value having two names on different platforms, where reading one of them succeeds everywhere and is correct in only some places.

## 41. A configuration file is worth more than the format it is written in

Date: 2026-09-14

`.zeroturn.json` declares a version, and any value other than 1 was refused with this advice:

> run zeroturn init to write a supported file, or set version to 1

`init` overwrites the file. The advice for a file ZeroTurn could not read was to destroy the policy in it, which is the one action a person cannot undo. It was also given for a file written by a *newer* ZeroTurn, where the answer is to upgrade the program, and for a file with no version at all, which is what a fragment somebody typed by hand looks like.

There was no way forward either. The first change to the schema would have made every existing file an error. What stood in for migration was three cases written by hand in `Load`, filling in `credentials.mode`, `credentials.prompts` and `limits.projection` when they were empty. A fourth setting added later would have loaded as zero and been refused as out of range, and nobody would have found out until somebody upgraded.

Decision: `Migrate` decodes the file on top of the defaults rather than on top of a zero value. That one change is the whole migration: a setting the file does not carry keeps the value a new file would have, for every setting there is and every setting there will be, with nothing to remember. It reports what it filled and what it did not recognise, and refuses only a file from a newer build, by a distinct error type, so the advice can be to upgrade rather than to overwrite.

Two things this separates that were the same before. Absent and empty: a missing string and `""` both read as empty in Go, so a mode somebody typed wrong used to become the default silently; now absence is a default and an empty value is refused. And unknown settings: at the current version one is a spelling mistake and is refused by name, while in an older file it is a setting a later version removed, so migration drops it after saying so.

`zeroturn policy migrate` writes the file in the current format, with `--plan` to see it first. It is never required: a file is read whether or not it has been migrated. It exists so a file can be made to say what ZeroTurn is already reading from it.

The test that holds this walks the key registry. Every setting, one at a time, is removed from a complete file, and the file must still load with that setting at its default. Decoding into a zero value instead of the defaults fails it for twelve settings at once.

## 42. A longer report window needed a shorter lie fixed first

Date: 2026-09-14

The build pipeline asked for richer report windows. `report` offered `current`, `day` and `week`. Adding `month` looked like three lines.

It was not buildable. Session records older than seven days are deleted when a new session is first seen, so `week` was already the entire history. A thirty day window would have printed a heading over at most seven days of data, which is the same class of overstatement this project has twice called a defect in its own documents.

Seven days was a deliberate privacy choice, so raising it quietly was not available either. How long a record of your own work survives on your own machine belongs to the person whose machine it is.

Decision: retention becomes a setting, `report.retentionDays`, defaulting to the seven days it has always been, so nothing changes for anyone who does not ask. The store is told the value before it writes, because the automatic purge runs deep inside a write where the configuration is not in hand. A test fixes the two defaults together, so the number a new file holds and the number the store falls back to cannot drift apart.

`month` and `--since` follow, `--since` reading a date, a number of days, or a duration. Every report that reaches back further than the records that survive now says so, and says how to change it. Saying it on every report would be noise, so it is said only when the window is longer than the retention. Both halves of that were shown failing.

The published report gains `windowStart`. A document saying `"window": "week"` never said when the week began, and a span given with `--since` has no name at all.

Three things this turned up, all the same shape as decision 33. The window names were written out in four places and only one of them was the code. The key registry described guard settings only, so a setting outside the guard had nowhere to live; it now describes the whole configuration, which also gave `git.remote` a name it can be set by for the first time. And `config.schema.json` was a second description of the type that nothing compared with it: the conformance suite validates the files in this repository against the schema, which catches a missing property only once a file on disk carries it. All three now read the code and compare, in both directions.

`policy show --json` prints the whole configuration rather than the guard alone. A document holding only the guard would have claimed to be the policy while a setting sat outside it. This is the shape `config.schema.json` already described.

## 43. A warning nobody can act on teaches people to skip warnings

Date: 2026-09-14

The last item in the build pipeline was a quieter first run. Nothing had measured it, so the first step was to install ZeroTurn in an empty home directory and an empty repository and run what a new reader runs.

Two things were wrong, and neither was the amount of output as such.

`zeroturn doctor` on a machine with nothing wrong with it reported four warnings out of seven checks. Three of them could not be acted on: the Claude CLI not being on PATH, the Copilot CLI not being on PATH, and an explanation of why no `zt` alias is installed. That last one can never become anything else, however long you use the tool. A reader who finds that three of four warnings are not worth reading has been taught to skip the fourth, which was the only one that mattered: no configuration yet.

The harness warnings were also wrong on the facts. A harness invokes ZeroTurn, never the other way round, so a harness that is not on PATH changes nothing about whether the integration works. On the machine this was written on, the Claude CLI is not on PATH and everything works.

`zeroturn init --yes` printed seventy two lines, fifty five of them the configuration file it was about to write. `--yes` means do not ask me. Printing the file is how a person reads what they are approving, and with nothing to approve it is the answer to a question nobody asked.

Decision: doctor gains a third level. A warning is something the reader can act on, and a note is something true that they cannot. The two harness checks and the alias explanation become notes, and the harness note says why it does not matter. `init --yes` prints what it detected and what it wrote, and not the file.

The rule is now a test rather than a habit: every warning doctor prints must name a ZeroTurn command to run. It found one more straight away, a warning that ended "run this again" instead of naming the command. Measured afterwards, a first run went from four warnings to one, and `init --yes` from seventy two lines to fifteen.

The general form is worth keeping. A tool that reports everything it noticed at the same severity has not saved the reader any work; it has moved the sorting to them and called it transparency.

## 44. An audit aimed at finding, rather than at building

Date: 2026-09-14

Every defect in the last three sessions was found while building something else. Five were already there. That is a poor way to find defects, so this pass went looking, with the hypotheses taken from the shapes this project has already been bitten by: drift, tests that cannot fail, platform assumptions, and output claiming more than the data supports.

Two things were clean and are worth recording as such. The suite passes under the race detector, and it passes under three shuffled orderings, so no test depends on another's leftovers. Neither was run by anything before, so both now run on one continuous integration job rather than by hand.

What the pass found in the first batch:

`integrations/claude/settings.example.json` was missing the credential guard. The integration document tells a reader to copy the entries out of that file and replace the path, and the same document promises a `PreToolUse` entry with matcher `Read`. The file had five entries; the installer writes six. Anyone who installed by hand lost the credential guard with nothing to tell them, and no Go code had ever opened the file. It is now compared with `claude.Hooks` in both directions.

The README's picture of the file `init` writes had lost a section. `report` was added to the configuration earlier the same day, and the same document tells a reader to change `report.retentionDays` in a file its own depiction did not contain. The picture is now compared with the `Config` type by reflection, and is also parsed and validated, so a block a reader copies has to be one ZeroTurn would accept.

The rule written a few hours earlier, that every warning `doctor` prints must name a command to run, was enforced only on the checks that run without `--compat`. The `--compat` checks were never held to it and one of them said "Add --live" without naming the command that carries the flag. The rule now covers both.

The lesson is not any one of these. It is that a pass aimed at finding, with the hypotheses written down first, turned up in one sitting more than three sessions of building had.

## 45. What the audit found wrong, rather than merely undescribed

Date: 2026-09-14

The audit's first batch was drift. This is the half that made ZeroTurn give a wrong answer or not work at all. Each was reproduced before it was fixed, and each test was shown failing.

`zeroturn report` counted every repository on the machine. Records are stored for the machine, and `status`, `doctor` and `policy tune` all narrow them to the repository they were run in. `report` did not, and nothing said so. Reproduced by planting one session record belonging to another repository and running `report day` inside a repository with no sessions of its own: it reported that session and its nine subagent starts. The retention line added to the same command hours earlier made it worse, printing "This repository keeps records for 7 days" under a machine wide count.

A report now covers the repository it was run in. Outside a repository there is nothing to narrow to, so it covers the machine and the heading says which. `--all-repositories` asks for the machine deliberately, and the published document carries a `scope` field, because a reader who cannot tell a quiet repository from a busy machine has been given a number and no way to read it.

`verify` and `ship` did not run at all in a linked worktree or a submodule. In both, `.git` is a file holding the path of the real Git directory, and the log directory was built as `<root>/.git/zeroturn/logs`. Creating it failed with "not a directory", and the message a person saw said the `.git` directory was not writable, which sends them to look at permissions. The Git directory is now resolved by reading the one line `gitdir:` form, which is what Git itself writes; a test checks that belief against Git rather than against the documentation of it.

The Gradle wrapper step could never run, on any platform. `filepath.Join(".", "gradlew")` cleans the `.` away and returns `gradlew`, so `exec.LookPath` searched `PATH`, never found the wrapper in the repository, and the step was dropped every time. A project that ships a wrapper means the wrapper to be used, because it pins the build tool version. The test that existed asserted the broken value. Wrappers are now named by their absolute path, the Maven wrapper is honoured the same way, and on Windows the `.bat` is chosen because the extensionless file beside it is a shell script.

`zeroturn uninstall`, shipped the same day, left the validation logs behind. They live in the repository's Git directory rather than in the state directory, so a removal that looked only where ZeroTurn keeps its own records reported a clean machine and was wrong. The package comment already stated the rule this broke: every location comes from the package that writes there. `internal/verify` was the writing package nobody had wired in.

One finding from the audit is recorded and not fixed. `policy.Trigger` publishes an `Available` field, documented as false when the harness did not supply the measurement, and it is never set to false anywhere. It is a required property of two published schemas that can only ever be true. Removing it takes something away from a contract that has so far only added; implementing it would mean putting entries in the trigger list for thresholds that were not crossed, which changes what every reader of that list is looking at, including the sentence the gate shows a developer. It is the same question as telling "nothing happened" apart from "nothing was measured", which is worth answering properly rather than in passing.

## 46. Twelve tests that could not fail, and something to find the next one

Date: 2026-09-14

This project's oldest written rule is that a test asserting an absence has to be shown failing before it is relied on. An audit aimed at that rule broke the code deliberately, one behaviour at a time, and ran the tests. Twelve tests passed with the behaviour they name removed. Six survived the whole suite: six things could be deleted outright and `go test ./...` stayed green.

The shapes were the same few, repeated:

A helper that decided the answer. The status line colour test built its own disabled colour and then checked that nothing was coloured, so it proved a disabled colour stays disabled and never called the function that decides. Terminal detection could be replaced with "always colour" and nothing noticed, which means escape codes in a pipe or a log file.

An expectation derived from the thing it checks. The integration test built its list of hooks by walking the table the installer installs from, so deleting a hook removed it from both sides. The window help text test, written the same day as this audit, compared a generated string with the list it was generated from.

An assertion satisfied by failure. `RemoteDisplay` returns the bare remote name when the Git call fails, and the test only asked that the result not contain the password. Breaking the Git call passed. The trust record test discarded the read error, so a record that was never written passed.

An assertion the data never reached. The hostile input test checked that no finding carries the secret value, and no hostile case produced a finding at all. The tool filter test used a fixture with no measurements, so the session crossed nothing and Strict allowed it whatever the filter did.

A claim not observable where it was made. "Missing measurements are never treated as zero" cannot fail inside `internal/policy`: a threshold is a percentage of at least 1, so a zero reading is below every one of them and produces the same empty trigger list an absent reading does. The claim is real and belongs where the value is shown, so it moved to `internal/status`, where reading absent as zero prints `ctx 0%`.

A check that never reached the thing it checked. The schema support test validated one empty document, and the validator only screens keywords in the parts a document exercises, so it screened the root of each schema and nothing else. Four unsupported keywords added to a nested property passed everything. `jsonschema` gained `Unsupported`, which walks the whole tree.

Every one is now shown failing against the same break that exposed it.

The more useful half is `scripts/mutate`. It changes one operator in the source at a time, runs the tests for that package, and reports the changes the tests accept. Finding twelve of these by hand took a session; the point of the tool is the thirteenth. It refuses to run against a dirty working tree, restores the file it edited even on an interrupt, and counts a change that does not compile as noticed rather than as a pass.

Not every survivor is a defect: some operator changes alter nothing at all. Those are recorded in `scripts/mutate/accepted`, one per line with the reason, and an entry the tests later do notice is reported as out of date, so the file cannot quietly excuse a change made afterwards at the same place.

Run against `internal/policy` it made 56 changes and the tests noticed 55. The one it did not notice alters nothing. Getting there took seven new tests, every one of them for a rule written into a condition that no test could disagree with: the minimum span a rate can be measured over, a window that resets exactly now, a window reporting exactly five hours left, a projection landing exactly on the limit, the sentence that separates asking from denying, counting the categories after the first, and lowering the first letter of a clause.

The continuous integration job runs the packages that are clean, and the list grows as packages are brought up to it. Recorded and not yet done, measured on 2026-09-14:

- `internal/security`: seven changes the tests accept, at security.go lines 122, 134, 153 twice, 229, 233 and 237. The last three are inside `Redact`, and one of them is the difference between redacting a value at the start of a line and leaving it there.
- `internal/tune`: nine, at tune.go lines 105, 120, 124, 185, 188, 195, 202, 220 and 230. That matches what a separate pass over this package found: it is the least tested thing that produces a number a person acts on.
- `internal/config`, `internal/state`, `internal/verify`, `internal/status`, `internal/git`, `internal/settings`, `internal/trust`, `internal/jsonschema` and `internal/harness/claude` are not measured yet.

## 47. The documentation was mostly for me

Date: 2026-09-14

Fourteen documents, thirty thousand words, against ten thousand lines of Go. Opening `docs/` on the public repository was the first time anyone had looked at it as a stranger would: an unordered list of filenames with no way to tell which two mattered.

Three of them were working notes wearing a documentation costume. `going-public.md` was a checklist for an event that had already happened, and it opened by saying the repository was private, which it had not been for days. `dogfood.md` was a transcript of a version of the tool that no longer exists: no note level in `doctor`, no scope on a report, no `uninstall`. `architecture-review.md` was a narrative whose every finding was already an entry in this file. Each of those contradicted something, and this project's own rule is that a document contradicting the code is worse than no document.

The rest was a judgement rather than a defect. Publishing documentation is normal and good practice, and decision records are an established one; the first instinct to cut was partly wrong and is recorded here as wrong. What has changed is that disproportionate prose has become a recognised sign of a generated repository, and thirty thousand words wrapped around ten thousand lines reads that way to a reader who has never met the project. That signal, not the practice of publishing, is what the cut was for.

What is left is three documents: this file, `how-this-was-built.md`, and the Claude Code integration. The findings about how a hook based guardrail fails, which are the most useful thing here for anyone building a control on an agent harness, moved into `how-this-was-built.md` rather than being lost. The contract survives as the published schemas and the conformance suite that runs the real executable against them, which is the form an adapter author actually consumes. The research, the benchmark method, the release steps and the Copilot notes left the repository and remain in its history.

Deleting eleven documents left nine dead links behind, in files nobody had touched: the conformance suite's own README, the adapter template, and two of the documents that stayed. Nothing noticed, because nothing checked. A test now walks every Markdown file and fails when a relative link leads nowhere. It was shown failing by adding one link to a document that had just been deleted.

The number worth recording: after removing eleven of fourteen documents, this file is seventy seven percent of what remains. The volume was never spread across the directory. It was always one document, and that document is the one worth keeping.

## 48. The counted noun, fixed for the third time

Date: 2026-09-15

"Result: 1 checks passed" is the last line of `verify`, the most run command in the Direct Lane. It had read that way since the command was written.

This class had already been fixed twice. Once for the gate prompt, which asked for approval with "1 subagents are active and 1 subagents started this session", the first sentence a developer would ever have read from this tool. Once for `doctor`, which reported "1 checks failed". Each fix added a helper, and each helper was used only at the place that prompted it. Eighteen other counted nouns had neither: `ship` saying a branch is "1 commits behind", `report purge` deleting "1 records older than 1 days", `uninstall` reporting "1 of 1 items", `integrate` repointing "1 entries", and five in `policy tune`.

Decision: one helper, `output.Counted`, in the package the whole project already imports for formatting. The singular is written out in full rather than derived, because the verb changes with it: one step did not pass, two steps did not pass. `output.CountedPair` handles the "N of M" shape, where the noun agrees with the second number and that is the one that read as "1 of 1 steps". The copy in `internal/policy` is gone and its three call sites use the shared one.

The fix that matters is not the helper, it is the test. A number followed by a plural noun, in a string a person reads, has to pass through `Counted`. The test parses every non test source file, finds string literals matching that shape, and fails for any that is not an argument to `Counted` or `CountedPair`. It found two the manual pass had missed and one false positive, a verb that ends in s, which is now in a short list of words that are not plural nouns. Putting one string back the way it was fails it by name.

Two other things went with it. `policy tune --days 30` printed a thirty day heading over records that are deleted after seven, which is exactly the defect entry 42 fixed in `report`; the helper written for that was in the file next door and `tune` never called it. And the Claude Code integration document said "Every measurement the guard compares against a threshold arrives that way", which the next sentence in the same paragraph disproved: the subagent counts are compared against thresholds and arrive from hooks. It now names the three measurements that do come from the status line.

## 49. The contract takes something away, so it is 2.0.0

Date: 2026-09-15

`zeroturn policy show --json` printed the guard. A reader took `.mode` from the top level of that document. Entry 42 changed it to print the whole configuration, because retention became a setting outside the guard and a document calling itself the policy while omitting a policy setting is wrong. The same value is now at `.guard.mode`.

That is a change that takes something away, and the contract version stayed at 1.2.0, whose own comment said the versions before it took nothing away. So the comment had become false, which is the thing this project spends most of its time finding.

Decision: the contract is 2.0.0, and the comment on it says what moved and why. The alternatives were worse. Emitting the guard fields at the top level and adding the new sections beside them keeps the number and produces a document that is half a guard and half a configuration. Reverting puts back the defect that a policy document does not contain one of the policy settings.

Nobody is broken by it: no forks, no other installs, and the only consumer is this repository. The cost of pretending otherwise would have been a published number that says a promise was kept when it was not.

Two things went with it. The changelog had no 0.1.0 section: everything was still under "Unreleased, first prototype, nothing has been released yet", four days after the release. The release checklist says to move it and nobody did. And the version a build from a checkout reports was written twice, once as the value and once as the string it is compared with, so bumping one and not the other would have made every development build claim to be a release. It is one constant now.

## 50. A test that passed only on a machine without the harness

Date: 2026-09-15

`TestAMissingHarnessIsANoteAndSaysWhyItDoesNotMatter` asserted that the `claude` and `copilot` checks in `doctor` are both notes. It passed on this machine, and on all four continuous integration runners, because none of them has Claude Code on `PATH`. It failed for anyone who does, which is most people who would run ZeroTurn at all: the check finds the executable, reports its version, and is an ok rather than a note.

It was written the same day, in the change that gave `doctor` a third level, and it encoded the machine it was written on rather than the rule.

The rule does not depend on what is installed. A harness invokes ZeroTurn, never the other way round, so a harness that is absent changes nothing about whether the integration works, and is never worth a warning. The test says that now: found means ok and says something about it, absent means a note that says why it does not matter, and neither is ever a warning. It passes with the harness on `PATH` and without it.

How it was found is the part worth recording. Continuous integration could not find it, because no runner has the harness installed and adding one is not straightforward. It was found by specgap, the evaluation environment in the repository next door, which ran ZeroTurn's own test suite inside an agent workspace with `PATH` set differently. The environment built to look for gaps in how an agent solves a problem found a defect in the project it was pointed at instead.

The first report of it was also wrong, and that is worth recording too. specgap printed `visible 100%, 402 of 403`, which rounded a failure away, and recorded only which hidden tests had failed, so the visible one had no name. Re-running by hand appeared to pass, and it was written off as flaky. It was not flaky. It was deterministic and depended on `PATH`, which differed between the two runs. Both faults in the environment are fixed, and the second run named the test immediately.

## 51. Bringing two packages up to the mutation guard

Date: 2026-09-16

`scripts/mutate` ran in continuous integration against two packages, because those were the two where every change it could make was noticed. `internal/security` had seven changes the tests accepted and `internal/tune` had nine, both recorded with line numbers and neither looked at. Both are now at zero.

The scanner is the one that mattered. Seven of fifteen possible changes to it went unnoticed, in the package that decides whether a credential reaches a model. Two were real:

A value with almost no character variety is treated as a template rather than a secret, on a count of distinct characters. The count it turns on had nothing checking it, so moving the boundary by one changed which values are reported and no test disagreed.

A detector that matches a whole value never asks whether the value looks like a template, because the shape already answered that. `AKIA` followed by sixteen capital As is a well formed key id with three distinct characters in it, and the template rule would throw it away if the template rule applied. Two of the seven changes made it apply. Nothing noticed, and the effect would have been a real key left in a log.

The other five were changes that alter nothing, recorded with the reason: truncating a line at exactly the cap gives the same line, and three bounds sit on a submatch index that is always inside its own match.

A test written the day before is worth naming here. It was written to hold exactly the case above, a value at the start of a line being left in place, and it used a detector whose value group is the whole match. That detector never reaches the code the test was written for. The test passed, the behaviour was untested, and the mutation run said so.

`internal/tune` turned up one thing worth fixing rather than testing. Its report was sorted by how often a threshold was asked about, and nothing ordered a tie, so the same observations could print in a different order on two runs. It is ordered by name within a tie now.

The comparator for that sort is the other thing worth recording. A change from `>` to `>=` makes it invalid, because sort requires that nothing is less than itself, and the order it then produces is undefined. It survived every test, including one written for it that sorted four different starting orders and required the same answer: an invalid comparator still lands on the right order for any particular input. It was named, moved out of the sort call, and asked the question directly. A row is not less than itself.

`internal/config` and `internal/state` have been measured for the first time and have thirty eight between them, which is recorded here and not yet done.
