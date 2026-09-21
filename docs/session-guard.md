# Session Guard

Session Guard reads what the harness already reports: context use, the five hour and seven day usage windows, and session duration. It adds one thing the harness does not report, the number of subagents started and currently running, counted from the harness's own start and stop events.

```text
ZT  ctx 82%  5h 81%  7d 47%  session 3h12m  agents 2  ask
     |        |       |       |             |         |
     |        |       |       |             |         what happens to the next subagent
     |        |       |       |             subagents running now, counted by ZeroTurn
     |        |       |       how long this session has been open
     |        |       seven day usage window
     |        five hour usage window
     context used in this conversation
```

Before a new subagent starts, the gate compares those values with your thresholds. Each measurement is checked against its own limit. Percentages from different measurements are never added together. A value the harness did not send is left out, and is never shown as zero.

Session Guard needs a harness that draws a status line, which means a terminal. The [README](../README.md#where-each-part-works) has the table of what works where.

## Modes

![Three modes side by side. observe, the default, in green: shows the session condition and counts subagents, never blocks or asks. confirm, in yellow: asks before a new subagent once a threshold is crossed, you decide each time. strict, in red: denies a new subagent at critical context, other thresholds ask, and you approve it first.](img/modes.svg)

```sh
zeroturn policy show
zeroturn policy check                      # evaluate the current session, change nothing
zeroturn policy set guard.mode confirm
zeroturn policy set guard.context.confirm 85
zeroturn policy reset
```

Strict mode is never switched on by a file alone. `zeroturn policy set guard.mode strict` shows the exact policy, explains what can be blocked and how to turn it off, checks the installed harness, runs a compatibility test, and asks you to confirm. A repository that commits `"mode": "strict"` gets Confirm behavior on every machine where nobody has approved Strict.

## Rate, not only level

Seventy five percent of the five hour window is fine when the window resets in ten minutes and a problem when it resets in four hours. A threshold cannot tell those apart, so the gate also measures the rate.

ZeroTurn keeps the first five hour reading of the window that is running now, and from it works out how fast the window is being used and where that lands when it resets:

```text
New subagent requires approval. Five hour usage is 41% and rising 19% an hour, with 3h20m left before it resets.
```

Forty one percent crosses no threshold. The trajectory does.

The estimate is treated as an estimate. It never denies, even in Strict mode. It is ignored when the rate has been measured over less than fifteen minutes, when usage is not rising, when the window has already reset, and when the reading belongs to a window that has rolled over. Turn it off with `zeroturn policy set guard.limits.projection off`.

## Thresholds from what actually happened

The default thresholds are starting points, not measurements. `zeroturn policy tune` reads what the gate asked and what you did next, and proposes thresholds from that.

```text
$ zeroturn policy tune
ZEROTURN POLICY TUNE
Observations from the last 7 days in this repository.

sessions observed      4
asked                  9
approved after asking  8
denied                 0

fiveHour  (guard.limits.fiveHourWarn, now 75)
  all 6 asks were approved, the highest at 81
  zeroturn policy set guard.limits.fiveHourWarn 82
```

Approval is inferred from what the harness reports: a subagent starting after an ask means you approved it. A threshold you always approve is asking too early. One you often decline is doing its job, and the command says so. It needs five observations before it suggests anything, and it never changes a setting itself.

## Speed

Measured over 50 runs on an Apple M4, including process start: status line 7.5 ms, gate 7.7 ms, startup 6.3 ms, binary 2.5 MB. Neither the status line nor the gate makes a network request or starts a background process.
