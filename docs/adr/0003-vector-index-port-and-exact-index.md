# ADR 0003: The VectorIndex port and the exact index

- **Status:** accepted
- **Date:** 2026-11-26
- **Module:** 12, Exact search and the index contract

## Context

Until module 11, the vectors of an ingestion were checked and dropped: Garimpo had nowhere
to keep them and no way to search them. Module 13 brings an approximate index (HNSW), which
is fast and may miss; to know how much it misses, Garimpo needs an index that never does and
one set of rules that both are held to. INC-11 measured the core of a search, one dot
product per vector: 7.2 to 8.0 ms per query over the 41,980 chunks of the corpus.

## Decision

1. **The port `search.VectorIndex`, in the use-case layer** (`internal/app/search`), where
   the searches consume it: `Add(id, v) error`, `Search(ctx, q, k) ([]retrieval.Hit, error)`
   and `Len() int`, safe for concurrent calls. The errors wrap four sentinels:
   `ErrZeroChunk`, `ErrWrongDimension`, `ErrDuplicateChunk` and `ErrInvalidK`. The context
   is checked before the scan starts, not during it.
2. **A contract suite**, `indextest.Contract`, outside the port's package as with the
   `Embedder` and the `Journal`: ten cases, from "a new index finds nothing" to concurrent
   `Add` and `Search` under `-race`. It computes the k best itself, scanning every vector,
   and asks for the same chunks in the same order. It was checked against a naive map
   index, broken three ways (ties by score only, duplicates kept, no lock), and each one
   failed its case.
3. **A fixed ranking order, in the domain.** `retrieval.Hit` is a chunk and its score, a
   plain value. `retrieval.Compare` puts the higher score first and, between equal scores,
   the lower `ChunkID` (`ChunkID.Compare`, a change to the shared kernel): two indexes, or
   two runs, rank equal scores the same way, and the suite can compare rankings exactly.
4. **The exact index, `flatindex.Index`.** The vectors sit one after the other in one
   `[]float32`, so a search reads memory in order. A `sync.RWMutex` guards it: searches
   share it, an `Add` takes it alone. A search offers every hit to a `topk.Keeper`, a heap
   of k, instead of sorting them all.
5. **`internal/topk`, a support package.** `Keeper[T]` keeps the k best of a stream in a
   heap with the worst at the root. It fills its slice and calls `heap.Init` and
   `heap.Fix`, never `heap.Push` or `heap.Pop`, which take and return an `any`.
6. **`flatindex.Sharded`, scatter-gather.** n `flatindex.Index` shards; a chunk goes to the
   shard a hash of its ID picks (`hash/maphash.Comparable`), so a duplicate meets the shard
   that has it. A search runs every shard in a goroutine and merges their k best with a
   `Keeper`: the k best of all are among the k best of some shard, so it stays exact.

## Measurements behind it

All taken on 2026-11-26 with Go 1.27.4, i7-13700H in WSL 2, 41,980 random vectors of 384
dimensions, the 10 best of a query, `benchstat` with 10 runs.

| Decision | Evidence |
|---|---|
| Heap, not sort | `labs/topk`, CPU 2: the 10 best of 41,980 scores take 3.616 ms sorting them all and 26.01 µs with a heap; about 17 comparisons per score against little more than one |
| Heap in the index | `flatindex.Search`, CPU 2, interleaved: 13.632 ms sorting, 7.368 ms with the `Keeper` (−45.95%); 1,481.875 KiB per search to 2.297 KiB |
| No `heap.Push` | In `labs/topk`, `container/heap` allocated 2·k + 3 times per call, a `float32` in each `any`; in `topk`, `TestOfferDoesNotAllocate` fails with `Offer allocated 1 times per call, want 0` when `Offer` uses `heap.Push` and `heap.Pop` |
| Hash, not round-robin | Sending each chunk to the next shard let a duplicate into another one: the suite failed `AddRefusesWhatTheIndexCannotKeep` with 8 shards (`Len() = 2`). With 3 it passed by chance: the refused calls before it moved the counter back to the same shard |
| Shards for latency | CPUs 2,4,6,8: one search at a time, 7.137 ms with 1 shard, 2.383 ms with 4 (−66.60%), 2.410 ms with 8 and 2.434 ms with 16. On 8 CPUs: 7.415, 2.325, 2.028 and 1.805 ms with 1, 4, 8 and 16 shards |
| Not for throughput | CPUs 2,4,6,8, four searches at once: 2.156 ms per search with 1 shard and 2.110 to 2.182 ms with 2 to 16, no difference beyond the noise (p from 0.052 to 0.796) |

## Alternatives considered

- **Sort every hit.** Simple, and 46% of the time of a search. Rejected for the heap.
- **A sorted slice of k.** As fast as the heap for k=10 (32.79 µs against 26.01), and 3.7
  times slower for k=1000 (1.660 ms against 445.0 µs). Rejected: the heap holds for any k.
- **The port in the domain.** The domain is sequential and does no I/O; an index is
  concurrent and, from module 14, keeps snapshots. The port is the use cases' need.
  Rejected.
- **A generic port** (`VectorIndex[K]`). Garimpo has one key, `ChunkID`, and a type
  parameter on the port would follow every use case for nothing. Rejected; the generics
  stay where they pay, in `topk.Keeper[T]`.
- **Round-robin sharding.** Even, and blind to duplicates. Rejected for the hash.

## Consequences

- Every index of Garimpo passes `indextest.Contract`; the HNSW of module 13 will pass it
  with a tolerance of recall, measured against `flatindex`.
- An `Add` waits for the searches under way, and the searches wait for an `Add`: an index
  that takes vectors while it answers will make searches wait. Module 14 measures it.
- Shards lower the latency of a search only while there are idle CPUs. On a busy server,
  every CPU already has a query, and a sharded index does the same work with more
  goroutines (12 allocations per search with 1 shard, 54 with 8).
- No command uses the index yet: `garimpo ingest` still drops the vectors.
