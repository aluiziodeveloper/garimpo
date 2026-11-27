# ADR 0004: The concurrent HNSW index

- **Status:** accepted
- **Date:** 2026-11-27
- **Module:** 13, Concurrent HNSW

## Context

The exact index (ADR 0003) reads every vector for every query: 5.9 ms per search over the
38,744 vectors of the corpus. Garimpo will search while it ingests, and the ingestion adds
vectors from many workers. It needs an index that reads few vectors per query, takes `Add`
calls at the same time, and misses little: a recall@10 of at least 0.95 (RNF04).

## Decision

1. **HNSW** (Malkov and Yashunin, 2018), in `internal/adapters/hnsw`: a graph in levels,
   a node on level l with probability M^-l, at most M links per node on each level and 2·M
   on level 0, links picked by the heuristic that keeps them in different directions, a
   greedy descent from the entry point and a search of `EfSearch` candidates on level 0.
   Defaults: `M` 16, `EfConstruction` 200, `EfSearch` 64.
2. **The contract with a tolerance.** `indextest.ApproximateContract(t, minRecall, ...)`
   runs the cases of `Contract`, asks every hit to be a known chunk, once, with its score,
   in order, and asks for at least minRecall of the best over all the searches of a case.
   HNSW passes it with 0.95, with `EfSearch` 64 and 1.
3. **Recall on the corpus, by hand.** `garimpo ingest -vectors` writes the vectors of an
   ingestion to a file (`internal/adapters/vectorfile`); `TestRecallOnTheCorpus` measures
   the graph on it against `flatindex`, and fails below 0.95. The CI runs only the test on
   random vectors.
4. **Concurrent `Add`, with a lock per node.** The index lock is taken alone only to put a
   node in the list and to raise the entry point. Each node has a `sync.Mutex` for its
   links: a walk copies a list under it, and `link` and `setLinks` change one under it.
5. **A node's own links are merged, never overwritten** (INC-13). `setLinks` adds them to
   what other inserts already linked, then prunes like `link`.
6. **A node is looked up without a lock.** `Add` publishes the list of nodes in an
   `atomic.Pointer`; the list only grows, and a walk learns of a node only through a link
   or the entry point, written after the `Add` that published it. Module 14 makes the same
   tool the way searches read the whole index (copy on write, snapshots).
7. **`topk.Keeper` changes.** `Offer` says whether it kept the value, `Len` and `Worst` let
   a search stop when the candidate left is worse than the worst it keeps, and the heap
   fills with `heap.Fix` on the last element instead of `heap.Init` at the end. ADR 0003
   says "`heap.Init` and `heap.Fix`"; it stays as written, and this record supersedes that
   detail.

## Measurements behind it

All taken on 2026-11-26 and 27 with Go 1.27.4, i7-13700H in WSL 2, the 39,244 vectors of
the corpus (`all-minilm`, 384 dimensions) or 2,000 random ones for the build benchmark.

| Decision | Evidence |
|---|---|
| HNSW | One lock, 8 goroutines: recall@10 0.982 at `EfSearch` 64, 388 µs per search against 5.922 ms for the exact index; 0.998 at `EfSearch` 256, 1.01 ms |
| A lock per node | Build of 2,000 vectors, 8 virtual CPUs, interleaved: one lock takes 3.6 s with any number of goroutines; a lock per node, 3.964 s with 1 (+8.13%), 2.045 s with 4 (−42.84%), 2.215 s with 8 (−39.01%) |
| Not a shared lock held during the walk | Holding the index `RLock` while walking and taking it alone to add a node gave no gain: 3.604 s with 8 goroutines against 3.506 s with one lock (+2.77%) |
| Merge, not overwrite | Corpus, 8 goroutines, 5 runs: overwriting, 0.904 to 0.949 at `EfSearch` 64 and at most 0.962 at 256; merging, 0.969 to 0.978 and 0.995 to 0.997. With 1 goroutine, 0.982 either way |
| No lock to look a node up | CPU profile of the build by 8 goroutines: 47% in the atomic add of `RLock`; the mutex profile does not show it. With `atomic.Pointer`: 2.254 to 0.810 s (−64.07%), 4.9 times as fast as 1 goroutine, 78% of the CPU in the dot product |
| A lock per list (lab) | `labs/contention`, 8 virtual CPUs, 8 goroutines, 90% reads: 4.65 ns per operation with a lock per list, 21.17 with 64 striped locks, 25.23 with one `RWMutex`, 58.51 with one `Mutex` |

## Alternatives considered

- **One `RWMutex` for the index.** Correct, and a build by 8 goroutines no faster than by
  one: they take turns. Rejected.
- **Hold the shared lock while walking.** An `Add` waiting to take it alone waits for every
  walk under way, and every walk that starts waits for it (`sync.RWMutex` gives a waiting
  writer the turn). Measured with no gain. Rejected.
- **Striped locks.** 64 locks for all the nodes: in the lab, 21 ns with 8 goroutines against
  4.7 for a lock per list. Padded to one per cache line, 12.67 ns: better, and still behind.
  Rejected.
- **Overwrite and accept the loss.** Rejected by RNF04: 0.904 to 0.949.
- **A fixed capacity, so the list of nodes never moves.** No atomic, and an index that
  refuses an `Add` past a number chosen in advance. Rejected.

## Consequences

- The HNSW index passes the approximate contract and, on the corpus, 0.95 at the default
  `EfSearch`. It is not exact: a search may miss a chunk the exact index finds.
- A graph built concurrently changes from run to run. Its recall is a range, and a test that
  judges it on few searches fails by chance (PR #98).
- In lesson 13-06, one goroutine built 8% slower with a lock per node than with one lock,
  with 2.1 times the allocations: a walk copies every list of links it reads. The copies
  are where the next gain is.
- Searches still take the index `RLock`, briefly, for the entry point and the chunk IDs. No
  command uses the index yet: it enters `garimpo` from module 14 on.
