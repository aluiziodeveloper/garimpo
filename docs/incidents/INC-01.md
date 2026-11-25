# INC-01: the binary that sees 20 CPUs in a 2-CPU container

- **Date:** 2026-11-25
- **Module:** 01, Environment and workflow
- **Reproduction:** `labs/maxprocs`

## Summary

A CPU-bound Go program, limited to 2 CPUs by its container, ran with `GOMAXPROCS: 20`. It
started 20 threads for a quota of 2, the kernel throttled the container in most scheduling
periods, and the same work took about 1.4 to 1.8 times longer.

## Impact

Measured with `labs/maxprocs`, 4,000 jobs, 2-CPU quota (`LAB_CPUS=2`):

| | GOMAXPROCS | Time | Throttled periods |
|---|---|---|---|
| Healthy (default) | 2 | 2.265 to 2.466 s | 0 |
| Incident (`GODEBUG=containermaxprocs=0`) | 20 | 3.474 to 4.12 s | 35 to 42 |

Four runs of each. One incident run, as printed:

```
NumCPU: 20  GOMAXPROCS: 20  pool: gomaxprocs = 20 workers
4000 jobs in 3.54s
throttled: 36 periods, 57.764s summed over CPUs
```

## Cause

`GOMAXPROCS` is how many threads run Go code at once. Before Go 1.25 it defaulted to the
number of CPUs of the machine, and a CPU quota set by the container (a cgroup limit) did not
change it. Since Go 1.25 the runtime reads the quota and lowers `GOMAXPROCS` to match, but
the old behaviour comes back in three cases:

1. a binary built with Go older than 1.25;
2. a main module whose `go.mod` says `go 1.24` or older (a copy of the lab with `go 1.24`,
   built by Go 1.27.4, printed `GOMAXPROCS: 20` and 39 throttled periods);
3. `GODEBUG=containermaxprocs=0`.

With 20 threads and a quota of 2 CPUs, the container uses up its 200 ms of CPU early in each
100 ms period and waits, stopped, until the next one.

`runtime.NumCPU()` is not the cause here. It still reports 20 in the healthy case, and a pool
of 20 goroutines under `GOMAXPROCS: 2` cost little: 2.385 s and 4 throttled periods.

## Fix

- Keep the `go` directive at 1.25 or newer and never set `containermaxprocs=0`
  ([ADR 0001](../adr/0001-toolchain-and-workflow.md), decision 6).
- Size CPU-bound pools with `runtime.GOMAXPROCS(0)` (`workers()` in `labs/maxprocs`), so the
  pool follows whatever the runtime decided.

## Regression test

`TestWorkersFollowsGOMAXPROCS` (`labs/maxprocs/main_test.go`) sets `GOMAXPROCS` to 3 and fails
if the pool follows `NumCPU`:

```
--- FAIL: TestWorkersFollowsGOMAXPROCS (0.00s)
    main_test.go:24: workers() = 20, want 3 (GOMAXPROCS, not NumCPU = 20)
```

It runs in CI with the race detector, like every test.

## Same symptom, different program

Ollama is not a Go program and does not read the quota: with `cpus: 4` it started
`n_threads = 10` and took 1.74 to 1.90 s per generation, against 0.62 to 0.65 s with
`num_thread=4`. Garimpo sends `num_thread` on every request (ADR 0001, decision 5).
