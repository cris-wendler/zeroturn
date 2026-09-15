# Claude Code integration

Status: supported. Tested on Claude Code 2.1.265 on macOS.

ZeroTurn connects to Claude Code through its documented status line and hook interfaces. It reads the JSON the harness sends on standard input and, for the subagent gate, answers with the harness's own permission decision. It does not scrape the interface, read transcripts, or add text to the model's context.

## Install

From inside the repository:

```sh
zeroturn integrate claude --plan    # show the change, write nothing
zeroturn integrate claude --apply   # write it after you confirm
```

Start a new session for the change to take effect.

The entries go into `.claude/settings.local.json`, the settings file Claude Code applies to you only in this repository. They contain the absolute path of your `zeroturn` executable, which is why they do not belong in the shared `.claude/settings.json`. The plan warns when the file is not ignored by Git.

`--user` writes to `~/.claude/settings.json` instead, which applies to every repository. It shows the same plan and asks for the same confirmation. ZeroTurn never changes that file without it.

Before writing, `--apply` copies the existing file to ZeroTurn's backup directory. Running it again changes nothing. An existing status line is left in place unless you pass `--replace-status-line`.

To install by hand, copy the entries from [integrations/claude/settings.example.json](../../integrations/claude/settings.example.json) and replace `/usr/local/bin/zeroturn` with the path of your executable.

## Remove

```sh
zeroturn integrate claude --remove
```

This deletes ZeroTurn's own commands and nothing else. If you added a command of your own to the same hook entry, it stays. The file is backed up first, and running the command again changes nothing.

## What is installed

| Setting | Purpose |
| --- | --- |
| `statusLine` | Renders the session line from the status line payload |
| `hooks.PreToolUse`, matcher `Agent` | The subagent gate |
| `hooks.PreToolUse`, matcher `Read` | The credential guard, before a file is read |
| `hooks.SubagentStart` | Counts a subagent as started and active |
| `hooks.SubagentStop` | Counts it as stopped |
| `hooks.Stop` | Reads how many background tasks are still running |
| `hooks.SessionEnd` | Marks the end of the session |

Both matchers contain only letters, so Claude Code compares them as exact strings rather than regular expressions. `PreToolUse` fires for no other tool on ZeroTurn's behalf.

A `UserPromptSubmit` hook is installed only when the prompt guard is switched on with `zeroturn policy set guard.credentials.prompts block`. With the guard off, which is the default, the hook is not in the settings file at all, so ZeroTurn is never in the path of a message. ZeroTurn never rewrites a prompt and never adds text to model context, in either case.

## Why the gate uses PreToolUse

`SubagentStart` fires after the decision to start a subagent and cannot block it. Its only response is text added to the model's context, which ZeroTurn does not use. `PreToolUse` on the `Agent` tool is the one interface that can allow, ask, or deny before the subagent exists. ZeroTurn gates there and counts in `SubagentStart` and `SubagentStop`.

## The credential guard

For the `Read` tool, ZeroTurn reads one field of `tool_input`, `file_path`, opens that file, and scans it for credentials. Nothing else from `tool_input` is declared, so the subagent prompt, commands, and replacement text are still never bound to a variable.

| Result | Response |
| --- | --- |
| No credential found | Nothing is printed, and the read proceeds |
| Found, mode `ask` | `ask`, with the file, the line, and the category |
| Found, mode `deny` | `deny`, with the same message |
| Mode `off`, or the path is not an ordinary file under 1 MB | Nothing is printed |

The file content is scanned in memory and never stored. Only a count of warnings is recorded.

## The prompt guard

Off by default. When it is on, `UserPromptSubmit` hands ZeroTurn the message text before it is sent.

| Result | Response |
| --- | --- |
| No credential found | Nothing is printed, and the message is sent |
| Found | `{"decision":"block","reason":"..."}`, and the harness stops the message |

The harness offers no way to ask about a message, so the only answers are block and silence. A blocked message is erased by the harness rather than returned, which `zeroturn policy set guard.credentials.prompts block` states before it asks for confirmation.

The message is held in memory for the length of the scan. It is never stored, logged, or included in the reason, and a test walks the records to confirm it.

## Decisions

| Mode | Threshold crossed | Response to the harness |
| --- | --- | --- |
| observe | any | nothing, the harness proceeds as usual |
| confirm | a confirm level threshold | `ask`, the harness asks you |
| strict | context at the critical threshold, approved on this machine | `deny`, the subagent does not start |
| strict | any other threshold | `ask` |

An allow decision prints nothing, so ZeroTurn never overrides permission rules you configured in the harness.

The reason shown in the prompt names at most two measurements, for example:

```text
New subagent requires approval. Context is 82% and 2 subagents are active.
```

The full policy and the session report are never sent. The content of the subagent prompt is never read.

If ZeroTurn cannot parse an event, open its state, or load the configuration, it prints nothing and exits successfully, and the harness behaves as if ZeroTurn were not installed.

## Tested behavior

