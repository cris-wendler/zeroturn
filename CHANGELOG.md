# Changelog

Notable changes, newest first. The project follows semantic versioning from the first release.

## Unreleased

**Added**

- ZeroTurn installs as a Claude Code plugin: `/plugin marketplace add cris-wendler/zeroturn`. It carries the hooks and not the status line, because a plugin cannot contribute one, so it is a second install path rather than a replacement for `integrate`. The plugin does not bundle the executable either, and its hooks call `zeroturn` by name. `integrations/claude-plugin/hooks/hooks.json` is generated from the hook list and a test fails if the committed file is not what the generator produces.
- `status --json` and `policy check --json` carry `unmeasured`, the thresholds the gate could not check because the harness sent no reading for them. `triggers` still means the thresholds that were crossed, so a reader counting it to ask whether anything fired gets the answer it has always got. Every entry in `triggers` has `available` true and every entry in `unmeasured` has it false, which is what that field was declared for and never carried: nothing ever set it false. The contract version is 2.2.0 and nothing was taken away.

**Fixed**

- `zeroturn init` chose a package manager by ranging over a map, and ranging over a map is randomised in Go, so a repository holding more than one lockfile got a different proposal on different runs of the same command against the same files. The lockfiles are read in a fixed order now, most specific first.
- `schemas/policy-tune.schema.json` advertised a trigger named `backgroundTasks`, which `policy tune` cannot produce: a threshold is only reported for a measurement the tuner can read, and that is not one of them. An adapter reading the published contract would have written handling for a value that can never arrive. No output changes, because the value was never emitted.
- Cancelling `verify` or `ship` could wait for the whole step anyway. When the step's first process had already been reaped, the process group was not signalled, and what it had started kept the output pipe open until it finished by itself. The group is signalled by its id now, which does not depend on the first process still existing.

**Internal**
- The packages the mutation guard names in the workflow are compared with the packages that are there, so a group cannot name one that has been renamed or moved, and no package sits in two groups.
- Two hand written insertion sorts are `sort.Strings`. One of them sat in a file that already imported `sort`.
- `report purge --all` has a test that it removes evidence records and leaves everything else in the directory, and the installer has one that a repair rewrites ZeroTurn's command inside an entry it shares with somebody else's.
- The five copies of "use git's message, or this one when git printed nothing" in `internal/git` are one function, which is tested directly rather than through five wrappers that cannot make git fail quietly.
- The mutation guard covers `internal/verify`, `internal/settings`, `internal/git`, `internal/status`, `internal/trust`, `internal/jsonschema`, `internal/harness/claude`, `internal/snapshot` and `internal/evidence`, which brings it to fifteen packages. It runs as three groups now, so its wall time is the slowest group rather than the sum. It found that the log writer could stop after the first line with every test still passing, that the characters at each end of an allowed range in a step name were never exercised, and that a settings rewrite had no test for where a new key lands.
- The mutation guard skips a file this platform does not build. Every change to `proc_windows.go` was reported as unnoticed on Linux and macOS, where nothing compiles it.
- `topLevelOrder` tracked nesting it can never see. The value after each key is decoded whole, so the only delimiter reaching that loop is the brace that closes the document, and two branches were unreachable.
- The step statuses in `schemas/verify.schema.json` and `schemas/evidence.schema.json`, and the assessment words in `schemas/report.schema.json`, are compared with the constants the code publishes them from, in both directions. Those were the last lists in the published contract that nothing compared with the code.
- The trigger names in `schemas/status.schema.json` and `schemas/policy-check.schema.json` are compared with the names the gate can emit, read out of the source, in both directions. They were the last two copies of that list kept by hand, and the same list in `schemas/policy-tune.schema.json` had already drifted.

- The exit code table in the README is compared with the codes the program publishes, which are already compared with the constants themselves, so the three copies form a chain rather than three opinions.
- The trigger names in `schemas/policy-tune.schema.json` are compared with the names the code can emit, read out of the switch that decides them, in both directions.
- `.zeroturn.example.json` is compared with `config.Default()` rather than only checked for loading, so the file somebody reads to see what the defaults are cannot quietly stop showing them. The validation steps are excluded and held by their own check, because they illustrate a JavaScript project on purpose.

## 0.3.0, 2026-09-17

**Added**

