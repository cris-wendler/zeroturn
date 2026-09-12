# Dogfood run

ZeroTurn run against its own repository, on 2026-09-11, with the release build described in [benchmarks.md](benchmarks.md).

Everything below is real output. The session values come from `fixtures/claude/statusline-full.json` and are sample data, not a real account. The run used a clone of this repository with a local bare remote, its own Git configuration, and its own ZeroTurn records, so nothing on the machine changed. Long paths are shortened for reading.

Repeat it with the commands shown. Each section is one of the twelve behaviors the plan asked to demonstrate.

## 1 and 2. Session event ingestion and status line

```text
$ zeroturn status --stdin --harness claude < statusline.json
ZT  ctx 42%  5h 81%  7d 47%  session 3h12m  ask
```

The line carries only what the payload contained. The last word says what would happen to the next subagent, here `ask`, because five hour usage is past its threshold.

## 3. Observe mode

```text
$ zeroturn policy set guard.mode observe
guard.mode is now observe

$ zeroturn event --harness claude --event PreToolUse < proposed-subagent.json; echo "exit $?"
exit 0
```

No output at all, even with context at 95%. Silence means allow, so the harness proceeds under the permission rules the developer already set.

## 4. Confirm mode

```text
$ zeroturn event --harness claude --event PreToolUse < proposed-subagent.json
{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask","permissionDecisionReason":"New subagent requires approval. Context is 84% and five hour usage is 81%."}}

$ zeroturn policy check
ZEROTURN POLICY CHECK
mode       confirm
level      confirm
decision   ask
thresholds crossed:
  context          Context is 84% (limit 80)
  fiveHour         Five hour usage is 81% (limit 75)
  activeSubagents  2 subagents are active (limit 2)
```

The reason names two measurements. The rest stays in `policy check`, which changes nothing.

## 5. Strict mode compatibility

The repository file asked for `strict`, and no one had approved Strict on this machine:

```text
$ zeroturn policy show | tail -2
Strict mode denies a new subagent when the critical threshold has been crossed.
Strict mode is requested by .zeroturn.json but has not been approved on this machine, so the gate asks instead of denying. Run zeroturn policy set guard.mode strict to approve it.

$ zeroturn event --harness claude --event PreToolUse < proposed-subagent.json
{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask","permissionDecisionReason":"New subagent requires approval. Context is 95%."}}
```

A committed file cannot deny subagents for whoever clones the repository. That a denial is honoured when Strict is approved was shown separately by `zeroturn doctor --compat --live`, which ran one real session and reported that the harness honoured the denial and no subagent started.

## 6. Subagent counting

Three starts and one stop, from the harness's own events:

```text
$ zeroturn status --stdin --harness claude < statusline.json
ZT  ctx 42%  5h 81%  7d 47%  session 3h12m  agents 2  ask
```

The harness reports no such number. This count is ZeroTurn's own observation, which is why the README says so wherever it appears.

## 7. Configuration trust

```text
$ zeroturn verify; echo "exit $?"
These commands are configured by this repository and have not been approved on this machine:
  vet       "go" "vet" "./..."
  build     "go" "build" "./..."
They run directly, without a shell.
zeroturn verify ran nothing
  reason: this repository has not been approved on this machine
  next:   review the commands above, then run zeroturn verify --approve
exit 6
```

Nothing ran. The commands are shown exactly as they would be executed, one argument per quoted string.

## 8. Validation

```text
$ zeroturn verify --approve   (answered yes at the prompt)
Approved. This approval is withdrawn automatically if the configured commands change.
ZEROTURN VERIFY

PASS  vet        2.4s
PASS  build      0.7s

Result: 2 checks passed in 3.1s
```

## 9. Ship dry run

```text
$ zeroturn ship --message "docs: add dogfood notes" --files DOGFOOD.md --dry-run
ZEROTURN SHIP
files:
  DOGFOOD.md
message:  docs: add dogfood notes
branch:   dogfood
remote:   origin  /path/to/remote.git

PASS  vet        0.5s
PASS  build      0.4s

Result: 2 checks passed in 0.9s

READY TO SHIP
Dry run finished. Nothing was staged, committed, or pushed.
```

## 10. Divergence refusal

With one commit here and one commit on the remote:

