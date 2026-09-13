# GitHub Copilot CLI

> [!IMPORTANT]
> No Copilot adapter ships in this version. ZeroTurn claims support for a harness only after it has run against a real session on that harness, and nobody has done that yet. This page records what the installed CLI exposes, verified by reading the package, so that an adapter can be written against facts rather than documentation.

## What was checked, and how

Version 1.0.83, read on 2026-09-12 from the installed package rather than from the documentation. The earlier research in [product-boundary.md](../product-boundary.md) was taken from the documentation and was wrong for this version, which is recorded as [decision 19](../decisions.md).

The code that matters is not in the package root. It is in the platform sub package, `@github/copilot/node_modules/@github/copilot-darwin-x64`, where `app.js` is the host and `prebuilds/darwin-x64/runtime.node` holds the hook engine. The interface below was read from `schemas/api.schema.json`, `copilot-sdk/types.d.ts`, `app.js`, and the string table of the native module.

## The two halves, and where each one comes from

ZeroTurn needs two things from a harness: a decision point before a subagent starts, and measurements describing session pressure. On Copilot they come from two different places, and this is the single most important fact on this page.

| What ZeroTurn needs | Where Copilot provides it |
| --- | --- |
| A decision before a subagent starts | A `preToolUse` hook, which may answer `allow`, `deny`, or `ask` |
| Context use, premium requests, credits | The `statusLine.command` process, which receives them as JSON on standard input |

**No hook payload carries usage, quota, token counts, or session duration.** That was checked directly: the hook input types in `copilot-sdk/types.d.ts` carry only `sessionId`, `timestamp`, `workingDirectory`, `toolName`, `toolArgs`, and the per event additions. The quota structures live in the account client, a different module of the native binary, and reach the CLI through a usage RPC rather than through hook dispatch.

The status line is the way in. `statusLine.command` is run with the platform shell and receives the session status as JSON on standard input, including `context_window.used_percentage`, `context_window.context_window_size`, `cost.total_premium_requests`, and `ai_used`. This is the same shape of arrangement ZeroTurn already uses on Claude Code, where the status line payload is the source of every pressure measurement.

## Hooks

### Where a hook is defined

| Location | Scope |
| --- | --- |
| `.github/hooks/` in the repository | Repository, loaded only after folder trust is confirmed |
| `~/.copilot/hooks`, or `$COPILOT_HOME/hooks` | The person, across repositories |
| A `hooks` key inside `config.json`, `settings.json`, or `settings.local.json` | Whichever of those files it appears in |
| `hooks/hooks.json` inside a plugin | The plugin |

`disableAllHooks` turns off every hook at both levels. File based hooks are also gated by `enableFileHooks`, which an SDK host can switch off independently of callback hooks.

### Event names

Verified from the `HookType` enum in `schemas/api.schema.json`:

`preToolUse`, `preMcpToolCall`, `postToolUse`, `postToolUseFailure`, `userPromptSubmitted`, `userPromptTransformed`, `sessionStart`, `sessionEnd`, `postResult`, `prePRDescription`, `errorOccurred`, `agentStop`, `subagentStart`, `subagentStop`, `preCompact`, `permissionRequest`, `notification`.

Two spellings differ from Claude Code in a way that will silently do nothing if an adapter guesses: the prompt event is `userPromptSubmitted`, not `userPromptSubmit`, and the end of turn event is `agentStop`, with no plain `stop`. Copilot does accept Claude Code's PascalCase names as an alias, and a hook configured that way receives a snake case payload carrying `hook_event_name`, `session_id`, and `tool_input` instead of the camel case one.

### A hook can be an external command

This was the open question, and the answer is yes. A hook definition carries exactly one of:

| Field | What it runs |
| --- | --- |
| `command` | A shell command, the cross platform alias for the two below |
| `bash` or `powershell` | A shell command for that platform |
| `exec` | A native executable, without a shell |
| `url` | An HTTP POST, which must use HTTPS for events that affect authorization |
| `prompt` | Text handled by the model |

The process's standard output is captured and parsed as JSON. Exit code 2 denies the tool call. Other fields on a definition are `matcher`, `type`, `timeoutSec` with `timeout` as an alias, and `allowedEnvVars`. Matchers cannot be empty, and nesting deeper than one level is refused.

### The decision shape

From `PreToolUseHookOutput` in `copilot-sdk/types.d.ts`:

```ts
permissionDecision?: "allow" | "deny" | "ask";
permissionDecisionReason?: string;
modifiedArgs?: unknown;
additionalContext?: string;
suppressOutput?: boolean;
```

The first two fields are the same words ZeroTurn already produces for Claude Code. An adapter still has to translate the envelope, and it must never turn an `ask` into a `deny`.

`additionalContext` puts text into the model's context. ZeroTurn does not use fields of that kind on any harness, and an adapter must not use this one.

### Timeouts

A per hook timeout exists and is set with `timeoutSec`. A hook that times out is allowed to proceed rather than blocked, which matches ZeroTurn's own fail open rule. A hook that fails with an error is treated differently from one that times out, so an adapter must still exit 0 on its own errors rather than relying on the harness.

The default timeout is a compiled constant with no text form anywhere in the package, so it is not stated here. Set `timeoutSec` explicitly rather than assuming a value.

## What is still unverified

Nothing on this page should be read as a working adapter. These remain unchecked because they cannot be checked by reading a package:

- Whether a command hook in a `.github/hooks` file receives the same fields as an SDK callback in practice, and whether its answer is honoured end to end.
- The exact key layout of a `.github/hooks/*.json` file, which is described by validation messages rather than by a published schema.
- Whether the status line payload reaches a child process with the fields named above in a real session.
- The default hook timeout.

Every one of them is a question that a single real session answers, which is how the Claude Code gate was proven before the rest of that integration was written. See [decision 2](../decisions.md) for that method and [claude-code.md](claude-code.md) for the result.

## Why there is no `zeroturn integrate copilot`

`zeroturn integrate` writes a settings file for a harness whose interface has been observed. Writing one for an interface read from a binary would be claiming enforcement nobody has watched, which is the one thing [the harness contract](../harness-contract.md) says an adapter must never do.

When an adapter exists, it will be a normalized adapter: it translates a Copilot event into the event in [normalized-event.schema.json](../../schemas/normalized-event.schema.json) and sends it to `zeroturn event --harness normalized`. [integrations/template/](../../integrations/template) is the worked example to copy, and [adapter-authoring.md](../adapter-authoring.md) has the steps.
