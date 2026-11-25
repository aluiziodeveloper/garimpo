# Lab: the scheduler through schedtrace

`GODEBUG=schedtrace=500` makes the runtime print one line about its scheduler every 500 ms.
This lab gives it something to schedule: `-workers` goroutines that either hash on the CPU
(`-kind cpu`) or sleep in 10 ms steps (`-kind wait`), for `-for` (2 s by default).

## Run it

Build first, and set `GODEBUG` only on the binary:

```bash
docker compose run --rm lab sh -c 'go build -o /tmp/schedtrace ./labs/schedtrace &&
    GODEBUG=schedtrace=500 /tmp/schedtrace -workers 8 -kind cpu'
docker compose run --rm lab sh -c 'go build -o /tmp/schedtrace ./labs/schedtrace &&
    GODEBUG=schedtrace=500 /tmp/schedtrace -workers 8 -kind wait'
docker compose exec dev go test -race ./labs/schedtrace
```

## What it printed

On 2026-11-25, `lab` service (2 CPUs), Go 1.27.4. Eight CPU workers, then eight that wait:

```
SCHED 0ms: gomaxprocs=2 idleprocs=1 threads=4 spinningthreads=0 needspinning=0 idlethreads=1 runqueue=0 [ 0 0 ] schedticks=[ 1 3 ]
SCHED 501ms: gomaxprocs=2 idleprocs=0 threads=4 spinningthreads=0 needspinning=1 idlethreads=1 runqueue=4 [ 3 0 ] schedticks=[ 26 30 ]
SCHED 1011ms: gomaxprocs=2 idleprocs=0 threads=4 spinningthreads=0 needspinning=1 idlethreads=1 runqueue=2 [ 2 2 ] schedticks=[ 51 56 ]
SCHED 1518ms: gomaxprocs=2 idleprocs=0 threads=4 spinningthreads=0 needspinning=1 idlethreads=1 runqueue=1 [ 3 2 ] schedticks=[ 76 81 ]
workers=8 kind=cpu GOMAXPROCS=2 hashes=71210000
----
SCHED 0ms: gomaxprocs=2 idleprocs=0 threads=4 spinningthreads=1 needspinning=0 idlethreads=0 runqueue=0 [ 1 0 ] schedticks=[ 1 0 ]
SCHED 502ms: gomaxprocs=2 idleprocs=2 threads=5 spinningthreads=0 needspinning=0 idlethreads=3 runqueue=0 [ 0 0 ] schedticks=[ 146 153 ]
SCHED 1011ms: gomaxprocs=2 idleprocs=2 threads=5 spinningthreads=0 needspinning=0 idlethreads=3 runqueue=0 [ 0 0 ] schedticks=[ 334 249 ]
SCHED 1521ms: gomaxprocs=2 idleprocs=2 threads=5 spinningthreads=0 needspinning=0 idlethreads=3 runqueue=0 [ 0 0 ] schedticks=[ 394 444 ]
workers=8 kind=wait GOMAXPROCS=2 hashes=0
```

How to read a line, from `schedtrace` in `runtime/proc.go`:

| Field | Meaning |
|---|---|
| `gomaxprocs` | How many Ps, the scheduler's slots for running Go code |
| `idleprocs` | Ps with nothing to run |
| `threads` | Ms, the OS threads the runtime created (`mcount()`) |
| `spinningthreads`, `needspinning` | Threads looking for work, and whether one more is wanted |
| `idlethreads` | Threads parked with no P |
| `runqueue` | Goroutines in the global run queue |
| `[ a b ]` | Goroutines in each P's local run queue |
| `schedticks=[ a b ]` | Each P's `schedtick`, "incremented on every scheduler call" |

With 8 CPU workers and 2 Ps, two run and six wait: `runqueue=2 [ 2 2 ]` at 1011 ms. Each P
schedules about 25 times per 500 ms, one switch every 20 ms or so: the runtime preempts a
goroutine that ran for 10 ms (`forcePreemptNS` in `runtime/proc.go`), but only when
`sysmon` notices, and `sysmon` sleeps up to 10 ms between checks. With 8 waiting workers, both Ps are idle most of the time
(`idleprocs=2`) and schedule far more often, once per wake-up.

## The trap: `GODEBUG` on `go run`

`docker compose run --rm -e GODEBUG=schedtrace=500 lab go run ./labs/schedtrace` traces the
`go` command and the compiler too. On 2026-11-25 it printed 11 `SCHED` lines for one 2-second
run, four of them at `0ms`, from several processes, with the build output in between.
