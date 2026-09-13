# Product boundary

Status: validation complete. Implementation approved with the scope limits recorded at the end of this file and the narrowed scope recorded in [decisions.md](decisions.md).

Research access date: 2026-09-10. Every claim below was checked on that date. Anything that could not be checked is marked unverified.

## What was validated

ZeroTurn proposes two connected functions.

Session Guard shows when a coding session is becoming expensive and can request approval before another subagent starts.

Direct Lane runs routine validation and safe Git workflows directly on the local machine.

The product boundary is: ZeroTurn works with coding harnesses. It is not another coding harness.

Before writing code, three things had to hold. A maintained project must not already provide substantially the same product. The required platform interfaces must exist. The name must not carry a material conflict.

## Platform interfaces

### Claude Code 2.1.265

Verified two ways. The published reference was read, and the installed executable at `/usr/local/Caskroom/claude-code@latest/2.1.265/claude` was inspected for the symbols it actually ships. Both agree.

The status line payload is delivered as JSON on standard input. The fields ZeroTurn consumes are present in the shipped build:

| Field | Meaning | Present in 2.1.265 |
| --- | --- | --- |
| `session_id` | Session identifier | Yes |
| `version` | Harness version | Yes |
| `model.display_name` | Model label | Yes |
| `context_window.used_percentage` | Context used, 0 to 100, may be null early in a session | Yes |
| `context_window.context_window_size` | Context size in tokens | Yes |
| `rate_limits.five_hour.used_percentage` | Five hour window usage | Yes, account dependent |
| `rate_limits.seven_day.used_percentage` | Seven day window usage | Yes, account dependent |
| `rate_limits.*.resets_at` | Window reset, Unix seconds | Yes |
| `cost.total_duration_ms` | Wall clock session duration | Yes |
| `exceeds_200k_tokens` | Fixed threshold flag | Yes |
| `workspace.current_dir`, `workspace.project_dir` | Directories | Yes |

The `rate_limits` object is absent for accounts that have no such window. ZeroTurn treats absence as unavailable and never substitutes zero.

Two fields ZeroTurn needs are not supplied by the harness at all: the number of subagents currently active, and the number of subagent starts so far in the session. ZeroTurn derives both from its own hook events and stores them itself. This is recorded in the README and in the Claude integration document, because it changes what the numbers mean.

Hook events confirmed present in the shipped build: `PreToolUse`, `PostToolUse`, `SubagentStart`, `SubagentStop`, `Stop`, `SessionStart`, `SessionEnd`, `UserPromptSubmit`, `PreCompact`, `Notification`.

The subagent gate depends on one specific finding. `SubagentStart` cannot block. It accepts only `additionalContext` in its response, which would inject text into model context and is something ZeroTurn refuses to do. The only interface that can allow, ask, or deny a subagent is `PreToolUse` with the matcher `Agent`, answering with `hookSpecificOutput.permissionDecision`. ZeroTurn gates there and counts elsewhere.

The matcher `Agent` contains only letters, so the harness treats it as an exact string comparison rather than a regular expression. This satisfies the requirement to avoid an unbounded pattern.

`Stop` and `SubagentStop` carry a `background_tasks` array. That is the only documented source of background work state, so background pressure can only be evaluated at those two moments.

### GitHub Copilot CLI 1.0.83

Re-examined on 2026-09-12 by reading the installed package rather than the documentation. The earlier finding below was wrong for this version, and is kept underneath so the correction is visible.

What version 1.0.83 ships, verified in `@github/copilot/node_modules/@github/copilot-darwin-x64`:

| Capability | Where it was verified |
| --- | --- |
| Hook events including `preToolUse`, `subagentStart`, `subagentStop`, `sessionStart`, `sessionEnd`, `userPromptSubmitted` | `schemas/api.schema.json`, the `HookType` enum |
| A pre tool decision of `allow`, `deny`, or `ask`, with a reason | `copilot-sdk/types.d.ts`, `PreToolUseHookOutput` |
| Quota snapshots: entitlement, used requests, remaining, reset date | `schemas/api.schema.json`, `AccountQuotaSnapshot` and `AccountGetQuotaResult` |
| Context window token counts | `schemas/api.schema.json`, `HistoryCompactContextWindow` |
| Token usage exportable locally as JSON lines | `copilot help monitoring`, `COPILOT_OTEL_FILE_EXPORTER_PATH` |
| Its own session budget, with warnings at 50, 75, and 90 percent | `copilot help limits`, `--max-ai-credits` |
| Hooks configurable as files, without the experimental extension interface | `copilot help config`, `hooks` keyed by event name, same schema as `.github/hooks/*.json` |

Not verified, and therefore not claimed: the exact schema of a `.github/hooks/*.json` file, whether a hook defined as a command receives the same input as an extension callback and may answer with a permission decision, and whether quota or context values reach a hook at all rather than only the account API and the session event stream.

Consequence for the product: the reason Copilot support was cut no longer holds in the form it was written. The gate and subagent counting look possible through documented interfaces, and session pressure looks possible through the account API or the local telemetry file. What ZeroTurn claims about Copilot stays at nothing until an adapter is written and tested against a real session.

The earlier finding, from 2026-09-10, which this replaces:

Three extension surfaces exist and they are not equivalent.

Hooks are documented and carry no experimental label. The event list includes `preToolUse`, which accepts `permissionDecision` with values allow, deny, or ask. Subagents are spawned through the built in `task` tool, so gating is possible at the tool layer.

Extensions require `@github/copilot-sdk` and the `--experimental` flag. GitHub's own documentation states that extensions are experimental and subject to change. Custom slash commands such as `/zt-status` are only available through this path.

Plugins are generally available and can carry hooks and MCP servers.

What is missing is decisive for Session Guard. No Copilot hook payload carries usage consumption, premium request consumption, context window utilization, or session duration. Those values appear only in the interactive terminal interface and in an optional OpenTelemetry export that omits premium request consumption and context percentage. There is no local quota query command.

Consequence for the product: on Copilot, Session Guard cannot show session pressure, because the pressure values are not exposed. Subagent gating is possible through `preToolUse` on the `task` tool. Locally handled commands are possible only through the experimental extension interface.

Copilot support is therefore classified as experimental and partial, and the README says so in those words. The core does not change to accommodate it.

## Existing projects

### Directly relevant