- `zeroturn verify` records what state the repository was in when the checks ran, and `zeroturn report` says whether that answer still covers the code on disk. The state is a digest over content: the commit, the blob hash Git holds for everything staged, the contents of every path the working tree disagrees with Git about, and the definition of the steps themselves. A result and whether it is current are separate, so a failure that has gone stale still reports as a failure. One record per repository, replaced by the next run. Step output and file contents are not stored. The contract version is 2.1.0 for this, and the addition is additive.
- `doctor` says whether the executable can be run by name. `go install` puts it in `GOBIN`, nothing guarantees `GOBIN` is on `PATH`, and every instruction in the README names the command that way. Installed and not reachable names the line to add, with the directory filled in. Two installs where typing the name runs the other one is a warning. A build run from the source with `go run` is a note, because nothing was installed.
- `report purge --all` removes the evidence record along with the session records, and `uninstall` removes it with the state directory.

**Fixed**

- `doctor` printed whatever a harness wrote to standard output as that harness's version. A program asked for its version answered `Install GitHub Copilot CLI? ['y/N']`, an installer offering to install the thing being looked for, and that went into the table as the version with an ok beside it. It claimed a harness was present when what was present was something offering to install it, and it put a question to a reader with no way to answer. A version has to name a number now, and anything else is reported as found without a version.
- `policy check` printed "No threshold has been crossed" whether the values behind those thresholds had been read and found below the limit or had never arrived at all. The two are the same sentence, and the second is the ordinary case in an editor extension, where the status line is never drawn and context, both usage windows, and session duration are all absent. It now names what was not measured, and says so plainly when nothing was.
- `doctor` reported the status line as delivering measurements when it had sent a context value and neither usage window. `rate_limits` arriving as null is a real payload, not a broken one, and the check counted only the context value, so a gate with two thirds of its thresholds unreachable looked healthy. It now names what is missing from the most recent session.
- `doctor` said "most recent" of whichever record happened to be listed last. Records are not listed in time order, so it now compares them.
- `verify` and `ship` printed nothing while a step ran, because a result only exists once the step has finished. On a repository where the tests take a minute that was indistinguishable from a program that had hung. A step now announces itself before it is waited on, and only where ZeroTurn already writes escape sequences, so a pipe, a log, and the JSON output are unchanged.
- Result lines ended in trailing whitespace. `SKIP` and `STOP` padded the step name and wrote nothing after it, in piped output as well.
- `capabilities` and `verify` read the raw build version rather than the one the executable reports, so a published release described itself as a development build in the document adapter authors are told to read, and stamped that version into every evidence record. Both now read it the same way `version` does.
- `integrate` answered a directory that is not a Git repository with advice about the home directory and the exit code for an internal failure. It now answers it the way every other command does.
- `integrate --help` called the Copilot integration experimental and partial while the README and `capabilities` both said no adapter ships. Running it then advised `capabilities --json`, which reports the same refusal and installs nothing.
- `doctor` warned that it was not inside a Git repository and named nothing to do about it. The rule that every warning names a command was only ever checked inside a repository, so this one escaped it.
- `capabilities` published one tested harness release after a second had been tested, so the contract named an older version than the README did.
- A confirmation that nothing answered was reported as a refusal. `/dev/null` is a character device, so it passed the terminal check, and reading it ends at once: every command that asks before acting printed a prompt into nothing and then said the person had declined, advising them to run it again against the same silent input. All nine now say that nothing answered and name the one thing that works.
- `doctor` reported the `claude` and `copilot` checks as notes on the assumption that neither was installed, and failed for anyone who has Claude Code on their `PATH`.
- A record from an unknown schema version was read as current, and a file the record store does not own could be read as a record.
- A well formed credential made of few distinct characters could be taken for a template and left in a log.
- `policy tune` had no order for a tie, so the same observations could be reported in different orders.

**Internal**

- `scripts/mutate` covers six packages in continuous integration: `policy`, `session`, `security`, `tune`, `config`, `state`. Every change that survives is recorded in `scripts/mutate/accepted` with the reason it alters nothing.
- A newer push on a branch cancels the older continuous integration run. The default branch is left to finish. The mutation job is skipped when only markdown changed, and the test matrix is not, because this project tests its documentation against its code.
- The test for the `PATH` check judged every answer that was not ok as though it were a warning, so it demanded an action from a note, which exists for answers that have none. It failed only on a machine with ZeroTurn installed and on `PATH`, which no continuous integration runner is, so it passed everywhere it ran and failed for somebody running the suite on their own machine. Warnings and notes are held to their own rules now, and a second test applies both to every answer the check can give.

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
