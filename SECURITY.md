# Security policy

![Two columns. Report privately: open the Security tab, choose Report a vulnerability, and include the version, the system, and the steps, with real credentials removed. Never in a public issue, because a public report tells everyone before there is a fix. You get an acknowledgement within seven days and are told whether the report is accepted.](docs/img/security.svg)

> [!CAUTION]
> Do not open a public issue for a security problem. A public report tells everyone before there is a fix.

## How to report

| Step | What to do |
| --- | --- |
| **1** | Open the repository's **Security** tab on GitHub |
| **2** | Choose **Report a vulnerability** |
| **3** | Include `zeroturn version`, your operating system, and the steps that show the problem |
| **4** | Remove real credentials from anything you attach |

You will get an acknowledgement **within seven days**, and you will be told whether the report is accepted and when a fix is expected.

## Supported versions

| Version | Fixes |
| --- | --- |
| The latest commit on `main` | Yes |
| Anything older | Not until the first release |

## Most useful findings

These are the promises ZeroTurn makes. A way around any of them is worth reporting.

| Promise | A finding would look like |
| --- | --- |
| **Repository commands need your approval** | A `.zeroturn.json` whose commands run without `zeroturn verify --approve` |
| **Strict mode needs local approval** | A committed configuration that denies subagents on a machine where nobody approved Strict |
| **Credential values never appear** | A key visible in output, a log, JSON, or ZeroTurn's own records |
| **Private content is never stored** | A prompt, a response, transcript content, or a repository path in the records |
| **ship stages only what you name** | An unnamed file staged, a force push, a rebase, a protected branch accepted, or Git hooks skipped |
| **ship stays inside the repository** | A path outside it accepted, through a symbolic link or otherwise |
| **integrate touches only its own entries** | Settings removed or changed that ZeroTurn did not write |

## What ZeroTurn does not protect against

> [!NOTE]
> Knowing the limits is part of using it safely.

- **The credential scan looks for high confidence patterns.** It is not a replacement for a dedicated secret scanner, and a credential in an unusual format can pass it.
- **The gate depends on the harness.** It can only ask or deny where the harness honours the answer, which is why `zeroturn doctor --compat --live` exists.
- **It runs as you.** ZeroTurn has your permissions and does not defend against other software running as your user.
