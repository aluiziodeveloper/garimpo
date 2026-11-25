# Lab: when more goroutines do not help

A goroutine is cheap, not free. This lab measures two costs:

- **Memory.** `go run` parks `-n` goroutines on a channel and reports how much the stack
  memory in use (`runtime.MemStats.StackInuse`) grew per goroutine.
- **Time.** `BenchmarkPool` runs 512 jobs on 1, 2, 4, 16 and 512 workers. A job is a chain
  of SHA-256 sums: 2,000 of them (about 110 µs) or a single one (far less than starting a
  goroutine).

The benchmarks run in the `bench` service: the `dev` image pinned to two CPUs with
`cpuset` (`BENCH_CPUSET`, `2,4` by default), so `GOMAXPROCS` is 2 and the host does not
throttle the run in the middle.

## Run it

```bash
docker compose run --rm bench go run ./labs/goroutine-cost
docker compose run --rm bench sh -c 'go test -run "^$" -bench . -count 10 ./labs/goroutine-cost > /tmp/b.txt &&
    go tool -modfile=tools/go.mod benchstat -col /workers /tmp/b.txt'
docker compose exec dev go test -race ./labs/goroutine-cost
```

## What it printed

On 2026-11-25, `bench` service (CPUs 2 and 4 of an i7-13700H), Go 1.27.4:

```
100000 parked goroutines: 2048 bytes of stack each, 195.3 MiB in all
```

```
goos: linux
goarch: amd64
pkg: github.com/aluiziodeveloper/garimpo/labs/goroutine-cost
cpu: 13th Gen Intel(R) Core(TM) i7-13700H
                   │      1      │                  2                  │                  4                  │                 16                  │                  512                  │
                   │   sec/op    │   sec/op     vs base                │   sec/op     vs base                │   sec/op     vs base                │    sec/op     vs base                 │
Pool/rounds=2000-2   56.05m ± 1%   29.14m ± 2%  -48.00% (p=0.000 n=10)   29.58m ± 3%  -47.22% (p=0.000 n=10)   29.40m ± 3%  -47.55% (p=0.000 n=10)    29.60m ± 4%   -47.19% (p=0.000 n=10)
Pool/rounds=1-2      38.98µ ± 3%   29.97µ ± 2%  -23.12% (p=0.000 n=10)   31.13µ ± 8%  -20.14% (p=0.000 n=10)   32.66µ ± 6%  -16.22% (p=0.000 n=10)   113.95µ ± 2%  +192.34% (p=0.000 n=10)
geomean              1.478m        934.6µ       -36.77%                  959.6µ       -35.08%                  979.8µ       -33.71%                   1.837m        +24.25%
goos: linux
```

With the heavy job, the second worker halves the time (−48%), because there are two CPUs.
Four, 16 or 512 workers take the same 29 ms as two: once both CPUs are busy, more
goroutines only wait for a P. With the cheap job, starting a goroutine costs more than the
job: 512 workers, one per job, are 192% slower than a single worker.

The rule of thumb: for CPU-bound work, size the pool by `GOMAXPROCS`; for work that waits
(network, disk), the number that matters is how many waits the other side can take, and
that is a limit you measure (modules 07 and 09).
