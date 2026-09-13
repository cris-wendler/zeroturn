# Harness contract

This is what ZeroTurn promises to an adapter, and what an adapter must do in return. The version is `zeroturn.event/1` for events and `1.2.0` for the command surface, the JSON output, and the exit codes together. `zeroturn capabilities --json` reports both.

The schemas named here are in [schemas/](../schemas), and [conformance/](../conformance) checks that ZeroTurn's own output follows them.

## Process invocation

An adapter starts ZeroTurn as a process. There is no daemon, no socket, and no port.

```text
zeroturn event --harness normalized
```

- **Arguments** are passed as an array. An adapter must not build a command line string and hand it to a shell.
- **Working directory** is the directory of the session. ZeroTurn finds the repository from it, reads `.zeroturn.json` there, and records a hash of the repository path. An adapter that cannot set the working directory sends `cwd` in the event instead.
- **Standard input** carries exactly one JSON event. ZeroTurn reads to the end of input, so the adapter must close it.
- **Standard output** carries the response, when there is one. For the gate that is a decision document; for everything else it is empty.
- **Standard error** carries a short message when ZeroTurn ignores an event. It is for a person reading logs, not for the adapter to parse.
- **Environment** is inherited. `ZEROTURN_STATE_DIR` moves the local records, which tests and the live compatibility check use.

One process handles one event. A hook that fires ten times starts ten processes, which is why the gate and the status line are measured in milliseconds. See [benchmarks.md](benchmarks.md).

## Events

The adapter sends one document that follows [normalized-event.schema.json](../schemas/normalized-event.schema.json).

```json
{
  "contract": "zeroturn.event/1",
  "harness": "example",
  "type": "subagent.pre",
  "sessionId": "01H...",
  "cwd": "/path/to/repository",
  "contextPercent": 82,
  "fiveHourPercent": 81,
  "fiveHourResetsAt": 1788000000,
  "durationMs": 11520000
}
```

| Type | When to send it |
| --- | --- |
| `status` | The harness reports session values, typically to draw a status line |
| `subagent.pre` | A subagent is proposed and the harness can still allow, ask, or deny |
| `file.read` | A file is about to be read into the conversation, carrying `filePath` |
| `subagent.start` | A subagent has started |
| `subagent.stop` | A subagent has finished |
| `session.stop` | A turn has ended, carrying the number of background tasks still running |
| `session.end` | The session has ended |

There is no event for a prompt. The prompt guard exists only on the Claude path, through `zeroturn event --harness claude --event UserPromptSubmit`, and an adapter never sends message text to ZeroTurn.

Rules for the adapter:

- Send only the fields in the schema. ZeroTurn's decoder declares no others, so anything else is discarded, but sending it means the adapter has read it.
- Never send prompts, responses, transcripts, file contents, diffs, credentials, or environment values. The gate makes its decision from measurements alone. The one exception is `filePath` on `file.read`, which is a path, not content: ZeroTurn opens that file itself and reports only the file, the line, and the category of what it finds.
- A measurement the harness did not report is left out, or sent as `null`. It is never sent as zero. Zero means the measurement was zero.
- Send `fiveHourResetsAt` whenever the harness reports one. A usage percentage on its own gives the level and not the trajectory, and the gate uses the reset time to tell a window filling faster than the clock from one that will reset before it matters. Without it, the rate projection cannot run.
- `sessionId` must be stable for the life of a session. Counts belong to it.
- `agentId` on `subagent.start` and `subagent.stop` lets ZeroTurn tell subagents apart, so a repeated or out of order stop cannot drive the active count below the truth.

## Permission decisions

For `subagent.pre` and `file.read`, ZeroTurn may answer on standard output with a document following [decision.schema.json](../schemas/decision.schema.json):

```json
{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"ask","permissionDecisionReason":"New subagent requires approval. Context is 82% and five hour usage is 81%."}}
```