| Decision | Result on 2.1.265 |
| --- | --- |
| deny | Honoured. No subagent started. Observed through `zeroturn doctor --compat --live`. |
| allow | Honoured. The subagent started, and start and stop events arrived with an agent identifier. |
| ask | Honoured. Observed interactively on 2.1.270: the harness showed the reason and the refusal stopped the subagent. In an unattended run it refuses the call, because nobody can answer. |

`zeroturn doctor --compat` checks that each mode produces the decision it claims, using fixture events. Adding `--live` starts one short non interactive session with the smallest model, in a temporary repository with temporary ZeroTurn state and a temporary settings file, and reports whether the harness honoured a denial. It spends a small amount of usage.

Other versions are not refused. `zeroturn policy set guard.mode strict` reports when the installed version differs from the tested one.

## Where this overlaps with the harness

Claude Code tracks the context window on its own. It shows how much is left before it summarises the conversation, and it summarises automatically when that point is reached. At high context you can therefore hear about context twice, once from the harness and once from ZeroTurn.

They answer different questions. The harness asks whether the conversation needs summarising. ZeroTurn asks whether another subagent should start, and it asks earlier, while there is still room to decide. The measurements the harness never reports, the five hour and seven day usage windows, session duration, and how many subagents are running, are the ones only ZeroTurn watches.

To hear less about context, raise the thresholds, for example `zeroturn policy set guard.context.confirm 88`. To stop context from triggering the gate at all, raise `guard.context.confirm` and `guard.context.critical` to 100. The status line keeps showing the percentage either way.

## Fields read from the status line payload

| Field | Shown as |
| --- | --- |
| `context_window.used_percentage` | `ctx 76%` |
| `context_window.context_window_size` | recorded, not shown |
| `rate_limits.five_hour.used_percentage` | `5h 81%` |
| `rate_limits.seven_day.used_percentage` | `7d 47%` |
| `rate_limits.*.resets_at` | recorded, not shown |
| `cost.total_duration_ms` | `session 3h12m` |
| `model.display_name` | recorded, not shown |
| `session_id`, `version`, `cwd` | used to find the session and the repository |

`context_window.used_percentage` can be null early in a session, and `rate_limits` is absent for accounts without usage windows. A missing value is omitted from the line, never shown as zero.

## Values the harness does not provide

The number of active subagents and the number started in the session are not in any payload. ZeroTurn counts them from `SubagentStart` and `SubagentStop`, so they cover only subagents started while the integration was installed. A stop that never arrives, because the session was interrupted, is cleared at the end of the turn: a subagent cannot outlive the turn that started it. The totals and the peak are kept, since they are the record of what happened.

Background task counts come from the `background_tasks` array on `Stop` and `SubagentStop`. Only the number of entries is used, because each entry carries a description that could repeat your request. The count is as current as the last of those events.

ZeroTurn does not use `cost.total_cost_usd` or `exceeds_200k_tokens`, and it does not assume a model specific weekly limit that the payload does not contain.

## Fields discarded on receipt

Hook payloads also carry `transcript_path`, `tool_input` (which holds the subagent prompt), `last_assistant_message`, and other fields. ZeroTurn's decoder declares no field for them, so they are never bound to a variable, stored, or logged. A test checks that none of this content reaches ZeroTurn's state.

## Performance

The status line reads one small file, starts no Git process, and makes no network request. On the machine used for the measurements it renders in 8.0 ms, of which about 6 ms is process start, and the gate answers in 7.7 ms. The method and the full table are in `scripts/bench`.

## When the executable moves

The settings hold the full path of the `zeroturn` executable. If it is moved, reinstalled somewhere else, or removed, the entries keep pointing at the old place and the hooks quietly do nothing, which for a guard is the worst way to fail.

```text
FAIL  integration path       the settings point at /old/path/zeroturn, which is not there, so the hooks
                             do nothing. Run zeroturn integrate claude --apply to repoint them
```

`zeroturn doctor` reports it, `zeroturn integrate claude --plan` shows what it would repair, and `--apply` rewrites only the entries ZeroTurn owns. A path that reaches the same file through a symbolic link, which is how a package manager usually installs an executable, is not treated as a change.

## Troubleshooting

`zeroturn doctor` shows whether the integration is installed for the current repository and whether the configuration loads.

If the status line shows `session data unavailable`, the harness has not sent context or usage values yet. They usually appear after the first response.

If they never appear, check where you are running Claude Code. The status line is a terminal feature, so an editor extension never draws one and never invokes the command. Every measurement the guard compares against a threshold arrives that way, and no hook payload carries any of them, so in an extension the guard has nothing to read. The hooks still fire, which is why subagents are still counted and the credential guard still works, and why the integration looks installed. `zeroturn doctor` compares the sessions it has recorded and says how many carried measurements.

If the gate never asks, run `zeroturn policy check` in the repository to see which thresholds the current session has crossed.

The status line has been checked in the terminal interface. Whether other interfaces of the harness render custom status lines has not been verified.
