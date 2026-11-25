# Lab: a first execution trace

`runtime/trace` records what the scheduler did: every goroutine that became runnable, ran,
blocked, and on which P and M. This lab runs a pool of `-workers` goroutines over `-jobs`
CPU-bound jobs (about a millisecond each) under the tracer, and writes the trace to `-o`.
Each job is a region named `hash` inside a task named `pool`.

`go tool trace` opens a web viewer. The compose project publishes no ports, so this lab
reads the trace in text: a scheduler latency profile (`-pprof=sched`) and the parsed events
(`-d=parsed`).

## Run it

```bash
docker compose run --rm lab sh -c 'go build -o /tmp/tr ./labs/trace &&
    /tmp/tr -workers 2 -o /tmp/w2.trace && /tmp/tr -workers 16 -o /tmp/w16.trace &&
    go tool trace -pprof=sched /tmp/w16.trace > /tmp/w16.sched &&
    go tool pprof -top -nodecount=3 /tmp/w16.sched &&
    go tool trace -d=parsed /tmp/w16.trace | grep -m1 -A5 "G=1 StateTransition.*Waiting->Runnable"'
docker compose exec dev go test -race ./labs/trace
```

## What it printed

On 2026-11-25, `lab` service (2 CPUs), Go 1.27.4:

```
$ /tmp/tr -workers 2 -o /tmp/w2.trace
workers=2 jobs=400 GOMAXPROCS=2 in 232ms, trace=/tmp/w2.trace
$ /tmp/tr -workers 16 -o /tmp/w16.trace
workers=16 jobs=400 GOMAXPROCS=2 in 226ms, trace=/tmp/w16.trace
$ go tool trace -pprof=sched /tmp/w2.trace > /tmp/w2.sched && go tool pprof -top -nodecount=3 /tmp/w2.sched
Main binary filename not available.
Type: delay
Showing nodes accounting for 113.62ms, 99.71% of 113.95ms total
Dropped 12 nodes (cum <= 0.57ms)
      flat  flat%   sum%        cum   cum%
  113.62ms 99.71% 99.71%   113.62ms 99.71%  runtime.chanrecv2
         0     0% 99.71%   113.69ms 99.77%  main.run.func1
         0     0% 99.71%   113.71ms 99.79%  sync.(*WaitGroup).Go.func1
$ go tool trace -pprof=sched /tmp/w16.trace > /tmp/w16.sched && go tool pprof -top -nodecount=3 /tmp/w16.sched
Main binary filename not available.
Type: delay
Showing nodes accounting for 3062803.07us, 100% of 3062819.20us total
Dropped 3 nodes (cum <= 15314.10us)
Showing top 3 nodes out of 7
      flat  flat%   sum%        cum   cum%
2913804.03us 95.13% 95.13% 2913804.03us 95.13%  runtime.chansend1
148992.90us  4.86%   100% 148992.90us  4.86%  runtime.chanrecv2
    6.14us 0.0002%   100% 2913810.18us 95.13%  main.run
== parsed
M=47 P=1 G=1 StateTransition Time=21034614676096 GoID=16 Waiting->Runnable Reason=""
Stack=
	runtime.chansend1 @ 0x413db6
		/usr/local/go/src/runtime/chan.go:161
	main.run @ 0x4beb2c
		/src/labs/trace/main.go:78
Waiting->Runnable: 404  Running->Runnable: 0
```

Sixteen workers finished in the same time as two (226 ms and 232 ms): there are 2 Ps. What
changed is the waiting. The scheduler latency profile adds up, for every goroutine, the time
between becoming runnable and running: 114 ms with 2 workers, 3.06 s with 16. That is more
than the run itself, because 14 workers wait for a P at the same time.

## Reading the stack in the sched profile

The 3.06 s show up under `runtime.chansend1`, and only `main` sends on the channel. The
profile records the stack of the event that made the goroutine runnable (`startEv.Stack()`
in `computePprofSched`, `cmd/trace/pprof.go`). A worker blocked on `range queue` becomes
runnable when `main` sends it a job, so the event happens on `main`, with `main`'s stack:
`G=1` above is `main`, `GoID=16` is the worker that waited.

The stack says who woke the goroutine up, not where the goroutine waited. No transition in
the 16-worker trace was `Running->Runnable`: no goroutine was preempted.
