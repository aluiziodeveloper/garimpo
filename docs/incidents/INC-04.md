# INC-04: ten thousand goroutines are not ten thousand threads, until they are

- **Date:** 2026-11-25
- **Module:** 04, The runtime inside
- **Reproduction:** `labs/threads`

## Summary

A program started 10,000 goroutines, each waiting 2 s in a blocking system call. Goroutines
that wait on a channel or a timer cost no thread, so the design looked safe. A blocking
call is different: each one held an OS thread, the runtime kept starting threads for the
other goroutines, and at 10,000 threads it stopped the program.

## Impact

Measured with `labs/threads`, `lab` service (2 CPUs), each goroutine waiting 2 s:

| Goroutines | How they wait | Threads | Result |
|---|---|---|---|
| 10,000 | channel | 4 | done in 2.004 s |
| 10,000 | `time.Sleep` | 4 | done in 2.014 s |
| 1,000 | blocking call | 1,003 | done in 2.106 s |
| 5,000 | blocking call | 5,003 | done in 2.501 s |
| 10,000 | blocking call | over 10,000 | crash |

The crash, the first two of about 160,000 lines:

```
runtime: program exceeds 10000-thread limit
fatal error: thread exhaustion
```

## Cause

The scheduler runs goroutines (G) on OS threads (M), and a thread runs Go code only while it
holds a P; there are `GOMAXPROCS` Ps. A goroutine that blocks on a channel, a timer or the
network parks: it leaves its M and P free for another goroutine. A goroutine in a blocking
system call cannot park. Its M stays in the call, the runtime hands the P to another M, and
starts a new M when none is idle.

The `runtime` package documentation: "There is no limit to the number of threads that can
be blocked in system calls on behalf of Go code; those do not count against the GOMAXPROCS
limit." The limit that does exist is the crash: `runtime/debug.SetMaxThreads`, "The initial
setting is 10,000 threads."

The crash depends on how long the calls block. With 500 ms per call, the same 10,000
goroutines did not crash: the first calls returned before the runtime reached 10,000
threads (6,969 when sampled).

## Fix

Bound the blocking calls, not the goroutines. A buffered channel used as a semaphore lets at
most N goroutines into the call; the others wait on the channel, which costs no thread:

| Goroutines | Limit | Threads | Time |
|---|---|---|---|
| 10,000 | 500 | 504 | 4.053 s (200 ms per call, 20 rounds of 500) |

Raising `SetMaxThreads` is not a fix: it moves the crash to the operating system's own
limits.

## Regression test

`TestLimitBoundsTheThreadsOfBlockingCalls` (`labs/threads/main_test.go`) starts 200
goroutines in blocking calls with a limit of 20 and fails if the process has more than 70
threads. With the limit ignored:

```
--- FAIL: TestLimitBoundsTheThreadsOfBlockingCalls (0.06s)
    main_test.go:50: limit 20: 203 threads, want at most 70
```

## Where Garimpo meets it

Garimpo talks to Ollama over the network, which parks. Reading the corpus does not: a
comment in `os/file_unix.go` says the network poller does not support "disk files on Linux
systems", so each read of a corpus file is a blocking call. The ingestion pipeline (module
07) reads with a bounded number of workers, never one goroutine per file.
