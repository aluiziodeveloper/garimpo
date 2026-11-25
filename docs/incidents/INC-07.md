# INC-07: the queue that swallowed the memory

- **Date:** 2026-11-25
- **Module:** 07, The ingestion pipeline
- **Reproduction:** `RunIngestion` at tag `aula-07-06`, over the whole corpus, with an
  embedder that takes 9 ms per chunk; `BenchmarkRunIngestion` at tag `aula-07-10`

## Summary

The first ingestion pipeline put a queue between the cutter and the embedder, so that the
cutter would never wait for the model. It never did: in 200 ms the cutter had read the
whole corpus and cut 41,980 chunks, the embedder had done 84, and the queue held the rest.
A chunk is a substring of its document, so the queue kept every document alive: module 06
had taken the corpus out of memory, and the queue put it back. Nothing failed; at that pace
the run would have taken about 98 s with the whole corpus in memory, and the memory grows
with the corpus.

While measuring the fix, a second bug appeared: the first run over the whole corpus failed
at `time/tzdata/zzipdata.go` with `chunk already handled`.

## Impact

`RunIngestion` with 4 workers and an embedder that waits 9 ms per chunk, as `all-minilm`
took one text at a time (596 ms for 64), `lab` service (2 CPUs), Go 1.27.4; progress and
heap 200 ms and 2 s into the run, then canceled:

| Version | At | Chunks cut | Embedded | Pending | Heap in use | Live at the last collection |
|---|---|---|---|---|---|---|
| queue (`aula-07-06`) | 200 ms | 41,980 | 84 | 41,896 | 37.5 MiB | 28.4 MiB |
| queue (`aula-07-06`) | 2 s | 41,980 | 852 | 41,128 | 37.8 MiB | 28.4 MiB |
| at most 256 pending (`aula-07-09`) | 200 ms | 340 | 84 | 256 | 0.4 MiB | none yet |
| at most 256 pending (`aula-07-09`) | 2 s | 1,096 | 840 | 256 | 0.6 MiB | 0.4 MiB |

The embedder did about the same work in both (852 and 840 chunks in 2 s): the limit cost
no throughput.

`BenchmarkRunIngestion`, the whole corpus with an embedder that waits 20 µs per chunk on 8
workers, `bench` service (CPUs 2 and 4 of an i7-13700H), 10 runs of 2 ingestions:

| `MaxPending` | Peak pending | Peak live heap | Allocated |
|---|---|---|---|
| 64 | 64 | 4.073 MiB | 108.4 MiB |
| 256 | 256 | 4.200 MiB | 108.4 MiB |
| 4,096 | 4,096 | 7.060 MiB | 108.6 MiB |
| 65,536 (no limit: more than the corpus) | 35,913 | 27.434 MiB | 113.3 MiB |

## Cause

1. **An unbounded queue.** The queue held every chunk the embedder was not ready for, in a
   slice that grew as long as the cutter was faster, and it was much faster: in the first
   200 ms it cut 41,980 chunks, 500 times the 84 the model embedded. A queue without a bound does not remove the wait; it turns
   the wait into memory. RN03 says the ingestion never holds more pending chunks than the
   configured limit, and nothing enforced it.
2. **Two chunks with one ID.** The `IngestionRun` refuses a chunk it already handled (RN02).
   A line longer than the window is split into pieces (lesson 06-07), and line 5 of
   `time/tzdata/zzipdata.go` has two pairs of equal 700-byte pieces: same path, same line,
   same text, so the same `ChunkID`. The corpus had 41,980 chunks and 41,978 IDs. The tests
   and the 2 s reproductions never reached document 2,946; the first full run did:

   ```
   err=chunk already handled: 8cda2bced5f28954dd6979adea8ab6ca9d485ab719f75bf481292ec8dbb1ba21 state=failed documents=2946 chunks=40214 embedded=40092 failed=0 pending=122
   ```

## Fix

1. **Backpressure.** The queue is gone. The cutter takes one of `MaxPending` slots (256 by
   default) before it counts a chunk as cut, and a worker gives it back after counting the
   chunk as embedded or failed; the chunks wait in a channel with room for `MaxPending`.
   When the slots run out, the cutter waits for the model. A buffered channel alone bounds
   the chunks too, but not at the configured number: with 8 slots and 2 workers, 11 chunks
   were pending at once (8 in the channel, 2 in the workers' hands, 1 in the cutter's).
2. **Every chunk has its own ID.** `NewChunkID` takes the byte where the chunk starts in its
   file (ADR 0002, Changes). The full run completes: 2,972 documents, 41,980 chunks
   embedded.

## Regression tests

`TestRunIngestionNeverHoldsMoreThanMaxPending` (`internal/app/ingestion/ingest_test.go`)
runs 100 chunks through 2 workers and an embedder that takes 1 ms, with a limit of 8, and
checks the pending count on every embed call. Against the queue:

```
--- FAIL: TestRunIngestionNeverHoldsMoreThanMaxPending (0.12s)
    ingest_test.go:230: 99 chunks pending at once, want at most 8 (RN03)
```

With the slot given back when the worker takes the chunk, before embedding it:

```
--- FAIL: TestRunIngestionNeverHoldsMoreThanMaxPending (0.12s)
    ingest_test.go:230: 10 chunks pending at once, want at most 8 (RN03)
```

`TestEveryChunkHasItsOwnID` (`internal/domain/ingestion/chunk_test.go`) cuts a line of 30
equal bytes into three equal pieces of 10 and wants three IDs. Before the offset:

```
--- FAIL: TestEveryChunkHasItsOwnID (0.00s)
    chunk_test.go:96: chunk #2 (2-2 "xxxxxxxxxx") has the ID of chunk #1
    chunk_test.go:96: chunk #3 (2-2 "xxxxxxxxxx") has the ID of chunk #1
```

## Where Garimpo meets it

Every stage of the ingestion that hands work to a slower one does it through a bounded
channel, and the limit is a number in `Config`, not the size of the corpus. Module 08 makes
the embedder take chunks in batches, and `MaxPending` bounds the chunks those batches wait
in; module 09 makes the number of workers follow the model's latency.
