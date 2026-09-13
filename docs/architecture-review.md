# Architecture review

A review of the whole codebase carried out on 2026-09-12, across four dimensions: package boundaries and dependency design, correctness and resource safety, Go idiom and testability, and test architecture and evolvability.

Findings marked **reproduced** were demonstrated by running the real executable or a test, and the reproduction is quoted. Everything else is a reading of the code.

## Verdict

The architecture is sound, and the defects found are not architectural.

The `internal` packages form an acyclic layering with genuine leaves. `policy` is pure logic over two data packages with the clock supplied by the caller. `events` enforces the privacy promise through the shape of a decoder struct rather than through a filter someone has to remember. No package reaches sideways, no package is a grab bag, and the error layering is right: no `internal` package imports `output`, so the exit code contract is not smeared through the libraries.

What the review found instead is a small number of live defects, one of them serious, and one structural flaw: the command package has become the place where domain logic accumulates, because it is the one place with no boundary defending it.

## What is right, and should not be changed

| Property | Where |
| --- | --- |
| Privacy enforced by type shape, not by a filter | `internal/events`, where `tool_input` declares exactly one field so a subagent prompt cannot be bound even in principle |
| Policy is pure, and the clock is a parameter | `policy.EvaluateAt` |
| The Git restriction is structural | `internal/git` has no reset, stash, rebase, force push, or `--no-verify` implementation, so no code path can reach one |
| No shell anywhere | Every external command is an argument array |
| Exit codes stay in one layer | Only `cmd` maps errors to codes |
| Privacy tested against what is actually on disk | Tests walk the state directory after each kind of event |
| The scanner has a false positive corpus | Lockfiles, checksums, UUIDs, and data URIs that must not be reported |
| Fail open is tested at the level it lives at | The real executable, against a read only state directory and a state directory that is a file |

Two judgements worth recording, because they look like omissions and are not:

**The absence of interfaces is correct.** A store interface, a Git command runner, a filesystem abstraction, and a clock interface were each considered. The tests build real repositories with real Git and a real filesystem, which is how a tool that must never corrupt a repository should be tested. Faking Git would make the tests weaker and the safety properties unverifiable. The clock is already handled the better way, as a `now time.Time` parameter.

**The slow process driven tests are earning their cost.** The product is process behaviour: each hook is a separate process start. Guarantees like "event always exits 0", "allow prints nothing", and "writes are atomic across processes" can only be asserted against a real process.

## Defects

### 1. The state lock could not fail

**Reproduced.** With `state.lock` present as a directory rather than a file, the real executable ran at one whole processor and never exited:

```text
$ zeroturn event --harness claude --event Stop < stop.json
STILL RUNNING after 7s   %CPU 103.0
```

The retry loop treated every failure to create the lock file as a lock another process was holding. When the entry looked stale it removed it and repeated, with no deadline check and no sleep on that path. A removal that could not succeed spun forever.

This contradicted the fail open rule: a hook that never returns holds the harness until its timeout and then keeps burning a processor. Fixed, with [decision 22](decisions.md) recording it.

### 2. Asking for help is an error on every command

**Reproduced.** The top level usage says to run `zeroturn <command> --help`. What happens:

```text
$ zeroturn status --help
Usage of status:                     (the Go default format, not this project's)
  -harness string ...
zeroturn could not read the flags
  reason: flag: help requested
exit=2
```

Seven commands behave this way. `flag.ContinueOnError` returns `flag.ErrHelp` for `--help` and every command treats it as a parse failure. Only `ship`, which is parsed by hand, is correct.

Exit 2 makes it worse than cosmetic. It is `ExitInvalidUsage`, and it is also the code a harness reads as a blocking error.

### 3. The rate projection cannot reach any adapter

`normalized-event.schema.json` declares `additionalProperties: false` and has no reset time fields, so an adapter cannot legally send one. `policy.Project` requires a reset time, so it returns false immediately for every harness except Claude Code.

The contract version was raised to 1.2.0 for a feature the adapter facing half of that contract cannot express. The next roadmap item is an adapter for a second harness, whose purpose is to prove the contract is not shaped around one harness, so this is the shape that item exists to catch.

### 4. A durability decision escaped the place it was reasoned about

There are two atomic write implementations. The one in `state` deliberately does not flush, which is correct and well argued for session records rewritten many times a minute. The one in `config` does flush.

The harness settings file, and its only backup, are written through the one that does not flush.