```text
$ zeroturn ship --message "docs: notes" --files DOGFOOD.md --dry-run; echo "exit $?"
ZEROTURN SHIP
files:
  DOGFOOD.md
message:  docs: notes
branch:   dogfood
remote:   origin  /path/to/remote.git

zeroturn ship changed nothing
  reason: branch dogfood is 1 ahead and 1 behind origin/dogfood
  next:   reconcile the histories yourself with the merge or rebase you intend, then run zeroturn ship again
exit 3
```

ZeroTurn does not merge, rebase, or force anything. It says what the state is and leaves the choice to the developer.

## 11. Credential refusal

```text
$ zeroturn ship --message "add deploy settings" --files deploy.env --dry-run; echo "exit $?"
Credential indicators were found in the files you asked to ship:
  deploy.env:2  aws access key id
zeroturn ship changed nothing
  reason: the proposed content contains aws access key id
  next:   remove the credential from the file and rotate it, then run zeroturn ship again
exit 1
```

The file, the line, and the category. Never the value.

## 12. Integration install and removal

```text
$ zeroturn integrate claude --plan | head -12
ZEROTURN INTEGRATE CLAUDE (plan)
settings file   /path/to/repo/.claude/settings.local.json
                the file does not exist yet and would be created
backup          /path/to/state/backups/claude-settings-20260911-230936.json

would add:
  statusLine    "/path/to/zeroturn" status --stdin --harness claude
  hooks.PreToolUse     "/path/to/zeroturn" event --harness claude --event PreToolUse (matcher Agent, exact)
  hooks.SubagentStart  "/path/to/zeroturn" event --harness claude --event SubagentStart
  hooks.SubagentStop   "/path/to/zeroturn" event --harness claude --event SubagentStop
  hooks.Stop           "/path/to/zeroturn" event --harness claude --event Stop
  hooks.SessionEnd     "/path/to/zeroturn" event --harness claude --event SessionEnd

$ zeroturn integrate claude --apply   (answered yes)
Wrote /path/to/repo/.claude/settings.local.json
Start a new coding session for the change to take effect.

$ zeroturn doctor | grep "claude integration"
OK    claude integration     installed in /path/to/repo/.claude/settings.local.json

$ zeroturn integrate claude --remove   (answered yes)
Removed 6 ZeroTurn entries from /path/to/repo/.claude/settings.local.json
Backup /path/to/state/backups/claude-settings-20260911-230941.json
```

The file that remains holds what was there before ZeroTurn, and nothing of ZeroTurn's. In this run the file had not existed, so it is left empty.

## The report for the run

```text
$ zeroturn report day
ZEROTURN REPORT
Sessions observed         1
Peak context              95%
Subagents started         0
Highest active subagents  0
Confirmation requests     1
Denied subagent starts    0
Direct validations        1
Direct Git operations     0
Based only on events observed locally by ZeroTurn on this machine.
```

Counts of events observed locally. Not tokens, not cost, and not a saving.

## A second project

A second project on the same machine, a web application with its own test and lint scripts, was read to prepare this example. It was not modified, committed to, or pushed, and the configuration below was never written into it. The steps name scripts that exist in that project rather than ones invented for the example.

```json
{
  "version": 1,
  "guard": {
    "mode": "confirm",
    "context": { "warn": 70, "confirm": 80, "critical": 90 },
    "limits": { "fiveHourWarn": 75, "sevenDayWarn": 75 },
    "session": { "durationWarnMinutes": 240, "activeSubagentsWarn": 2, "subagentStartsWarn": 4 }
  },
  "verify": {
    "steps": [
      { "name": "typecheck", "command": ["npm", "--prefix", "apps/web", "run", "typecheck"] },
      { "name": "lint", "command": ["npm", "--prefix", "apps/web", "run", "lint"] },
      { "name": "test", "command": ["npm", "--prefix", "apps/web", "test"] }
    ]
  },
  "git": { "remote": "origin", "protectedBranches": ["main", "master"] }
}
```

`zeroturn init` proposes steps only for scripts it finds, so it would propose these three after reading that project's `package.json`. Running them still needs `zeroturn verify --approve` on the machine, which shows each command before anything executes.

## What the run showed

Everything above behaved as documented. Two things are still unproven and are recorded as such: the interactive approval prompt for Confirm mode has not been observed, and the integration has only been used on macOS.