| Project | Purpose | Maintained | License | Install | Overlap with ZeroTurn | Difference | Source |
| --- | --- | --- | --- | --- | --- | --- | --- |
| ccusage | Reads local usage files and reports cost per day, session, and five hour block | Yes, pushed 2026-09-11 | MIT | `npx ccusage@latest` | Reports session usage | Display only. No gate. No local Git or validation work. Parses transcript files, which ZeroTurn refuses to do | https://github.com/ccusage/ccusage |
| ccstatusline | Configurable Claude Code status line with a terminal configurator | Yes, pushed 2026-09-07 | MIT | `npx -y ccstatusline@latest` | Renders session status | Display only. No gate. Reads transcript files for values the payload lacks | https://github.com/sirmalloc/ccstatusline |
| claude-powerline | Powerline style status line with rate limit display | Yes, pushed 2026-09-06 | MIT | `npx -y @owloops/claude-powerline@latest` | Renders the same official rate limit fields ZeroTurn renders | Display only. No gate. No local command lane | https://github.com/Owloops/claude-powerline |
| cc-statusline | Generates a shell status line script from a wizard | Slower, pushed 2026-02-16 | MIT | `npx @chongdashu/cc-statusline@latest init` | Renders session status | Display only. Generates shell scripts rather than shipping an executable | https://github.com/chongdashu/cc-statusline |
| OpenUsage | Terminal dashboard aggregating usage across many providers, plus a Claude status line | Yes, pushed 2026-09-09 | MIT | `brew install janekbaraniewski/tap/openusage` | Shows usage windows | Display only. Multi provider dashboard, which is explicitly outside ZeroTurn's scope. Per session granularity implies local file parsing, unverified | https://github.com/janekbaraniewski/openusage |
| claude-code-tokenbudget | Daily, weekly, and monthly token and cost quotas | Pushed 2026-05-09, 2 stars | MIT | Plugin marketplace | Blocks work when a budget is crossed | Gates the user prompt, not delegation. Budget accounting rather than session pressure. No local command lane | https://github.com/outbit/claude-code-tokenbudget |
| cost-guardian | Cost tracking with a hard mode that blocks tool calls over budget | Pushed 2026-06-20, 35 stars | MIT | Plugin marketplace | Blocks tool calls through `PreToolUse` | Blocks on dollar budget across all tools. Not specific to delegation. No validation runner or Git workflow | https://github.com/Manavarya09/cost-guardian |
| claude-code-limiter | Per user rate limiting for a shared subscription | Early beta, pushed 2026-03-25 | MIT | `npx @howincodes/claude-code-limiter setup` | Blocks prompts over quota | Requires a remote server. ZeroTurn has no network service. Team administration rather than local awareness | https://github.com/howincodes/claude-code-limiter |
| context-guard, inside session-optimizer | Context budget enforced through a Stop hook | Yes, pushed 2026-09-10, 1 star | MIT | Plugin marketplace | Watches context pressure | Forces checkpoint, `/clear`, and resume, and delegates persistence to a subagent. ZeroTurn refuses automatic compaction, automatic clearing, and automatic handoff generation | https://github.com/cdeust/session-optimizer |
| AgentBudget | Dollar budget with circuit breaking and nested sub budgets | Pushed 2026-05-30, 108 stars | Apache-2.0 | `pip install agentbudget` and others | Allocates budget to child tasks | An SDK for agent authors, not a tool for a developer using a harness. Cost accounting rather than session pressure | https://github.com/AgentBudget/agentbudget |
| dsh-agent-budget | Token budget plugin gating subagent and workflow tools | Pushed 2026-08-12, 2 stars, no license file | Unlicensed | Plugin | Gates subagent tools | Targets a different harness. No license, so it cannot be reused or compared for code. Very early | https://github.com/vibeinging/dsh-agent-budget |
| RTK | Proxy that compresses command output to cut token consumption | Yes, pushed 2026-09-10 | Apache-2.0 | `brew install rtk` | Reduces tokens spent on routine commands | Compresses output of commands a model still runs. ZeroTurn instead lets the developer run the command without the model. Generic output compression is on ZeroTurn's refusal list | https://github.com/rtk-ai/rtk |
| git-safe, git-guardrails | Hooks that block destructive Git commands the model tries to run | git-safe parent pushed 2026-09-08 | MIT | Install script or skill file | Git safety | Restricts what the model may do. ZeroTurn gives the developer a direct safe path instead. Complementary, not overlapping | https://github.com/Bande-a-Bonnot/Boucle-framework |
| Commit Check | Commit message, branch name, and metadata policy, plus a no force push check | Yes, released 2026-09-06 | MIT | `pip install commit-check` | Refuses non fast forward pushes | Commit metadata policy. No session awareness. Python, which ZeroTurn does not depend on | https://github.com/commit-check/commit-check |
| pre-commit | Framework for running Git hooks | Yes, released 2026-08-10 | MIT | `pip install pre-commit` | Runs checks before a commit | A hook runner that ZeroTurn deliberately does not replace. ZeroTurn never passes `--no-verify`, so pre-commit keeps working underneath it | https://github.com/pre-commit/pre-commit |
| act | Runs GitHub Actions workflows locally in Docker | Yes, released 2026-06-01 | MIT | `brew install act` | Local validation | Runs CI workflow files. ZeroTurn runs a short configured list of project commands and requires no container | https://github.com/nektos/act |
| wrkflw | Local GitHub Actions and GitLab CI validator and runner | Yes, pushed 2026-09-08 | MIT | `cargo install wrkflw` | Local validation | Same category as act. Not session aware | https://github.com/bahdotsh/wrkflw |

