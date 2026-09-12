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

Date: 2026-09-11

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
