# INC-11: the dot product that rules the search

- **Date:** 2026-11-26
- **Module:** 11, Fast vectors
- **Reproduction:** a temporary benchmark on `main` after tag `aula-10-10`: an exact search over 41,980
  random vectors of 384 dimensions (as many as the chunks of the whole corpus, module 07),
  each query scoring every vector with `Vector.Dot` and keeping the 10 best, profiled with
  `-cpuprofile` on CPUs 2 and 4

## Summary

Garimpo has no index yet (module 12), but every search it will do has the same core: one
dot product per vector of the index, per query. The first measurement of that core, before
any index code, showed where the time of a search goes: 89.5% of the CPU was in
`Vector.Dot`, a plain loop of one `float32` at a time. Whatever the index does around it,
the search cannot be faster than that loop lets it be.

Nothing was wrong: the results were right and nothing failed. The incident is a cost found
before it was paid, at 7.2 to 8.0 ms per query over the corpus.

## Impact

```
BenchmarkINC11Search-2   	     154	   7815667 ns/op
BenchmarkINC11Search-2   	     168	   7208249 ns/op
...
      flat  flat%   sum%        cum   cum%
     7.07s 89.49% 89.49%      7.07s 89.49%  .../internal/domain/shared.Vector.Dot
```

The vectors were random: the time of a dot product does not depend on the values.

## Cause

1. **One addition at a time.** The loop had one accumulator: each addition waited for the
   one before it, and the processor, which can run several at once, ran one.
2. **One value per instruction.** The processor has 256-bit registers that hold 8 `float32`
   and an instruction that multiplies and adds all 8 (FMA). The loop used neither.
3. **What did not matter much:** the bounds check of `w.v[i]`. The dimension check of
   `Vector.Dot`, there since module 03 for correctness, already let the compiler prove it
   away; with the check, the loop is 4.32% faster than without it (143.5 → 137.3 ns).

## Fix

1. **`internal/vec`** (ADR 0002): `vec.Dot(a, b []float32)` checks the lengths and holds
   the faster versions; `Vector.Dot` checks the dimension and calls it.
2. **Four accumulators:** −26.11% (137.3 → 101.4 ns, `benchstat`, 10 runs on CPU 2). The
   version that also removes the bounds checks left in the unrolled loop, with pointers to
   arrays, is 24.84% slower than that, and was dropped.
3. **SIMD behind a build tag:** with `GOEXPERIMENT=simd` on amd64, `vec.Dot` uses AVX2 with
   FMA when the processor has both (`simd/archsimd`), and the scalar version otherwise.
   `Dot`: 99.58 → 44.53 ns, −55.28%. The package is experimental and outside the Go 1
   promise: the scalar version stays, and a build without the experiment is the default.

`archsimd.ClearAVXUpperBits` is part of the fix: without it, the AVX2 version takes 144.9 ns
against 57.73 (`labs/simd`), no gain at all. The portable package `simd` does not call it,
and takes 154.4 ns.

## Regression tests

- `TestDotIsTheSumOfTheProducts` checks `vec.Dot` against the sum in `float64` for lengths
  from 0 to 768, within the error of a `float32` sum, in both builds.
- `TestSIMDAgreesWithTheScalarVersion` checks the AVX2 version against the scalar one for
  every length from 0 to 99, at three offsets; it skips on a processor without AVX2 and FMA.
- The CI lints, tests and builds with and without `GOEXPERIMENT=simd`, and a step with `-v`
  shows whether the SIMD tests passed or skipped: on the GitHub runner, they passed.
- `BenchmarkDot` (`internal/vec`) and `BenchmarkVectorDot` (`internal/domain/shared`) keep
  the measurement.

## Where Garimpo meets it

The dot product got 55% faster; the search over the whole corpus, only 20%. The search
reads 64 MB per query, and the processor has 24 MiB of L3 cache. Over corpora of growing
size, scalar and SIMD runs interleaved on CPU 2:

| Vectors | Bytes | Scalar | SIMD | Gain |
|---|---|---|---|---|
| 500 | 0.8 MB | 111.6 ns/vector | 62.15 ns/vector | −44.31% |
| 2,000 | 3 MB | 111.9 | 61.96 | −44.62% |
| 8,000 | 12 MB | 126.0 | 78.71 | −37.53% |
| 41,980 | 64 MB | 153.7 | 123.0 | −19.94% |

While the vectors fit in the cache, SIMD gives its 44%. Past it, each vector costs twice as
much with SIMD, and the time is the memory's. A faster dot product no longer moves the search
much: reading fewer vectors does, which is what the HNSW index of module 13 is for.

Measuring this took a fixed CPU and runs in one session. Inside WSL 2, the 20 virtual CPUs
show as 10 cores of two threads, not the 6 P-cores and 8 E-cores of the i7-13700H, and all
twenty ran `Dot` in 108 to 120 ns: the `cpuset` fixes a virtual CPU, and Hyper-V picks the
core. On CPU 2, the same code took 99.58 ns at 11:38 and 107.8 to 112.0 ns at about 12:45: numbers from two
sessions are not compared, only A and B run side by side and read with `benchstat`.
