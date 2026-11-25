# Lab: goroutines and OS threads (INC-04)

Ten thousand goroutines are not ten thousand threads, as long as they wait the way Go can
park: on a channel, a timer, the network. A goroutine inside a blocking system call (a read
from a slow disk, a cgo call) keeps its thread until the call returns, and the runtime
starts another thread to keep the other goroutines running. Past 10,000 threads, the
program dies.

This lab starts `-n` goroutines that wait `-hold` (2 s by default) in one of three ways and
counts the process threads (`Threads:` in `/proc/self/status`) halfway through the wait.
`-wait syscall` uses `syscall.Nanosleep`, a blocking call that stands in for slow I/O.
`-limit` caps how many goroutines may be in the wait at once.

## Run it

```bash
docker compose run --rm lab go run ./labs/threads -wait chan
docker compose run --rm lab go run ./labs/threads -wait sleep
docker compose run --rm lab go run ./labs/threads -wait syscall -n 1000
docker compose run --rm lab go run ./labs/threads -wait syscall -n 5000
docker compose run --rm lab go run ./labs/threads -wait syscall -limit 500 -hold 200ms
docker compose run --rm lab go run ./labs/threads -wait syscall   # INC-04
docker compose exec dev go test -race ./labs/threads
```

## What it printed

On 2026-11-25, `lab` service (2 CPUs), Go 1.27.4:

```
$ go run ./labs/threads -wait chan
wait=chan limit=0 goroutines=10001 threads=4 GOMAXPROCS=2
all 10000 done in 2.004s
$ go run ./labs/threads -wait sleep
wait=sleep limit=0 goroutines=10001 threads=4 GOMAXPROCS=2
all 10000 done in 2.014s
$ go run ./labs/threads -wait syscall -n 1000
wait=syscall limit=0 goroutines=1001 threads=1003 GOMAXPROCS=2
all 1000 done in 2.106s
$ go run ./labs/threads -wait syscall -n 5000
wait=syscall limit=0 goroutines=5001 threads=5003 GOMAXPROCS=2
all 5000 done in 2.501s
$ go run ./labs/threads -wait syscall -limit 500 -hold 200ms
wait=syscall limit=500 goroutines=10001 threads=504 GOMAXPROCS=2
all 10000 done in 4.053s
```

Waiting on a channel or a timer, 10,000 goroutines ran on 4 threads. Each goroutine in a
blocking call took one thread: 1,003 threads for 1,000 goroutines, 5,003 for 5,000. The
`-limit 500` run held 504 threads; it took longer only because the 10,000 waits ran 500 at
a time (`-hold 200ms` keeps it to 4 s; the thread count does not depend on it).

The incident, `-wait syscall` with the defaults (10,000 goroutines, 2 s), ends with a stack
trace of about 160,000 lines. The first two:

```
runtime: program exceeds 10000-thread limit
fatal error: thread exhaustion
```

With `-hold 500ms`, the same run did not crash: the first calls returned before the
runtime reached 10,000 threads (6,969 when sampled). The limit is on threads alive at the
same time, so the crash depends on how long the calls block.

## The rule

The `runtime` package documentation: "There is no limit to the number of threads that can
be blocked in system calls on behalf of Go code; those do not count against the GOMAXPROCS
limit." And `runtime/debug.SetMaxThreads`: "The initial setting is 10,000 threads."

The fix is to bound the blocking calls, not the goroutines: a buffered channel used as a
semaphore, as `-limit` does.
