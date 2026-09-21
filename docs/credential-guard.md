# Credential guard

![Two guards. Files the model reads, on by default: before a file is opened ZeroTurn scans it and the harness asks you, with modes ask, deny, and off. Messages you send, off by default: switch it on and a message holding a key is stopped before it is sent, which means ZeroTurn reads your messages in that repository in memory, and a stopped message is erased. Both checks run on your machine, nothing is stored, and the explanation names the file, the line, and the kind, never the value.](img/credential-guard.svg)

## Files the model reads

Before the model reads a file, ZeroTurn scans that file. If it holds something shaped like a credential, the harness asks you first:

```text
Reading this file would put a credential into the conversation. deploy.env line 2 looks like
an aws access key id. Approving means the value is shared and should be rotated.
```

Approving reads the file as usual. Declining means the value never leaves your machine, and there is nothing to rotate.

| Setting | Behavior |
| --- | --- |
| `ask` | The default. The harness asks before that file is read |
| `deny` | The read is refused |
| `off` | No scanning |

```sh
zeroturn policy set guard.credentials.mode deny
```

This reads the file the model was about to open, and only that file. It never reads your prompts. The message names the file, the line, and the kind of credential, never the value.

It looks for high confidence patterns, so it catches a common mistake and not every possible one. A file larger than 1 MB is skipped, and a credential in an unusual format can pass. It is a guard, not a guarantee.

## Messages you send

A key pasted into a message is the other way one reaches the model. ZeroTurn can check for that too. It is off by default, because switching it on changes what ZeroTurn reads.

```sh
zeroturn policy set guard.credentials.prompts block
```

The command explains both consequences and asks you to confirm. With it on, in that repository only:

- Every message you send passes through ZeroTurn first. It is read in memory, checked, and never stored, logged, or sent anywhere.
- A message carrying a credential is stopped before it leaves your machine, with an explanation that never repeats the value.

> [!WARNING]
> The harness erases a stopped message and does not hand it back, so a long message is lost.

With it off, ZeroTurn never reads what you write, and the hook that would do so is not installed.