### Checked and found not relevant

Archon repositioned as a workflow engine that builds coding processes and manages worktrees. It is a harness layer, which is the thing ZeroTurn declines to be. MIT, actively maintained, https://github.com/coleam00/Archon.

OpenHarness is not one project. At least six unrelated repositories share the name, the most cited of which has 21 stars, no license file, and no push since 2026-01-23. There is nothing stable to compare against. https://github.com/jeffrschneider/OpenHarness.

ContextGuard as a distinct maintained Claude Code project does not exist. One repository by that name now returns 404. Another with the name is zero trust middleware for Model Context Protocol deployments and is unrelated. The functional match is the `context-guard` plugin listed in the table above.

agent-token-saver compacts tool output before the model sees it and explicitly never blocks. MIT, pushed 2026-09-10. Output compression is on ZeroTurn's refusal list, so the overlap is nil. https://github.com/Supersynergy/agent-token-saver

Copilot usage extensions are Visual Studio Code status bar widgets for premium request quota. They are display only, they do not gate anything, and they do not run local Git or validation work. Examples checked: https://github.com/kasuken/vscode-copilot-insights and https://github.com/ethanhubin/copilot-usage-tracker.

Handoff tools build a document summarising a session so work can resume elsewhere. Four were checked. All are user invoked, none gates delegation, none is session pressure aware. Automatic handoff generation is on ZeroTurn's refusal list. Examples: https://github.com/Sonovore/claude-code-handoff and https://github.com/thepushkarp/handoff.

## The six questions

**1. Does an existing tool already combine session pressure, subagent gating, and direct local operations?**

No. The three capabilities exist in three separate categories and no project spans them.

Session pressure display is a crowded category and it is uniformly display only. Not one status line project gates anything.

Gating exists, but it gates the wrong thing for this purpose. Two projects block on a dollar or token budget across all tool calls. One blocks user prompts and needs a server. One forces a context clear. None asks specifically before delegation, and none asks using session pressure as the input.

Local validation and safe Git exist as separate tools with no session awareness at all.

The gap is real and narrow: nothing connects "this session is expensive" to "so ask before starting another subagent", and nothing pairs that with a way to get the routine work done without a model turn.

**2. Would ZeroTurn provide a meaningful improvement?**

Yes, on a modest scale. Two specific improvements hold up.

A gate that fires on delegation specifically, using context percentage, usage windows, session duration, and subagent counts as its inputs, does not currently exist. The nearest tools gate every tool call on a budget, which is noisier and answers a different question.

Subagent counts are not supplied by the harness. A tool that maintains them from `SubagentStart` and `SubagentStop` and renders them next to context percentage is showing something no status line currently shows.

The improvement is not large and the README does not claim otherwise.

**3. Would the project merely combine unrelated features?**

This was the closest question, and it deserves the honest version.

The risk is real. A session monitor and a Git workflow tool are, on their face, two products. Many projects have been made worse by pairing.

What connects them is one specific idea. Both halves are about model turns. The gate asks whether another turn should start. The direct lane provides a way to finish routine work without one. A developer who is told the session is expensive and is given nothing else to do with that information has been informed, not helped.

The counterargument stands and is recorded here: a user who wants only the status line will carry a Git workflow they do not use, and the reverse. The mitigation is that the two halves share configuration, state, and output contract, and neither requires the other to function. Neither half needs a network, a model, or a daemon. If the pairing turns out to be wrong, the direct lane can be removed without changing Session Guard.

The judgement is proceed, with the pairing stated plainly in the README rather than justified.

**4. Can the useful scope remain small?**

Yes, and the refusal list is the mechanism. A model SDK, model selection, a chat interface, transcript collection, prompt inspection, remote telemetry, a hosted dashboard, output compression, automatic handoff, automatic compaction, automatic session clearing, arbitrary shell strings, automatic force pushing, automatic rebasing, credential bypasses, silent global changes, Python packaging, and containers without demonstrated need are all out of scope and recorded in `CONTRIBUTING.md`.

