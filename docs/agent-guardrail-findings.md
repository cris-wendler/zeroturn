# Two ways a hook based guardrail fails on Claude Code

Findings from building and running a policy guard on Claude Code's hook interface, September 2026, against versions 2.1.257, 2.1.265 and 2.1.270. Both were found by running the guard rather than by reading the documentation, and both have a reproduction.

The interface works as documented. Neither of these is a defect in the harness. They are the difference between what a hook can decide and what a person ends up experiencing, and anyone building a control on this interface inherits both.

## 1. A decision of `ask` can be disabled from inside its own prompt

**What a guard author expects.** A `PreToolUse` hook matched to `Agent` returns `permissionDecision: ask` with a reason. The harness shows the reason and the person answers. The control stays in force for the rest of the session.

**What happens.** The harness renders the prompt with three answers, not two:

```
Hook PreToolUse:Agent requires confirmation for this tool:
New subagent requires approval. Five hour usage is 29% and seven day usage is 4%.

Do you want to proceed?
❯ 1. Yes
  2. Yes, and don't ask again for Explore commands in /Users/.../project
  3. No
```

The second answer records a permission for that tool in that directory. Every later request of the same kind is approved without the person being asked again.

**Why it matters more than it looks.** The offer appears at the one moment the control is inconvenient, which is the moment a person is most willing to take it. It is permanent rather than scoped to the session. And the hook is not told: the guard sees no further decisions to make and cannot distinguish a session where its thresholds were never crossed from one where it was switched off in the first hour. A report built from what the guard recorded shows the same thing in both cases, which is nothing.

**Reproduction.** Install a `PreToolUse` hook on `Agent` that returns `ask`. Trigger it. Answer with the second option. Trigger it again.

**What to do about it.** A control that must hold cannot be built on `ask` alone, because `ask` is a request that the person can permanently withdraw. `deny` is not withdrawable the same way, so a policy that must hold has to deny and accept the cost of denying. A guard that stays on `ask` should say in its own documentation that it is advisory, and should not report an absence of prompts as an absence of risk.

## 2. A control that reads session state fails open, silently, outside a terminal

**What a guard author expects.** Context use and the usage windows are available to a hook, so a policy can be written against them.

**What happens.** They are not carried by any hook payload. The only carrier is the status line, which the harness invokes to draw a line at the bottom of a terminal. An editor extension draws no such line and never invokes the command. Every hook still fires, so the integration looks correct from every other angle: events arrive, counts accumulate, other guards work.

The policy engine then evaluates against measurements it never received. It cannot cross a threshold it cannot read, so it allows, quietly, for as long as the session runs.

**How it was found.** Not by reasoning about the code. By reading the stored records after three days of use and noticing that every one of them held a harness name, subagent counts and credential warnings, and none held a context value, a usage window, a duration or a model. Seven sessions across two machines, including the longest one. Feeding the same code a recorded status line payload produced all of them, which ruled out the code. Running the same integration in a terminal produced them too.

**Why it matters.** A control that fails open is worse than no control when nothing reports that it failed. Everyone who installed it in an editor had a guard that had never evaluated anything, and no signal distinguished that from a quiet session.

**Reproduction.** Install a status line command and a `PreToolUse` hook. Run the harness as an editor extension. Observe that the hook fires and the status line command does not.

**What to do about it.** A control has to be able to tell the difference between measuring nothing and measuring zero. The guard now compares the sessions it has recorded and reports how many carried measurements, which turns a silent failure into a visible one:

```
WARN  session data   6 of 7 recent sessions for this repository carried no
                     context or usage values.
```

## What the two have in common

Both are failures of a control's assumptions about its environment rather than of its logic, and neither is visible from inside the control. The first assumes an answer it is given stays given. The second assumes an input it is designed around will arrive. In both cases the guard reports success, because from where it sits nothing went wrong.

A guardrail built on an agent harness needs to state which of its inputs are optional, and to report when an input it depends on has never arrived.
