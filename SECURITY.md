# Security policy

## Reporting a vulnerability

Report security problems privately through GitHub: open the repository's Security tab and choose "Report a vulnerability". Do not open a public issue for a security problem.

Include the ZeroTurn version (`zeroturn version`), your operating system, and the steps that show the problem. Remove any real credentials from what you send.

You will get an acknowledgement within seven days. The maintainer will tell you whether the report is accepted, and when a fix is expected.

## Supported versions

Until the first release, only the latest commit on `main` receives fixes.

## What is in scope

Reports are especially useful in these areas:

- a way for a repository's `.zeroturn.json` to run a command that was not approved, or to deny subagents without local approval of Strict mode
- a credential value that appears in output, logs, JSON, or ZeroTurn's local records
- prompt text, responses, transcript contents, or repository paths stored in local records
- a way for `zeroturn ship` to stage a file that was not named, push with force, skip Git hooks, or act on a protected branch
- a path outside the repository accepted by `zeroturn ship`
- `zeroturn integrate` removing or changing settings that ZeroTurn did not write

## What ZeroTurn does not protect against

ZeroTurn's credential scan looks for high confidence patterns. It does not replace a dedicated secret scanner, and a credential in an unusual format can pass it. The subagent gate depends on the harness honouring its decisions. ZeroTurn runs with your user permissions and does not defend against other software running as you.
