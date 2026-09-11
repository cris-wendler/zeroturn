# Conformance suite

This checks that ZeroTurn keeps the promises in [docs/harness-contract.md](../docs/harness-contract.md): that every published output follows its schema in [schemas/](../schemas), that a valid normalized event is accepted, and that an event naming a different contract is refused with a message that says so.

```sh
go test ./conformance
```

It builds the executable, creates a temporary repository with a local bare remote and temporary ZeroTurn records, and runs the real commands. Nothing on the machine is changed.

## Checking one document

An adapter author can check a single event without running the suite:

```sh
go run ./conformance/validate schemas/normalized-event.schema.json my-event.json
```

The command reads standard input when no document is named. It exits 0 when the document follows the schema, 1 when it does not, and 2 when the schema or the document could not be read.

The validator supports the keywords the published schemas use. A schema using anything else is reported as unsupported rather than passed over, so a contract can never look checked when it is not. A test in the suite holds every schema to that rule.