### 5. The privacy allowlist test cannot detect a new field

**Reproduced.** The test that checks the stored record against a permitted field list builds one sample record and inspects its keys. Almost every field is an omitempty pointer, so a field the sample does not set never appears.

The three fields added by the rate projection are absent from the permitted list, and the test passes. A test that guards the project's central promise has been unable to fail since the first omitempty field was added.

The same pattern appears in an event test asserting that no transcript path survives parsing: no payload in its table carries one.

### 6. Two correctness rules live in the command package as convention

`applyTo` is the domain fold, event into session record. It is the core of the product and it is in `package main`.

`effectiveGuard` enforces that a committed configuration asking for Strict mode is downgraded until the machine approves it, so a repository cannot deny subagents for everyone who clones it. That is a security invariant, and it is applied by remembering to call it at five separate places. `policy.Evaluate` does not know the rule exists. A sixth call site that forgets reopens the hole, and nothing fails to compile.

### 7. The settings file writer is a library inside a command

`cmd_integrate.go` is 836 lines, of which about 330 are order preserving JSON surgery on a file the user owns, with a backup protocol. It is the riskiest write in the product outside Git, it can only be tested by driving the command, and it is already leaking: the doctor command decodes the same file with a third private struct shape and calls three of these functions across the file boundary.

### 8. Error wrapping is unused, and one contract depends on that staying true

There is no `%w`, `errors.Is`, or `errors.As` anywhere in non test code. The entry point inspects the error with a bare type assertion, so the first wrapped error anywhere in a command path silently turns a documented exit code into the internal failure code.

`internal/git` rebuilds every failure as a new error, discarding the exit status. A cancelled command therefore reports as a repository problem: interrupting `ship` says the branch could not be resolved and suggests checking that the repository has a commit.

### 9. Smaller confirmed items

| Finding | Effect |
| --- | --- |
| The preserved verify log drops everything after the last newline | A step whose output has no trailing newline leaves an empty log while the summary still points at it |
| The credential gate checks the file, then opens it by path again | Between the two the path can become a pipe, which blocks the read hook until the harness timeout |
| `ship` scans for credentials before the confirmation prompt and stages after it | A file rewritten while the prompt is open is committed unscanned |
| A missing session identifier becomes a shared record | Counts from unrelated sessions accumulate together and the gate acts on another session's numbers |
| A declined ask can later be recorded as approved | The tuning command learns the opposite of what happened |
| Only one signal is handled | After one interrupt the process cannot be interrupted again |
| `doctor --compat` has no test | It is the evidence for the project's central claim, and it gates enabling Strict mode |
| Exit codes have no test pinning the numbers | Every assertion is symbolic, so renumbering one keeps the suite green and breaks every adapter |

## Measurements

Coverage measured across the module rather than per package, because the end to end tests run a separate binary that carries no instrumentation:

| Area | Covered |
| --- | --- |
| Whole module | 60.4% |
| `internal/security` | 98.4% |
| `internal/trust` | 92.0% |
| `internal/verify` | 91.8% |
| `internal/policy` | 85.3% |
| `internal/state` | 80.9% |
| `internal/git` | 72.0% |
| `internal/events` | 69.9% |
| `internal/output` | 66.0% |

The per package figures reported by `go test -cover` are misleading in the other direction: the gate, the credential guard, and the prompt guard all read as zero while being among the best tested code here, because they are exercised through a separate process.

Cost of one recent change, as a measure of how far a single concept is spread: adding one configuration setting touched 19 files; adding one event type touched 24.

## What this suggests, in order

1. **The lock fix.** Done.
2. **Make the privacy allowlist derive from the type**, so a new field fails until it is permitted deliberately. The guarantee is the reason this project exists.
3. **Fix help, and pin the exit codes with a test.** Both are small, both are user facing, and one of them is a published contract with nothing holding it in place.
4. **Carry reset times in the normalized event**, before the second adapter is written rather than after.
5. **Move the two correctness rules behind types**: the fold into a package of its own, and the Strict downgrade into a guard value that can only be built the safe way.
6. **Extract the settings writer and the tuning analysis** into packages that can be tested directly.
7. **Collapse the three copies of the policy key table** into one registry, which is also what makes configuration migration and uninstall cheap later.

Items 5, 6, and 7 are the extraction the command package needs. They are ordered after the defects on purpose: none of them is a bug today, and all of them are easier once the contract gaps in 3 and 4 are closed.
