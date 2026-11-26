# ADR 0002: Bounded contexts, layers and the shared kernel

- **Status:** accepted
- **Date:** 2026-11-25
- **Module:** 03, Garimpo's architecture

## Context

Garimpo reads a corpus, cuts it into chunks, embeds them, indexes them and answers questions
citing file and line. The first prototype did it with `[]float32`, `string` and `int`: no type
said that a vector has the model's dimension and norm 1, or that a citation points to lines
that exist. The bugs came out as plausible numbers, never as errors (INC-03). The modules
ahead add concurrency, a journal, two indexes and HTTP; without boundaries, each one would
reach into the others.

## Decision

1. **Five bounded contexts**, each with its own words:

   | Context | Words | Main types |
   |---|---|---|
   | Ingestion | document, chunk, ingestion run, checkpoint | `IngestionRun`, `Document`, `Chunk` |
   | Indexing | vector, index snapshot | `IndexSnapshot` |
   | Retrieval | query, hit, ranking, fusion | `Query`, `Hit`, `Ranking` |
   | Answering | answer, citation, grounding | `Answer` |
   | Evaluation | case, gold answer, metric | `EvalCase`, `EvalRun`, `Metric` |

2. **A shared kernel**, `internal/domain/shared`, with the three types every context uses:
   `ChunkID`, `Vector` and `Citation`. It changes only when every context agrees.
3. **An anticorruption layer around Ollama.** The domain and the use cases see the ports
   `Embedder` and `Generator`, in Garimpo's words; the adapter translates Ollama's API, which
   never enters the domain.
4. **Hexagonal layers, enforced by the lint** (`depguard` and `forbidigo` in `.golangci.yml`):

   | Layer | Packages | May not |
   |---|---|---|
   | Domain | `internal/domain/...` | import `sync`, `net`, `os`, `app` or `adapters`; call `time.Now` |
   | Use cases | `internal/app/...` | import `adapters` or `net/http` |
   | Adapters | `internal/adapters/...` | (no rule yet) |
   | Composition root | `cmd/garimpo` | hold business rules (review, not lint) |
   | Labs | `labs/...` | import `internal/` |

   Ports are declared where they are consumed: `ChunkFinder` lives in `shared`, next to
   `Citation`, and the index will implement it.
5. **Value objects guard the invariants.** Unexported fields, one constructor that validates
   and returns an error wrapping a sentinel (`ErrInvalidChunkID`, `ErrInvalidVector`,
   `ErrInvalidCitation`), and a zero value that means "none" and is safe to use:
   - `ChunkID` is the SHA-256 of path, byte offset, line range and text, with NUL
     separators: the same chunk gets the same ID in every run (RN02), and two chunks never
     share one (see Changes);
   - `Vector` copies its input, scales it to norm 1 with a float64 sum and never shares
     memory with the caller (RN07); `Dot` panics on a dimension mismatch;
   - `Citation` exists only for lines inside a chunk the `ChunkFinder` knows (RN01).

## Measurements behind it

All taken on 2026-11-25 with Go 1.27.4.

| Decision | Evidence |
|---|---|
| Lint boundaries | A domain file importing `sync` and calling `time.Now` builds, and the lint stops it: `import 'sync' is not allowed from list 'domain'` (depguard) and ``use of `time.Now` forbidden`` (forbidigo) |
| NUL separators | Without them, `("x1", line 2)` and `("x", line 12)` got the same ID: `share the ID b2c486e3…` |
| No shared memory | Normalizing in place changed the caller's slice to `[0.6 0.8]`; returning the inner slice let a caller push the norm to 100.0018 |
| float64 sum | A float32 sum passed every example test and failed both property tests: values near 3e38 square to `+Inf`, and the vector becomes zeros |

## Alternatives considered

- **`type ChunkID string`, `type Vector []float32`.** Any conversion, `ChunkID("x")`, skips
  the constructor, and a slice type shares memory. Rejected for structs with unexported
  fields.
- **Checking the boundaries in review.** A rule that nothing runs erodes. Rejected for
  the lint, which fails the CI.
- **`Dot` returning an error.** It is called once per vector in every search, and a mismatch
  means vectors of two models in one index: a bug to stop, not an input to handle. Rejected.
- **One package per context now.** Only the shared kernel has code; the contexts get their
  packages when their first type arrives. Deferred.

## Consequences

- An invalid `Vector` or `Citation` cannot exist outside its package: the bug of INC-03 is a
  constructor error at the edge, with a message, not a score.
- Every adapter translates into these types, and every use case receives them already valid.
- A new layer rule is one entry in `.golangci.yml`, and breaking it fails the CI.

## Changes

- **2026-11-25, module 06: `internal/seq`, a support package outside the layers.** It holds
  the iterator helpers the layers share (`Results[T]`, with the generic method `FlatMap`).
  It has no business rules, imports only the standard library and declares no port: a
  generic method satisfies no interface, so ports take an `iter.Seq2`. Any layer may
  import it; today the use case `ingestion.SurveyCorpus` does. The labs still may not,
  since their rule denies all of `internal`. Keeping it free of Garimpo's packages is a
  review rule, not a lint one.
- **2026-11-25, module 07: the aggregate is `ingestion.Run`.** The package already names the
  context, and `ingestion.IngestionRun` would say it twice. The context's words do not
  change: it is still the ingestion run.
- **2026-11-25, module 07: the byte offset joined the `ChunkID`.** The first ingestion of
  the whole corpus failed with `chunk already handled`: the `ingestion.Run` refused a chunk
  it had already confirmed. A line longer than the window is split into pieces (lesson
  06-07), and line 5 of `time/tzdata/zzipdata.go` has two pairs of equal 700-byte pieces:
  same path, same line, same text, same ID. The corpus had 41,980 chunks and 41,978 IDs.
  `NewChunkID` now takes the byte where the chunk starts in its file, so equal pieces are
  different chunks, and the IDs of a run stay the same from one run to the next.
- **2026-11-26, module 11: `internal/vec`, a second support package outside the layers.** It
  holds the dot product (`vec.Dot(a, b []float32)`), which every search calls once per
  vector of the index. It works on plain `[]float32`, so the domain keeps the rules (the
  `Vector` checks the dimension and calls `vec.Dot`) and `vec` keeps the speed: the faster
  versions of module 11, SIMD included, change one package and no layer. The same rules as
  `internal/seq` apply: no business rules, only the standard library, any layer may import it.
