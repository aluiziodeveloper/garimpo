# labs/contention

Ten thousand small lists, shared by goroutines that read them and append to them, the way
the nodes of the HNSW graph share their links (module 13), under four kinds of lock. Every
kind keeps every append (`TestEveryStoreKeepsEveryAppend`, with `-race`). Measured in the
`bench` service: 8 virtual CPUs of an i7-13700H, Go 1.27.4, 2026-11-26.

| Kind | How it locks |
|---|---|
| `mutex` | one `sync.Mutex` for every list |
| `rwmutex` | one `sync.RWMutex`: readers share it, a writer takes it alone |
| `striped` | 64 `sync.Mutex`, list `i` under lock `i % 64` |
| `perlist` | one `sync.Mutex` in each list |

```bash
BENCH_CPUSET=2,4,6,8,10,12,14,16 docker compose run --rm bench sh -c 'go test -run "^$" -bench Store -cpu 1,2,4,8 -count 6 ./labs/contention > /tmp/b.txt &&
    go tool -modfile=tools/go.mod benchstat -col /gomaxprocs /tmp/b.txt'
```

`-cpu 1,2,4,8` runs each benchmark with 1, 2, 4 and 8 goroutines (`RunParallel` starts
GOMAXPROCS of them); `reads` is the percentage of operations that read. The time is per
operation, counting those of every goroutine, so a kind that scales goes down:

| reads | Kind | 1 | 2 | 4 | 8 |
|---|---|---|---|---|---|
| 50 | `mutex` | 16.99 ns | 24.98 ns | 44.61 ns | 67.18 ns |
| 50 | `rwmutex` | 21.76 ns | 32.78 ns | 34.18 ns | 34.46 ns |
| 50 | `striped` | 17.81 ns | 23.23 ns | 23.73 ns | 23.79 ns |
| 50 | `perlist` | 17.75 ns | 11.57 ns | 7.14 ns | 4.86 ns |
| 90 | `mutex` | 16.85 ns | 21.91 ns | 38.55 ns | 58.51 ns |
| 90 | `rwmutex` | 17.48 ns | 23.04 ns | 24.84 ns | 25.23 ns |
| 90 | `striped` | 16.41 ns | 20.64 ns | 19.95 ns | 21.17 ns |
| 90 | `perlist` | 16.53 ns | 10.48 ns | 6.53 ns | 4.65 ns |
| 99 | `mutex` | 17.92 ns | 19.17 ns | 35.30 ns | 53.08 ns |
| 99 | `rwmutex` | 17.16 ns | 22.28 ns | 29.78 ns | 38.69 ns |
| 99 | `striped` | 16.78 ns | 19.21 ns | 18.60 ns | 21.46 ns |
| 99 | `perlist` | 17.00 ns | 10.59 ns | 6.19 ns | 4.37 ns |

Every difference from 1 goroutine has p=0.002 (n=6); the full benchstat output is in the
capture of the lesson.

- **Alone, every lock costs the same**, 16 to 22 ns an operation: an uncontended lock is a
  pair of atomic instructions.
- **One `Mutex` gets slower with every goroutine**: 8 goroutines did, together, a third of
  the work of one.
- **More reads do not save the `RWMutex`.** With 99% reads it got slower from 4 to 8
  goroutines (29.78 to 38.69 ns): a reader that never waits still adds to a counter inside
  the lock, and every reader adds to the same one.
- **Only the lock per list scales**: 3.5 to 3.9 times the work of one goroutine with 8,
  at any share of reads. Two goroutines rarely want the same one of 10,000 lists.

`go run` gives the same comparison in time for 200,000 operations, and how long the
goroutines waited for locks, from `/sync/mutex/wait/total:seconds` of `runtime/metrics`:

```bash
BENCH_CPUSET=2,4,6,8,10,12,14,16 docker compose run --rm bench go run ./labs/contention
```

```
10000 lists, 200000 operations, 90% reads, GOMAXPROCS=8
kind       goroutines  time         waiting for locks
mutex      1           4.713ms      0s
mutex      2           6.293ms      60µs
mutex      4           9.678ms      897µs
mutex      8           11.883ms     14.18ms
rwmutex    1           4.971ms      0s
rwmutex    2           5.461ms      456µs
rwmutex    4           6.166ms      10.735ms
rwmutex    8           5.299ms      37.804ms
striped    1           3.864ms      0s
striped    2           4.349ms      0s
striped    4           4.114ms      583µs
striped    8           4.141ms      42µs
perlist    1           4.169ms      0s
perlist    2           3.342ms      0s
perlist    4           3.52ms       0s
perlist    8           1.788ms      0s
```

The wait counts only goroutines that blocked on a lock. `striped` waited almost nothing
and still did not scale: what slowed it was not waiting.
