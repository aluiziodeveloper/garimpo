# INC-15: the vector that does not find ctx.Done

- **Date:** 2026-11-27
- **Module:** 15, Hybrid search
- **Reproduction:** a probe at the `main` of the end of module 14 (`aula-14-11` and PR #112),
  `dev-env` capture `15-01-probe_test.go`, kept out of the repository: the real vectors of the
  corpus (`garimpo ingest -vectors`, 39,244 vectors of 384 dimensions), 38,744 in each exact
  index and the last 500 as queries; 4 goroutines, 100 searches each, every search with a
  `context.WithTimeout` of 1 or 2 ms, shorter than a scan; 8 virtual CPUs

## Summary

A scan of the corpus takes about 6 ms. A caller that gives a search 1 ms, as a server with
a deadline per request will, expects it to fail at about 1 ms. Every one of the 1,600
searches of the probe ran to the end of its scan, and returned its hits with no error,
milliseconds after its deadline: 25 ms after it, at worst.

No test failed. The port said what the indexes did: ctx's error "when ctx is done before
the search starts". The contract checked a context canceled before the call, never one that
ends during it.

## Impact

The probe, 2026-11-27 (`dev-env` capture `15-01-probe.txt`), the time from the deadline to
the return of the search:

| Index | Deadline | p50 | p99 | max | Errors |
|---|---|---|---|---|---|
| `Index` | 1 ms | 4.882 ms | 8.443 ms | 8.85 ms | 400 `nil` |
| `Index` | 2 ms | 3.8 ms | 8.284 ms | 25.451 ms | 400 `nil` |
| `Published` | 1 ms | 4.821 ms | 10.395 ms | 12.296 ms | 400 `nil` |
| `Published` | 2 ms | 3.803 ms | 7.879 ms | 8.287 ms | 400 `nil` |

Two harms. The caller got its answer late and could not tell: with `err == nil`, a late
answer looks like an answer on time. And the CPU kept scanning for a caller that had gone:
with the hybrid search, a keyword search that fails keeps the vector scan running to its
end.

## Cause

1. **Cancellation in Go is cooperative.** A context does not stop a goroutine: it closes a
   channel, `Done`, and makes `Err` return an error. Only code that looks at them stops.
2. **The scan never looked.** `Search` checked `ctx.Err()` once, before the scan, and then
   computed 38,744 dot products under a lock or over a snapshot: no channel, no I/O, nothing
   that would have run into the context.
3. **The port and the contract allowed it.** "ctx's error when ctx is done before the search
   starts" is what a search that checks once does; the suite tested a canceled context, not
   one that ends during the call.

## Fix

1. **Check the context during the scan** (lesson 15-09, `feat(app): search by vector and
   keyword`). `flatindex.Index` and `flatindex.Published` check `ctx.Err()` every
   `search.ScanCheck` (1,024) vectors and return ctx's error, with no hits. `Sharded` gets it
   from its shards. The HNSW index keeps 64 candidates on level 0 (`EfSearch`), a small part
   of the index, and checks only before it starts, which the port allows an approximate
   index. Same reproduction (`dev-env`
   capture `15-09-probe.txt`):

   | Index | Deadline | p50 | p99 | max | Errors |
   |---|---|---|---|---|---|
   | `Index` | 1 ms | 0.246 ms | 1.366 ms | 1.467 ms | 400 `DeadlineExceeded` |
   | `Index` | 2 ms | 0.330 ms | 1.146 ms | 1.256 ms | 400 `DeadlineExceeded` |
   | `Published` | 1 ms | 0.188 ms | 1.238 ms | 1.338 ms | 400 `DeadlineExceeded` |
   | `Published` | 2 ms | 0.367 ms | 1.112 ms | 1.155 ms | 400 `DeadlineExceeded` |

   The checks cost nothing measurable: `BenchmarkSearch` and `BenchmarkSearchPublished`, 10
   runs each before and after, differ within the noise (benchstat: p = 0.353 and 1.000).
2. **Say it in the port.** `search.VectorIndex` now says that an exact index checks ctx at
   least once every `ScanCheck` vectors and fails with its error when it is done.
3. **Cancel the other search.** `search.Hybrid` runs its two searches in an `errgroup`: the
   first error cancels the group's context, which the scan of the other now sees.

## Regression tests

- `SearchChecksTheContextAsItScans`, in `indextest.Contract`, counts the calls to `Err` of a
  search of 8,192 vectors and wants at least 4. Before the fix, every exact index called it
  once.
- `SearchStopsWhenTheContextIsDoneDuringTheScan` gives the search a context that is done
  from its second call to `Err` on. Before the fix, every exact index returned 5 hits and
  `nil`.
- Both use a context that counts its calls instead of a deadline: on a clock, the test would
  depend on how fast the machine scans.
- `TestAFailedSearchCancelsTheOther` makes the keyword search fail while the vector search
  waits on its context, and wants that wait to end.

## Where Garimpo meets it

- **A long loop of CPU must look at its context.** Nothing else will stop it: not the
  deadline, not the caller that left.
- **Check often enough, and no more.** 1,024 vectors of 384 dimensions are about 0.15 ms of
  scan (`BenchmarkSearch`: 6.2 ms for 41,980); the p99 after the fix, 1.1 to 1.4 ms past the
  deadline, is above that, and its breakdown was not measured.
- **Test cancellation without a clock.** A context whose `Err` changes after N calls tells
  whether the code looks, on any machine.
- **A late answer with no error is a wrong answer.** The caller of the hybrid search, and the
  server of module 16, can now tell a search that ran out of time.