Ten commands, one executable, no dependencies. That is small enough for one person. The proposed `sync` command was dropped after this review, see [decisions.md](decisions.md), entry 1.

**5. Is the name available?**

On the registries, yes. Free on npm, PyPI, crates.io, Homebrew core, and pkg.go.dev. No GitHub repository holds an actively maintained developer tool by this name. No software trademark for "ZeroTurn" was found in general web search. A formal trademark search was not performed and would be needed before any commercial filing. That gap is unverified and is recorded as a risk.

Two soft conflicts were found and both changed a decision.

ZeroTurnaround is a real Java developer tools company, maker of JRebel, acquired by Rogue Wave in 2017 and by Perforce in 2019, with an active GitHub organisation whose libraries are named `zt-zip`, `zt-exec`, and `zt-process-killer`. "ZeroTurn" truncates "ZeroTurnaround", and the proposed `zt` alias lands exactly on their naming convention. The `zt` name is also occupied on npm and PyPI by dormant packages.

Decision: the `zt` alias is not shipped. The executable is `zeroturn` and nothing else. A developer who wants a shorter name can make their own alias, and `zeroturn doctor` says so. This costs a little convenience and removes the only part of the name that pointed at another company's libraries. The product name stays ZeroTurn, because the domain does not overlap. ZeroTurnaround sells JVM hot reload. ZeroTurn is a local command line tool for coding sessions.

"Zero turn" is also the standard industry term for a class of lawn mower, which means generic search will not find this project. That is a discoverability cost, not a conflict.

The GitHub account `zeroturn` is registered to an inactive user from 2015, so the organisation name is not available. The module path is therefore under the maintainer's own account.

**6. Can the project be maintained by one primary maintainer?**

Yes, under the constraints already chosen. One executable, the Go standard library only, no dependencies to track, no service to operate, no account to administer, and no release cadence forced by a platform. The recurring cost is tracking harness interface changes, which is why the conformance suite and the compatibility test exist. The Copilot adapter is the one piece that could become a maintenance burden, which is why it is classified experimental and kept thin enough to remove.

## Stop conditions

Each condition was checked before proceeding.

| Condition | Triggered | Evidence |
| --- | --- | --- |
| A maintained project already provides substantially the same product | No | Sixteen projects examined. None spans the three functions. Closest are budget gates that gate all tool calls rather than delegation |
| The distinction relies only on different wording | No | The distinction is the gate input, the delegation specific trigger, and the locally maintained subagent counts, all of which are checkable in code |
| The feature set cannot remain coherent | No, with a recorded reservation | Answer 3 above states the reservation rather than dismissing it |
| Required platform interfaces are unavailable | No for Claude Code, partly yes for Copilot | Claude interfaces verified in the shipped executable. Copilot lacks usage and context signals, so Copilot support is reduced rather than claimed |
| Subagent control cannot be tested reliably | Not yet known | `PreToolUse` on `Agent` is the documented path. `zeroturn doctor --compat` runs a non destructive test and reports the result. If allow, ask, and deny cannot be demonstrated, Confirm and Strict are disabled and Observe is reported as the only supported mode |
| The name has a material conflict | No, after dropping the alias | Registries free, no software trademark found, `zt` alias removed to clear the ZeroTurnaround association |

## Scope limits carried into implementation

ZeroTurn never reads a transcript file. The `transcript_path` value arrives in nearly every hook payload and is discarded on receipt.

ZeroTurn never injects text into model context. This rules out `SubagentStart` `additionalContext`, `UserPromptSubmit`, and blocking `Stop` to make the harness continue.

ZeroTurn never calls a model to decide whether a model call should happen.

Percentages from different usage characteristics are never added together. Context percentage, five hour usage, and seven day usage are separate measurements of separate things and are displayed separately.

Subagent counts are ZeroTurn's own observations, not a harness supplied figure, and are labelled that way wherever they appear.

Copilot support is experimental and partial until GitHub exposes usage and context to hooks and moves the extension interface out of experimental status.
