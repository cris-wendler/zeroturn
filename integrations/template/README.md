# Adapter template

A worked example of an adapter for a harness ZeroTurn does not support yet. The payload in `example-payload.json` is invented, so the script is something to copy and change rather than something to install.

Try it from the repository root:

```sh
integrations/template/adapter.sh toolCallProposed < integrations/template/example-payload.json
```

With a session that has crossed a threshold and `guard.mode` set to `confirm`, it prints the harness response this example invents:

```json
{"action":"confirm","message":"New subagent requires approval. Context is 82% and five hour usage is 81%."}
```

The steps for writing your own are in [docs/adapter-authoring.md](../../docs/adapter-authoring.md), and the promises on both sides are in [docs/harness-contract.md](../../docs/harness-contract.md).
