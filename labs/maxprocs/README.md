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
LAB_CPUS=4 docker compose run --rm lab go run ./labs/maxprocs -jobs 4000
LAB_CPUS=1.5 docker compose run --rm lab go run ./labs/maxprocs -jobs 4000
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
`GODEBUG=containermaxprocs=0`, with a `go.mod` that says `go 1.24` or older, and with any Go
before 1.25. Twenty threads burn the 2-CPU quota early in each 100 ms period and the kernel
stops the whole container until the next one:

```
NumCPU: 20  GOMAXPROCS: 20  pool: gomaxprocs = 20 workers
4000 jobs in 3.54s
throttled: 36 periods, 57.764s summed over CPUs
```

The throttled time is the kernel's `throttled_usec`, added up over the CPUs the threads were
waiting on, which is why it is larger than the run itself.

In three earlier runs the same day: 2.265 to 2.315 s with no throttling against 3.474 to
4.12 s with 35 to 42 throttled periods. A copy of the lab with `go 1.24` in its `go.mod`, built
by the same Go 1.27.4, printed `GOMAXPROCS: 20` and 39 throttled periods.

## The quota (module 01 challenge)

The lab also prints the quota it reads from `/sys/fs/cgroup/cpu.max` (`readQuota` in
`main.go`): `"<quota> <period>"` in microseconds, or `max` for no limit. Real runs on
2026-11-25, same machine:

```
NumCPU: 20  GOMAXPROCS: 2  pool: gomaxprocs = 2 workers
quota: 2 CPUs
4000 jobs in 2.535s
throttled: 0 periods, 0s summed over CPUs
```

With `LAB_CPUS=1.5` the quota is not a whole number, and `GOMAXPROCS` is still 2: the runtime
"rounds up" a fractional limit and "will never set GOMAXPROCS less than 2 unless the logical
CPU count or CPU affinity mask count are below 2" (package `runtime`, *Default*). Two threads
on 1.5 CPUs get throttled:

```
NumCPU: 20  GOMAXPROCS: 2  pool: gomaxprocs = 2 workers
quota: 1.5 CPUs
4000 jobs in 3.234s
throttled: 32 periods, 1.481s summed over CPUs
```

In `dev`, with no limit, the line reads `quota: none`.

## The rule in code

Size CPU-bound pools with `runtime.GOMAXPROCS(0)` (`workers()` in `main.go`), never with
`runtime.NumCPU()`. `TestWorkersFollowsGOMAXPROCS` sets `GOMAXPROCS` to 3 and fails if the
pool follows `NumCPU`:

```
--- FAIL: TestWorkersFollowsGOMAXPROCS (0.00s)
    main_test.go:24: workers() = 20, want 3 (GOMAXPROCS, not NumCPU = 20)
```

Ollama is not a Go program and does not do this: it sizes its threads from the host CPUs, so
Garimpo sends `num_thread` on every request (see the main README).
