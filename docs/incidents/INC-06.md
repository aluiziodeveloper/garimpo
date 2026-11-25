# INC-06: 28 MB to read one file

- **Date:** 2026-11-25
- **Module:** 06, Reading with iterators
- **Reproduction:** `labs/load-all`, and `cmd/garimpo` at tag `aula-06-10`
  (`BenchmarkSurveyCorpus`)

## Summary

The first ingestion read the whole corpus, the Go standard library, into a slice before it
looked at any file. To find one file of 3.5 KB it held 28 MB. Nothing failed: the memory
just grew with the corpus. Reading one document at a time with an iterator held 0.1 MB.

Two more bugs came out of the same code. The validation prototype had silently skipped 27
files of the corpus, and the first chunker let a chunk of 1.3 MB through.

## Impact

`labs/load-all`, `lab` service (2 CPUs), Go 1.27.4, looking for `strings/builder.go`
(3,589 bytes) in 2,972 files; three runs, all equal:

| Mode | Files read | Heap in use after a collection | Allocated |
|---|---|---|---|
| `all`: load, then search | 2,972 | 27.9 MB | 57.9 MB |
| `each`: one file at a time, stop when found | 2,610 | 0.1 MB | 42.2 MB |

`BenchmarkSurveyCorpus`, the whole corpus read and cut into chunks, `bench` service (CPUs 2
and 4 of an i7-13700H), 10 runs of 5 surveys:

| Mode | Time | Peak live heap | Allocated |
|---|---|---|---|
| `stream`: `corpus.Walk` straight into `SurveyCorpus` | 71.47 ms ± 4% | 6.431 MiB ± 32% | 89.10 MiB |
| `load`: every document in a slice first | 69.03 ms ± 3% | 31.789 MiB ± 10% | 89.27 MiB |

Both modes allocate the same and take about the same time; what changes is how much is
alive at once. Loading first holds about five times as much, and that grows with the corpus.

## Cause

A slice of documents keeps every document alive until the slice dies, even after the
caller is done with it. The search needed one file; the slice held all of them.

Two more bugs, found while measuring:

1. **Files lost without an error.** The prototype of the validation (proof A) cut each file
   with a `bufio.Scanner` and ignored the error `filepath.WalkDir` returned.
   `time/tzdata/zzipdata.go` has a line of 1,375,316 bytes, over the scanner's 1 MB buffer:
   the scanner failed with `bufio.Scanner: token too long`, the walk stopped there, and the
   27 files after it in lexical order were never cut. Proof A counted 2,945 files and 38,659
   chunks; the corpus has 2,972.
2. **A chunk too large to embed.** The first `Cutter` kept a line longer than the window
   as one chunk. The largest chunk of the corpus was that line of `zzipdata.go`, 1,375,317
   bytes; the model would have cut it silently, the risk D15 was written to avoid.

## Fix

1. **Streaming.** `corpus.Walk` returns an `iter.Seq2[Document, error]` that reads one file
   per step, only when the loop asks for it, and stops the walk with `fs.SkipAll` when the
   loop breaks. `SurveyCorpus` consumes it with `seq.Results`, which holds one document at
   a time.
2. **Every error reaches the caller.** A read or validation error is the last step of
   `Walk`. `errcheck` flags a `for range` over the `Seq2` that drops it.
3. **No chunk over the window.** A line longer than the window is split into pieces of the
   window's size, never inside a character, and the window must be at least
   `utf8.UTFMax`. With the default window of 700, the corpus has 41,980 chunks, averaging
   653 bytes, the largest 700.

## Regression tests

`TestWalkStopsWhenTheLoopBreaks` (`internal/adapters/corpus/corpus_test.go`) breaks the
loop after one document and wants one file opened. With `return nil` in place of
`fs.SkipAll`, the walk calls `yield` again:

```
--- FAIL: TestWalkStopsWhenTheLoopBreaks (0.00s)
panic: runtime error: range function continued iteration after function for loop body returned false [recovered, repanicked]
```

`FuzzCut` (`internal/domain/ingestion/chunk_fuzz_test.go`) checks that the chunks rebuild
the text, are never empty, never larger than the window and always valid UTF-8, and that
their lines have no gap. Its first run split a line inside a character in 0.17 s:

```
--- FAIL: FuzzCut (0.17s)
    --- FAIL: FuzzCut (0.00s)
        chunk_fuzz_test.go:34: chunk 1-1 is not valid UTF-8: "\xcb"
```

The input it saved, `"ː"` with a window of 1, then gave an empty chunk once the cut backed
off to the start of the character; `NewCutter` now rejects a window smaller than 4 bytes.
The seed `strings.Repeat("ː", 5)` with a window of 5 keeps the split case in every CI run.
After the fix, 120 s of fuzzing ran 2,908,772 inputs without a failure.

## Where Garimpo meets it

The ingestion never holds the corpus: documents and chunks flow through iterators, one at a
time, and a slice of the whole corpus is a review finding. RN03, which module 07 enforces,
limits the chunks pending in memory; this incident is why it can hold.
