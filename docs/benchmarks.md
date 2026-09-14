# Benchmarks

These are measurements from one machine, not a promise about yours. Repeat them with `go run ./scripts/bench <path to zeroturn> 50` from the repository root.

## Method

`scripts/bench` starts the real executable as a separate process, the way a harness does, and records wall clock time from start to exit. Each command runs once to warm up, then 50 times. The table reports the median, the minimum, and the maximum, because process start dominates these numbers and varies.

The benchmark points ZeroTurn at a temporary state directory, so it never reads or writes the records of a real session.

| Setting | Value |
| --- | --- |
| Machine | Intel Core i7-8750H at 2.20 GHz, 12 processors |
| Operating system | macOS 15.7.9 |
| Executable | release build, 2.5 MB, built with `-trimpath -ldflags "-s -w"` |
| Go | 1.17.6 |
| Runs | 50 for each command, after one warm up run |
| Date | 2026-09-11 |

## Results

| Command | Median | Minimum | Maximum | What it does |
| --- | --- | --- | --- | --- |
| startup | 6.3 ms | 5.9 ms | 7.8 ms | `zeroturn version`, process start only |
| status line | 8.0 ms | 7.3 ms | 10.5 ms | reads one status line payload, updates the record, prints the line |
| subagent gate | 7.7 ms | 7.0 ms | 9.1 ms | reads one `PreToolUse` event, evaluates the policy, writes the decision |
| policy check | 24.1 ms | 21.3 ms | 34.8 ms | reads the configuration and every recent session record |
| repository status | 40.3 ms | 38.7 ms | 51.2 ms | the above plus `git` for the branch |

Policy evaluation on its own, without process start, measured with `go test -bench BenchmarkEvaluate ./internal/policy`:

```text
BenchmarkEvaluate-12    481686    2276 ns/op    928 B/op    17 allocs/op
```

## Several hooks at once

A turn that ends fires more than one hook at the same moment: the status line repaints, `Stop` arrives, and a subagent reports that it finished. Each is a separate process, and they contend for the state lock.

Twenty events started at the same instant, on 2026-09-12:

| Measurement | Result |
| --- | --- |
| All twenty finished in | 54 ms |
| Average per event | 2.7 ms |

Before the lock was changed, the same test took 89 ms, and sixty writers inside one process took 1.5 seconds rather than 0.2. The lock waited on a fixed 25 ms sleep, so a waiter stayed asleep while the lock was already free. It now waits 200 microseconds, doubling to at most 2 ms, which matches how long a hook actually holds it, about one millisecond.

## How much these numbers move

They move with the load on the machine. The table above was taken on an idle machine. The same commands measured while the test suite was running gave 10.0 ms for the status line and 11.2 ms for the gate, against 7.5 and 7.7 when idle. Treat the figures as the shape of the cost, a few milliseconds of process start plus a small file read and write, rather than as a guarantee.

## Scanning a file for credentials

The credential guard runs while a developer waits for a file to be read, so the cost of scanning matters.

| Content | Rate | A 1 MB file |
| --- | --- | --- |
| Ordinary source, no anchor words | 27 MB/s | 37 ms |
| Full of words like password and key, none of them credentials | 3.7 MB/s | 270 ms |

Each detector carries literal anchor strings, and a line holding none of them is never handed to a regular expression. Before that, every detector ran its expression on every line: 0.5 MB/s, which would have held a read for eight seconds on a large file. Combining the detectors into one expression made it worse, not better, because the engine handles a large alternation with captures poorly.

Files over 1 MB are not scanned. Credentials live in small files, and the guard is not worth a wait of a second on a log.

## Against the targets

| Target | Result |
| --- | --- |
| Status line under 20 ms after startup | Met. 8.0 ms in total, of which about 6 ms is process start. |
| Core startup under 30 ms | Met. 6.3 ms. |
| Policy evaluation under 10 ms | Met by a wide margin. 2.3 microseconds. |
| Cached repository status under 50 ms | Met. 40.3 ms, which includes starting `git` once. |
| Binary under 15 MB | Met. 2.5 MB. |
| No idle process, no network activity | Met by design. ZeroTurn runs only when a command or hook starts it, and makes no network request of its own. |

## What made it faster

Two changes, on 2026-09-11:

- The status line and the gate no longer start `git`. They find the repository by looking for a `.git` entry on the filesystem, which the status line needs on every repaint.
- The gate folds the event and the decision into a single state write, and session records are renamed into place without forcing a disk flush. A record is rewritten many times a minute and a lost record is replaced by the next event, so the flush cost was not worth paying. The write is still atomic and still taken under the state lock.

Before those changes, on the same machine, the status line took 31.3 ms and the gate 53.1 ms.

A third change, on 2026-09-14: the credential patterns are compiled when
one is first needed rather than when the package is loaded. The
executable starts fresh for every hook call, and a status line repaint
never scans anything, so it was compiling thirty expressions, and thirty
more loose copies of them for redaction, to use none of them. The
anchors mean that even a scan usually compiles none: a literal string
has to appear in the line before its expression is built.

That change was measured on a different machine from the table above, so
the figures are not comparable with it, only with each other:

| Measured on Apple Silicon, 18 processors, Go 1.27.1, plain `go build`, 200 runs | Before | After |
| --- | --- | --- |
| startup | 2.6 ms | 2.35 ms |
| status line | 3.15 ms | 2.9 ms |

About a quarter of a millisecond, which is what compiling those
expressions costs, and about eight percent of each. Two rounds,
alternating between the two executables, gave the same result.

## Limits of these numbers

One machine, one operating system, one storage device. A slower disk or a busy machine will change them. The status line figure is the cost of ZeroTurn's own work, not of the harness that draws the line. `repository status` depends on the size of the repository, because `git` reads it.
