# ZeroTurn as a Claude Code plugin

Installs ZeroTurn's hooks. It does not install the status line, because a
plugin cannot contribute one.

## Install

ZeroTurn is a separate executable and the plugin does not carry it:

```sh
go install github.com/cris-wendler/zeroturn/cmd/zeroturn@latest
```

Then, in Claude Code:

```
/plugin marketplace add cris-wendler/zeroturn
/plugin install zeroturn@zeroturn
```

The hooks call `zeroturn` by name, so it has to be on your `PATH`.
`zeroturn doctor` says whether it is and names the line to add.

## What you get

| | |
| --- | --- |
| Credential guard | a file holding a key is stopped before it is read |
| Subagent gate | asks or denies before a new subagent, on the counts ZeroTurn keeps |
| Subagent counts | started, stopped, currently running |
| Validation evidence | `zeroturn verify` and `zeroturn report`, which need no hooks at all |

## What you do not get

Context use, the five hour and seven day usage windows, and session
duration. Those reach ZeroTurn through the harness status line and
through nothing else, and a plugin cannot install one, so the thresholds
built on them can never be crossed.

That is not a defect in the plugin. It is the same state an editor
extension produces, and `zeroturn policy check` and `zeroturn doctor`
both say so rather than reporting an all clear.

For those measurements, install the status line as well:

```sh
zeroturn integrate claude --plan    # shows what would change
zeroturn integrate claude --apply
```

## Generated

`hooks/hooks.json` is generated from the hook list in
`internal/harness/claude` by `go run ./scripts/genplugin`. A test fails
if the committed file is not what that produces.
