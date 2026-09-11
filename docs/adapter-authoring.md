# Writing an adapter

An adapter connects one coding harness to ZeroTurn. It translates the harness's events into the normalized form, hands them to `zeroturn event`, and passes any decision back. It holds no policy of its own.

The promises on both sides are in [harness-contract.md](harness-contract.md). A worked example is in [../integrations/template/](../integrations/template).

## Before you start

Find out what the harness actually exposes, and write it down honestly. Three questions decide how much of ZeroTurn can work there.

1. **Does a hook run before a subagent starts, and can its answer stop the subagent?** If yes, the gate works. If the hook only reports, the harness supports observation, and the documentation for that harness must say so rather than implying enforcement.
2. **Does the harness report session values?** Context use, usage windows, and session duration are what the gate compares against thresholds. Without them the status line has nothing to show, as is the case with Copilot today.
3. **Does it report when subagents start and stop?** Those two events are what the subagent counts are built from.

If the first answer is no and the second is no, an adapter adds nothing. Say so in an issue rather than writing one.

## What the adapter does

1. Receives an event from the harness, in whatever form the harness uses.
2. Builds a JSON document following [normalized-event.schema.json](../schemas/normalized-event.schema.json).
3. Runs `zeroturn event --harness normalized`, with the document on standard input, the session's directory as the working directory, and arguments passed as an array rather than through a shell.
4. Reads standard output. Empty means allow. A decision document means ask or deny.
5. Translates that decision into the harness's own response, without strengthening it.

## Rules

- **Send measurements, nothing else.** No prompt text, no responses, no file contents, no diffs, no credentials, no environment values. If a field is not in the schema, it does not belong in the event.
- **Leave out what you do not have.** A measurement the harness did not report is omitted or sent as `null`, never as zero.
- **Keep the session identifier stable**, because the counts belong to it.
- **Send an agent identifier** with start and stop events when the harness provides one.
- **Never invent a stronger decision.** An `ask` stays an ask. If the harness cannot ask, report that in the harness documentation, and do not turn it into a deny.
- **Fail open.** If ZeroTurn is missing, slow, or returns nothing, the harness continues as if the adapter were not installed. A guard that breaks a session is worse than no guard.
- **Hold no policy.** Thresholds, modes, and decisions live in ZeroTurn. An adapter that decides anything on its own will disagree with `zeroturn policy check`, and the user will not know which to believe.
- **Set a timeout in the harness**, not in the adapter. The Claude Code integration uses ten seconds per hook.
- **Handle reload and a cleared session.** Starting fresh must not leave counts from a session that is gone.

## Checking your work

```sh
go run ./conformance/validate schemas/normalized-event.schema.json my-event.json
go test ./conformance
```

The first checks one event your adapter produced. The second checks that ZeroTurn still follows every published schema, which is what your adapter relies on.

Then check the behavior by hand, in this order:

1. An event type ZeroTurn does not model is ignored, and the harness continues.
2. With `guard.mode` at `observe`, a proposed subagent produces no output at all.
3. With `confirm` and a session past a threshold, the harness shows the reason and waits for the person.
4. With `strict` approved on the machine and context past critical, the subagent does not start.
5. Malformed input produces no output and exit 0.
6. Cancelling the session leaves no process behind.

## Shipping it

Adapters live in `integrations/<harness>/`, with a document in `docs/integrations/<harness>.md` that states plainly which parts work, which do not, and on which version of the harness that was observed. A pull request that adds an adapter should include the answers to the three questions above and the results of the checks.