- **allow** prints nothing. Silence means ZeroTurn has no objection, and the harness follows whatever rules the user already configured.
- **ask** means the harness should put the question to the person, showing the reason.
- **deny** means the subagent must not start, and the reason says why.

The shape above is Claude Code's. An adapter for another harness translates the decision into that harness's own response, and must not invent a stronger outcome than the one given: an `ask` is never turned into a `deny`.

If the harness has no way to ask or deny, the adapter reports that, and the documentation for that harness says the gate is observation only. Claiming enforcement that the harness does not honour is the one thing an adapter must never do.

## Mutation classification

| Command | Changes what |
| --- | --- |
| `status`, `policy show`, `policy check`, `report`, `capabilities`, `doctor` | Nothing outside ZeroTurn's own records |
| `event` | ZeroTurn's own records only |
| `init`, `policy set`, `policy reset` | `.zeroturn.json` in the repository, after showing what it will write |
| `integrate --apply`, `integrate --remove` | One harness settings file, after a plan and a confirmation, with a backup |
| `verify` | Runs the commands the user approved, writes logs under `.git/zeroturn/logs/` |
| `ship` | Stages, commits, and pushes, after a confirmation |

An adapter may call the first two groups freely. The rest are for a person at a terminal: they ask for confirmation and refuse when no terminal is attached.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | success |
| 1 | validation or policy failure |
| 2 | invalid invocation or configuration |
| 3 | unsafe Git state |
| 4 | required executable unavailable |
| 5 | internal failure |
| 6 | confirmation declined |
| 7 | incompatible contract version |
| 8 | integration unavailable |

`zeroturn event` is the exception: it exits 0 whatever happens. A gate that fails must not stop a developer's session, so a malformed event, an unreadable state directory, or a broken configuration all end in silence and exit 0. This is the fail open rule, and an adapter depends on it.

## Cancellation and timeouts

ZeroTurn stops on an interrupt signal. `verify` puts each step in its own process group and stops the whole group, so a cancelled run leaves no child processes behind.

ZeroTurn sets no timeout of its own on an event. The harness should: the Claude Code integration writes `"timeout": 10` on each hook. If ZeroTurn is killed part way through, its records stay readable, because every write is a rename into place under a lock.

## Redaction

Anything ZeroTurn prints or writes to a log passes through credential detection first. A finding names the file, the line, and the category, never the value. An adapter must do the same with anything it logs.

## Capability discovery

Read `zeroturn capabilities --json`, which follows [capabilities.schema.json](../schemas/capabilities.schema.json), rather than assuming. It reports the product version, the contract version, the event contract, the guard modes, the commands, the event types, the harnesses this build supports and which of them have a tested gate, the exit codes, and the fields that are recorded and never recorded.

## Versions and compatibility

- `contract` in an event names the event contract. A different value is refused with a message that names the one this build speaks, and the event is ignored. The adapter sees exit 0 and a line on standard error.
- `contractVersion` is `1.2.0`. Version 1.1.0 added the `file.read` event and the credential guard. Version 1.2.0 added the rate projection: three measurements in the session record and the `projection` trigger name. Neither took anything away from 1.0.0.
- `contractVersion` follows semantic versioning. A new field, event type, or command raises the minor version. Changing the meaning of an exit code, removing a field, or changing a decision shape raises the major version.
- ZeroTurn does not refuse a harness version it has not been tested against. It says so instead: `zeroturn policy set guard.mode strict` reports when the installed harness differs from the tested one.
- An adapter should send the harness version it is running against, so a report can say what was observed.

## Checking an adapter

```sh
go run ./conformance/validate schemas/normalized-event.schema.json my-event.json
go test ./conformance
```

The first checks one document an adapter produced. The second checks that this build of ZeroTurn still follows every published schema. A worked example of an adapter is in [../integrations/template/](../integrations/template), and the steps are in [adapter-authoring.md](adapter-authoring.md).
