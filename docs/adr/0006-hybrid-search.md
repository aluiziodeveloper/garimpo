# ADR 0006: Hybrid search, BM25 and Reciprocal Rank Fusion

- **Status:** accepted
- **Date:** 2026-11-27
- **Module:** 15, Hybrid search

## Context

The vector index finds chunks that mean what the query means. A name the model never
learned, such as `WriteString`, `EOF` or `ErrTextTooLong`, is better found by its terms.
RF03 asks for searches by vector, by keyword and both at once. The two searches score on
scales that cannot be added: a cosine between -1 and 1, and a BM25 score from 0 up. And a
search must stop when its caller gives up, which the exact indexes did not do (INC-15).

## Decision

1. **A port for the keyword index**, `search.KeywordIndex` (`Add(id, text)`, `Search(ctx,
   query, k)`, `Len`), beside `VectorIndex`, with the same errors.
2. **`bm25.Index`, an inverted index ranked by BM25.** The terms of a text are its runs of
   letters and digits, in lower case, of two characters or more. The IDF is Lucene's,
   `ln(1 + (N - n + 0.5) / (n + 0.5))`, which is never negative; `K1` is 1.2 and `B` 0.75.
   One `sync.RWMutex` guards the index; hits are ranked by `retrieval.Compare`.
3. **The terms are copies, stored once.** The key of a new term is `strings.Clone` of it,
   and its postings live behind a pointer, so that adding a posting never stores the key
   again. Not `unique.Handle`.
4. **The scores of a search in a pooled slice.** A search sums them in a `[]float64` with a
   place for every chunk, and lists the chunks it touched; a `sync.Pool` keeps both for the
   next search, with every score put back at 0.
5. **`retrieval.Fuse` in the domain.** Reciprocal Rank Fusion, k = 60: a chunk scores the sum
   of 1/(60 + its rank) over the rankings that hold it. It reads only the ranks.
6. **`search.Hybrid` in the application.** It asks both indexes for `Depth` hits (50 by
   default, at least k): the keyword search at once, the vector search once the query has
   its vector. They run in an `errgroup`; the first error cancels the other. The embedder
   port is declared again in `search` (`search.Embedder`), with the method of
   `ingestion.Embedder`, so the retrieval context does not import the ingestion one (ADR
   0002).
7. **The exact indexes check ctx every `search.ScanCheck` (1,024) vectors**, and the
   contract suite checks it with a context that counts its calls to `Err`.

## Measurements behind it

All taken on 2026-11-27 with Go 1.27.4, i7-13700H in WSL 2, 8 virtual CPUs; the 41,980
chunks of the Go library, or the 39,244 vectors of the corpus.

| Decision | Evidence |
|---|---|
| Copies of the terms, stored once | Heap of the index after a GC, 3 interleaved runs: 53.2 MiB with substrings of the texts, 52.0 to 52.1 with copies stored again at every `Add`, 45.7 with `unique.Handle`, 29.5 with copies stored once |
| Not `unique.Handle` | Build of the index: 725 to 751 ms with `unique.Make`, 417 to 427 with the copies |
| A pooled slice for the scores | 1,000 searches of 3 terms, 3 interleaved runs: a map per search, p50 454 to 493 µs and 557.5 to 557.9 KiB allocated per search; a slice per search, 186 to 197 µs and 496.6 KiB; a slice from the pool, 124 to 133 µs and 1.8 KiB |
| `sync.Pool` for buffers | `labs/pool`, 20,000 reports by 8 goroutines: 1,250.1 MiB allocated and 396 GC cycles without the pool, 0.5 MiB and none with it |
| Checking ctx during the scan | Searches with a deadline of 1 or 2 ms: returned 3.8 to 4.9 ms past it at p50 with `nil`, before; 0.19 to 0.37 ms with `DeadlineExceeded`, after. Benchmarks of the scan: no difference beyond the noise (p = 0.353 and 1.000) |

## Alternatives considered

- **Fuse the scores, normalized.** A min-max of each ranking depends on its worst hit, which
  changes with `Depth`, and a weight between the two would need data the course has not
  measured yet. RRF needs only the ranks. Rejected for now.
- **Intern the terms with `unique.Make`.** It frees the texts too, at 16 MiB more heap and
  300 ms more of build. Rejected.
- **A map of scores per search, or a slice per search.** 558 and 497 KiB allocated per
  search. Rejected.
- **The two searches one after the other.** The keyword search does not need the vector:
  it would wait for the embedding for nothing. Rejected.
- **Import `ingestion.Embedder`.** The search would depend on the ingestion context for one
  method. Rejected.
- **Check ctx at every vector.** One check per vector to stop at most 0.15 ms sooner (1,024
  vectors of `BenchmarkSearch`); 1,024 already costs nothing measurable. Rejected.

## Consequences

- The keyword search checks ctx only before it starts: its p99 on the corpus is about 0.5
  ms. The HNSW search too: it keeps 64 candidates on level 0, not the whole index.
- The pool keeps, for each search under way at once, a slice of 8 bytes per chunk plus a
  quarter: 410 KiB on the corpus. The GC empties it in two cycles when the searches stop.
- `Depth` is 50 with no measure of quality behind it: how much the fusion finds that each
  search alone misses is measured in module 22, with recall and MRR.
- `Hybrid` is not yet used by a command: the use case `Search` of module 16 serves it.
