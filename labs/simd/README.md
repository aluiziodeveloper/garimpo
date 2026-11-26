# labs/simd

The dot product of 384 dimensions with the SIMD of Go 1.27: `simd/archsimd`, the vector
registers of amd64 as types, and `simd`, the portable package. Both exist only with
`GOEXPERIMENT=simd`, are experimental, and are not covered by the Go 1 promise.

```sh
GOEXPERIMENT=simd go test -bench . ./labs/simd
go run ./labs/simd   # without the experiment: says what it needs
```

Measured on 26/11/2026 (Go 1.27.4, i7-13700H under WSL 2, one virtual CPU: `BENCH_CPUSET=2`, 10 runs,
`benchstat`):

| Version | sec/op |
|---|---|
| `scalar` | 142.6n ± 0% |
| `avx2` | 57.73n ± 1% |
| `avx2-no-clear` | 144.9n ± 1% |
| `portable` | 154.4n ± 2% |
| `portable-clear` | 69.75n ± 1% |

- `avx2` multiplies and adds 8 float32 per instruction (`MulAdd`, the FMA of AVX2).
- Without `archsimd.ClearAVXUpperBits`, the sum after the loop uses SSE instructions with
  the upper half of the registers dirty, and the gain is gone. `ClearAVXUpperBits` is
  `VZEROUPPER`; its documentation says the compiler may emit it on its own in the future.
- The portable `simd` builds the function once per width and picks one when it runs; on
  this machine it used the same `VFMADD213PS` as `avx2`, and did not clear the registers.
  With `ClearAVXUpperBits` added, it is at 69.75 ns.
- The tests skip on a processor without AVX2 and FMA.
