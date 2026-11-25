# INC-05: the counter that lies

- **Date:** 2026-11-25
- **Module:** 05, The memory model in practice
- **Reproduction:** `labs/racy-counter`, and `internal/app/ingestion` at tag `aula-05-07`

## Summary

Eight goroutines added 1 to a shared `int64` with `c++`, the way a first progress counter
for the ingestion would be written. Nothing crashed and nothing printed an error: the total
came out short, by a different amount on every run. With one CPU, or with little work, it
was right, so a small test passed. The race detector named the line on the first run.

The fix with `sync/atomic` made every count exact, and a second bug appeared: four exact
counters read one after the other are not a moment of the run.

## Impact

Measured with `labs/racy-counter`, `lab` service (2 CPUs), 8 goroutines adding 100,000 times
each, want 800,000:

| Run | Total |
|---|---|
| `go run`, first run | 800,000 |
| `go run`, next two runs | 454,320 and 431,025 |
| 20 runs of the built binary | 209,463 to 497,410, none right |
| `GOMAXPROCS=1` | 800,000 |
| 2 goroutines, 1,000 adds each | 2,000 of 2,000 |
| `go run -race` | `Found 2 data race(s)`, exit status 66 |
| `-counter atomic` | 800,000 |

## Cause

`c++` is a read, an add and a write. Two goroutines that read the same value both write
value + 1, and one add is lost. The Go memory model (<https://go.dev/ref/mem>) calls it a data
race: "a write to a memory location happening concurrently with another read or write to
that same location, unless all the accesses involved are atomic data accesses as provided
by the sync/atomic package". Nothing in the program ordered one goroutine's add before
another's: no channel, no lock, no atomic.

The guarantee the memory model gives is for programs without races: "In the absence of data
races, Go programs behave as if all the goroutines were multiplexed onto a single processor."
That is why `GOMAXPROCS=1` gave the right number, and why it proves nothing.

## Fix

1. **Each count:** `atomic.Int64`. "If the effect of an atomic operation A is observed by
   atomic operation B, then A is synchronized before B": no add is lost.
2. **The snapshot:** the ingestion's `Progress` (lesson 05-07) had four `atomic.Int64` and a
   `Snapshot` that loaded them one by one. A reader between two loads saw more chunks done
   than cut. Loading in another order fixed that check and broke another (24 pending chunks
   with 4 workers, a moment that never existed). Since lesson 05-08, one `sync.Mutex` guards
   the four counts, and a snapshot is a copy taken under the lock. It costs 21.24 ns per chunk
   alone and 24.86 ns with two goroutines, against 7.70 and 19.07 ns with atomics (`bench`
   service, `benchstat`, 10 runs).

## Regression tests

`TestAtomicCountIsExact` (`labs/racy-counter/main_test.go`) runs 8 goroutines under `-race`
in the CI. With the atomic counter changed back to `c++`:

```
--- FAIL: TestAtomicCountIsExact (0.00s)
    main_test.go:14: count(atomic, 8, 10000) = 40272, want 80000
    testing.go:1867: race detected during execution of test
```

`TestSnapshotIsAMomentOfTheRun` (`internal/app/ingestion/progress_test.go`) takes 100,000
snapshots while 4 workers cut and embed, and wants 0 to 4 pending chunks. With four atomics:

```
--- FAIL: TestSnapshotIsAMomentOfTheRun (0.00s)
    progress_test.go:54: snapshot 20564: documents=0 chunks=8 embedded=9 failed=0 pending=-1, want pending from 0 to 4
```

The race detector says nothing about the second bug: every access was atomic, so there was
no data race.

## Where Garimpo meets it

Every count shared by the ingestion's workers goes through `ingestion.Progress`. The CI runs
every test with `-race`, which costs about 7 times the time of the suite (1.6 s against
11.2 s, lesson 05-04) and finds races only in the code the tests run.
