# ADR 0005: Searches that read snapshots, and texts held by weak pointers

- **Status:** accepted
- **Date:** 2026-11-27
- **Module:** 14, Lock-free reads and CQRS

## Context

Garimpo will search the index while the ingestion adds to it. The exact index (ADR 0003)
guards its vectors with one `sync.RWMutex`, and every `Add` waited for the scans under way:
5 ms instead of 1 µs (INC-14). RN05 asks that a query never see the index half-changed: it
reads a complete version. And the searches will need the text of the chunks they return,
which the index does not keep.

## Decision

1. **`indexing.IndexSnapshot` in the domain** (`internal/domain/indexing`): one version of
   the index, immutable, with a version number. `With` returns the next version and leaves
   this one as it was. It writes just past the end of the shared arrays when no other version
   wrote there, so a growing index copies only when its arrays fill (to twice their size,
   at least 64 chunks); a second version made from the same snapshot copies. `With` is for
   one caller at a time; reading a snapshot is safe from any number of goroutines.
2. **A lightweight CQRS in `flatindex.Published`.** The command side, `Add`, takes a
   `sync.Mutex` that only the `Add` calls share, tells a chunk already held, makes the next
   snapshot and stores it in an `atomic.Pointer`. The query side, `Search`, loads the
   pointer and scans that snapshot, with no lock. Both sides are one process and one store:
   no bus, no second model.
3. **Eventual consistency, bounded by the `Add`.** A search sees every `Add` that returned
   before it started, and maybe some under way.
4. **`flatindex.Index` stays** as it was, for the comparison of the lessons. RN05 holds for
   `Published`; the HNSW index keeps its lock per node (ADR 0004).
5. **The scan through an iterator.** `IndexSnapshot.Scores` yields each chunk with its dot
   product; the adapter keeps the best with `topk`. It costs 3 allocations more per search
   than the loop of `Index`: 7 against 4.
6. **`chunktext.Store` for the texts.** It keeps where each chunk is, path, byte offset,
   length and lines, and implements `shared.ChunkFinder`. It reads a text from the corpus
   when asked, only its bytes (`io.ReaderAt`), and checks it against the `ChunkID`. A text
   read is shared while some caller holds its `*Text`: the store keeps a `weak.Pointer` to
   it, and `runtime.AddCleanup` takes the entry out after the GC frees it. `ingestion.Chunk`
   gained `Offset`.

## Measurements behind it

All taken on 2026-11-27 with Go 1.27.4, i7-13700H in WSL 2, 8 virtual CPUs; the 39,244
vectors of the corpus or the 41,980 chunks of the Go library.

| Decision | Evidence |
|---|---|
| Publish snapshots | 4 goroutines searching, one `Add` every 35 ms: `Add` p50 3.388 ms with `Index`, 24 µs with `Published`; search p50 6.442 against 6.842 ms |
| Copy on write that shares the arrays | `labs/cow`, 57.2 MiB list: an append took 5.404 ms with `RWMutex`, 10.027 ms copying the list, 1 µs writing past its end |
| The iterator, not a loop in the domain | Search of 41,980 random vectors, one CPU, 10 interleaved runs: 5.762 ms for `Index`, 5.714 with the iterator, 5.643 with a loop (p = 0.912 and 0.739) |
| Bounded staleness | From the start of an `Add` to a reader seeing it: p50 25 µs and max 63 µs with `Published`, 4.783 and 8 ms with `Index` |
| Texts read on demand | Every text held: 34.5 MiB of heap; the store with the IDs and the places, 8.3 MiB |
| The weak cache | 8 goroutines, 4,000 queries of 10 texts: 40,000 reads and 90.0 MiB allocated without the cache, 9,993 reads and 23.4 MiB with it |
| Only the bytes of the chunk | 1,000 texts held as substrings of their files: 108.7 MiB; as copies, 0.65 MiB |

## Alternatives considered

- **Keep the `RWMutex`, shorten the scans.** The `Add` would still wait for a scan, only a
  shorter one, and the searches would take turns with the writes. Rejected.
- **Copy the whole index at every `Add`.** Correct and simple: 10 ms per append on 57 MiB in
  the lab, and garbage to collect. Rejected.
- **Share the arrays without the check of who wrote last.** Two versions made from one
  snapshot overwrite each other's chunk (the 14-05 test). Rejected.
- **Publish from a goroutine, so `Add` returns sooner.** A search could miss an `Add` that
  had returned, and the next `Add` could start from an old version. Rejected.
- **Keep every text in memory.** 34.5 MiB for the library, more for a bigger corpus, most of
  it never shown. Rejected.

## Consequences

- An `Add` of `Published` no longer waits for the searches; the searches never wait for an
  `Add`. Adds still wait for each other, a few microseconds each.
- A snapshot held by a slow search keeps its arrays alive; after a reallocation, the old and
  the new arrays exist at once until the last search of the old version ends.
- The texts of the chunks cost memory only while something holds them, and two misses of
  the same chunk at once both read the corpus; the second one shares the first text stored.
- No command searches yet: the search use case comes with the hybrid search of module 15.
