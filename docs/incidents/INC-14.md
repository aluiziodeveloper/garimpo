# INC-14: the ingestion that waited for the searches

- **Date:** 2026-11-27
- **Module:** 14, Lock-free reads and CQRS
- **Reproduction:** `TestSearchDuringIngestOnTheCorpus` at tag `aula-14-06`, the rows of
  `Index`, on the real vectors of the corpus (`garimpo ingest -vectors`, 39,244 vectors of
  384 dimensions): 38,744 in the index, the last 500 as queries and as the vectors added, 4
  goroutines searching for 5 s, 8 virtual CPUs

## Summary

Garimpo will search the index while the ingestion adds to it. The exact index,
`flatindex.Index`, guards its vectors with one `sync.RWMutex`: a search holds it shared for
the whole scan of 38,744 vectors, and an `Add` takes it alone. A probe of the two at once,
before the course planned a fix, expected the searches to suffer. They hardly did; the
`Add` calls did. Alone, an `Add` took a microsecond; with 4 goroutines searching, 5 ms, the
time of a scan.

No test failed and no data was lost: the index was correct and slow on the side nobody
watched. The plan had named the incident after the symptom it expected, "the search that
froze during the ingestion"; the measure renamed it.

## Impact

The probe, 2026-11-27 (`dev-env` capture `14-01-probe.txt`):

| Searchers | Adds | Search p50 | Search p99 | Adds in 5 s | Add p50 | Add p99 |
|---|---|---|---|---|---|---|
| 4 | none | 5.507 ms | 9.374 ms | | | |
| 4 | one every 35 ms | 5.864 ms | 10.475 ms | 126 | 5.147 ms | 7.637 ms |
| 4 | as fast as possible | 6.012 ms | 7.846 ms | 855 | 5.985 ms | 7.813 ms |
| 0 | as fast as possible | | | 1,134,422 | 1 µs | 3 µs |

At the pace of an ingestion (39,244 vectors in 22m48s, one every 35 ms), each `Add` waited
5 ms: at that p50, about 3.4 minutes of a run (39,244 × 5.147 ms) spent waiting for scans. As fast as it could, the
ingestion went from over a million `Add` calls in 5 s to 855.

## Cause

1. **A scan holds the read lock for its whole length.** `Search` takes `RLock` once and
   reads every vector under it: 5 to 6 ms on the corpus.
2. **An `Add` waits for every scan under way.** `Lock` waits until no reader holds the lock.
   With 4 goroutines searching back to back, there is almost always one.
3. **A waiting writer stops the new readers.** `sync.RWMutex` gives a waiting `Lock` the
   turn: the searches that start after it wait for it, which is why they lost little, and
   why the `Add` never waited for more than the scans already under way.
4. **The lock guards two things that need not wait for each other.** A search needs a
   complete version of the index (RN05), not the newest one; an `Add` needs to make the next
   version, not to stop the reads of the current one.

## Fix

1. **An immutable snapshot in the domain** (lesson 14-05, `feat(domain): add the
   IndexSnapshot`). `indexing.IndexSnapshot` is one version of the index; `With` returns the
   next and leaves it as it was. It writes past the end of the shared arrays while no other
   version did, so a growing index copies nothing until the arrays fill, and copies when a
   second version is made from the same snapshot.
2. **Publish it with `atomic.Pointer`** (lesson 14-06, `feat(index): publish snapshots
   atomically`). `flatindex.Published` searches the snapshot it loads, with no lock; the
   `Add` calls take a `sync.Mutex` of their own, make the next version and store it. Same
   reproduction: an `Add` at the pace of an ingestion took 24 µs at p50 and 47 µs at p99,
   against 3.388 and 7.749 ms for `Index`, and the searches did not change beyond the noise
   (6.842 against 6.442 ms at p50).
3. **The staleness measured** (lesson 14-07). From the start of an `Add` to the moment
   another goroutine sees the vector: 25 µs at p50 for `Published`, 4.783 ms for `Index`,
   both the duration of the `Add` itself.

## Regression tests

- `TestTwoVersionsOfOneSnapshotKeepWhatEachAdded` makes two versions from one snapshot and
  asks each to keep its own chunk. With `With` writing in place whenever the array has room,
  the classic copy-on-write slip, it failed with `chunk 1 of the snapshot is ..., want
  chunk 2`.
- `TestPublishedMeetsTheContract` runs the contract of the port, with `-race`, on
  `Published`, and `TestASnapshotStaysAsItWasAfterAnAdd` asks a snapshot taken before an
  `Add` to keep its length and version.
- `TestASearchSeesEveryAddThatReturned` asks a search to find the last vector whose `Add`
  returned, on both indexes. Publishing the snapshot from a goroutine, so that `Add` would
  not wait, failed it with `after 5 Adds returned, a search for vector 4 found []`.
- `TestSearchDuringIngestOnTheCorpus` and `TestStalenessOnTheCorpus` need the vectors of the
  corpus (`GARIMPO_VECTORS`) and skip in the CI; they are run by hand, with the commands in
  the README.

## Where Garimpo meets it

- **Measure both sides of a lock.** The probe was written to find slow searches and found
  slow writes. A latency of the readers alone would have called `RWMutex` fine.
- **Name an incident after what was measured.** The plan's title described a symptom no
  measure showed.
- **Eventual consistency, bounded.** A search may miss an `Add` still under way, never one
  that returned; the window is the `Add` itself, measured in microseconds.
- **Only the exact index reads snapshots.** The HNSW index keeps its lock per node (ADR
  0004) and its searches take the index `RLock` briefly; RN05 holds for `Published`.
