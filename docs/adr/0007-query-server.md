# ADR 0007: The query server

- **Status:** accepted
- **Date:** 2026-11-27
- **Module:** 16, Query server

## Context

Modules 12 to 15 built the indexes and the hybrid search, but no command served them, and
the vectors of an ingestion were dropped: the file of `garimpo ingest -vectors` kept the
vectors without their chunks. RF03 asks for searches over HTTP, and RN01 for a citation
with every result. A search must end when its request has waited enough. Many clients ask
the same question at once (INC-16).

## Decision

1. **The use case `search.Searcher`** in the application: a `retrieval.Query` (a value
   object of the domain: valid UTF-8, no space at the ends, at most 1,000 bytes), `k` from 1
   to `MaxK` (50), a ranking from the port `Ranker` and, for each hit, its citation and its
   text from the port `Texts`. A hit the corpus does not have fails the search: no result
   without a citation.
2. **The vectors are kept with their chunks.** Version 2 of `vectorfile` writes the
   `ChunkID` with each vector, and syncs each record (`Add` with `fsync`); the ingestion keeps
   a vector before it confirms its chunk, through the port `ingestion.VectorStore`. Version 1
   still reads.
3. **`garimpo serve` builds the indexes when it starts.** It loads the vectors into a
   `flatindex.Published`, and cuts the corpus again for the BM25 index and the texts. It
   refuses vectors of chunks the corpus no longer has.
4. **The adapter `httpapi`** serves `GET /search?q=&k=`. Every search has `-timeout` (2 s)
   from when the request arrives: past it, 504. A bad query or `k` is 400; any other failure
   500, with the cause in the log and not in the body. A client that leaves cancels its
   search. The `http.Server` bounds the reading of a request (`ReadHeaderTimeout` 5 s,
   `ReadTimeout` 10 s), its writing (`-timeout` plus 5 s) and idle connections (1 min), and
   stops gracefully on SIGINT or SIGTERM, for up to 10 s.
5. **The JSON is `encoding/json/v2`'s.** It is on by default in Go 1.27. The body is encoded
   before the status: a body that cannot be JSON is a 500.
6. **Equal searches at once run once** (`search.Collapse`, `singleflight`), with a context of
   their own: `context.WithoutCancel` of the first caller's, and the deadline of a request.

## Measurements behind it

All taken on 2026-11-27 with Go 1.27.4, i7-13700H in WSL 2, the corpus of the Go library.

| Decision | Evidence |
|---|---|
| Vectors with their chunks | The ingestion with `-vectors`, 39,244 vectors: 23m37s and 61,534,604 bytes (12 + 39,244 × 1,568). Module 13's, without the IDs: 22m48s |
| The indexes built at start | `garimpo serve` loads 41,980 chunks, 39,244 with vectors, in 669 to 738 ms |
| A deadline per request | With `-timeout 1ms`, 504 in 2.5 ms |
| `encoding/json/v2` | `labs/jsonv2`: the response of 10 results in 6.547 µs with v1 and 6.420 µs with v2, no difference (p = 0.247), 2 allocations each. v1 writes `<`, `>` and `&` in 6 bytes each, v2 in 1; v1 turned a text of invalid UTF-8 into a 200 with U+FFFD, v2 refuses it |
| `singleflight` with a context of its own | 100 equal requests at once on the server: 1.498 and 1.547 s and 100 calls to the model without it, 13 and 14 ms and 1 call with it; 100 different ones, the same with or without. In `labs/singleflight`, `Group.Do` with the first caller's context failed all 100 questions when that one was canceled |

## Alternatives considered

- **A persisted index** that `serve` opens. No module of the plan has one; the exact index
  over the file of vectors loads in under a second. Not now.
- **Ingest when the server starts.** 24 minutes of the model at every start. Rejected.
- **`http.TimeoutHandler`** for the deadline. It answers 503 and writes its own body; the
  deadline in the context reaches the search, which stops (INC-15), and the handler answers
  504 with the JSON of the API. Rejected.
- **Encode straight to the `ResponseWriter`.** A failed encoding would come after the 200.
  Rejected.
- **A cache of results.** It would answer from an old snapshot after an `Add`, and needs a
  rule to forget. `singleflight` shares only what runs at the same moment. Not now.
- **`Group.Do` with the caller's context.** One client that leaves would cancel the search
  of all. Rejected.

## Consequences

- The server holds the whole exact index in memory (about 60 MB of vectors) and the places
  of the chunks, not their texts.
- A server started on a corpus that changed refuses to start: the vectors must be ingested
  again.
- `GOEXPERIMENT=nojsonv2` does not build Garimpo.
- A shared search runs until its deadline even when every client left: at most `-timeout`
  of work for nobody.
- Through HTTP, 100 different searches at once take about 1.4 to 1.7 s, bound by the model;
  where that time goes beyond the model was not measured.
