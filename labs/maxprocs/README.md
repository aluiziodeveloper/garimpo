# Lab: GOMAXPROCS in a container

How many CPUs does a Go program see inside a container limited to 2? The machine has more.
This lab runs the same CPU-bound work (SHA-256 chains) and reads the container's CPU
throttling from `/sys/fs/cgroup/cpu.stat` (cgroup v2) before and after.

The `lab` service in `compose.yaml` is the `dev` image with a CPU quota: `LAB_CPUS`, 2 by
default. It belongs to the `labs` profile, so it only runs when you ask for it.

## Run it

```bash
docker compose run --rm lab go run ./labs/maxprocs -jobs 4000
docker compose run --rm lab go run ./labs/maxprocs -jobs 4000 -pool numcpu
docker compose run --rm -e GODEBUG=containermaxprocs=0 lab go run ./labs/maxprocs -jobs 4000
docker compose run --rm -e GOMAXPROCS=8 lab go run ./labs/maxprocs -jobs 4000
LAB_CPUS=4 docker compose run --rm lab go run ./labs/maxprocs -jobs 4000
docker compose exec dev go run ./labs/maxprocs -jobs 4000   # dev has no CPU limit
docker compose exec dev go test -race ./labs/maxprocs
```

## What it shows

Real runs on 2026-11-25, on a machine with 20 CPUs (Go 1.27.4). Times change from run to run;
the CPU counts do not.

With the default (Go 1.25 or newer, `go 1.27.4` in `go.mod`), `GOMAXPROCS` follows the quota:

```
NumCPU: 20  GOMAXPROCS: 2  pool: gomaxprocs = 2 workers
4000 jobs in 2.466s
throttled: 0 periods, 0s summed over CPUs
```

`runtime.NumCPU` still reports the machine. Sizing a pool with it starts 20 goroutines, but
only `GOMAXPROCS` of them run at once, so the cost is small:

```
NumCPU: 20  GOMAXPROCS: 2  pool: numcpu = 20 workers
4000 jobs in 2.385s
throttled: 4 periods, 9ms summed over CPUs
```

The incident (INC-01) is `GOMAXPROCS` itself seeing 20 CPUs. That is what happens with
`GODEBUG=containermaxprocs=0`, with a `go.mod` that says `go 1.24` or older, with any Go
before 1.25, and with `GOMAXPROCS` set by hand (the environment variable or a call to
`runtime.GOMAXPROCS`), which takes precedence over the quota. Twenty threads burn the 2-CPU quota early in each 100 ms period and the kernel
stops the whole container until the next one:

```
NumCPU: 20  GOMAXPROCS: 20  pool: gomaxprocs = 20 workers
4000 jobs in 3.54s
throttled: 36 periods, 57.764s summed over CPUs
```

A `GOMAXPROCS` set by hand wins over the quota too (`-e GOMAXPROCS=8`, same day):

```
NumCPU: 20  GOMAXPROCS: 8  pool: gomaxprocs = 8 workers
4000 jobs in 2.833s
throttled: 29 periods, 16.077s summed over CPUs
```

The throttled time is the kernel's `throttled_usec`, added up over the CPUs the threads were
waiting on, which is why it is larger than the run itself.

In three earlier runs the same day: 2.265 to 2.315 s with no throttling against 3.474 to
4.12 s with 35 to 42 throttled periods. A copy of the lab with `go 1.24` in its `go.mod`, built
by the same Go 1.27.4, printed `GOMAXPROCS: 20` and 39 throttled periods.

## The rule in code

Size CPU-bound pools with `runtime.GOMAXPROCS(0)` (`workers()` in `main.go`), never with
`runtime.NumCPU()`. `TestWorkersFollowsGOMAXPROCS` sets `GOMAXPROCS` to 3 and fails if the
pool follows `NumCPU`:

```
--- FAIL: TestWorkersFollowsGOMAXPROCS (0.00s)
    main_test.go:24: workers() = 20, want 3 (GOMAXPROCS, not NumCPU = 20)
```

Ollama does not do this. Its server is written in Go, but inference runs in a C/C++ library
that sizes its threads from the host CPUs and never looks at `GOMAXPROCS`, so Garimpo sends
`num_thread` on every request (see the main README).
