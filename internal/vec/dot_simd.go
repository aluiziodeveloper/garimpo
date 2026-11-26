//go:build goexperiment.simd && amd64

package vec

import "simd/archsimd"

// hasAVX2 is whether this processor runs the AVX2 version: it needs the FMA instructions
// too, and an instruction the processor lacks stops the program.
var hasAVX2 = archsimd.X86.AVX2() && archsimd.X86.FMA()

func dot(a, b []float32) float32 {
	if !hasAVX2 {
		return dotScalar(a, b)
	}
	return dotAVX2(a, b)
}

// dotAVX2 multiplies and adds 8 float32 per instruction, with the FMA of AVX2, and adds the
// tail that is not a multiple of 8 one at a time. The caller has checked that the lengths
// are equal.
func dotAVX2(a, b []float32) float32 {
	b = b[:len(a)]
	// Four registers of sums, as the scalar version has four sums: each FMA waits only for
	// the one four back.
	var acc0, acc1, acc2, acc3 archsimd.Float32x8
	i := 0
	for ; i+32 <= len(a); i += 32 {
		acc0 = archsimd.LoadFloat32x8(a[i:]).MulAdd(archsimd.LoadFloat32x8(b[i:]), acc0)
		acc1 = archsimd.LoadFloat32x8(a[i+8:]).MulAdd(archsimd.LoadFloat32x8(b[i+8:]), acc1)
		acc2 = archsimd.LoadFloat32x8(a[i+16:]).MulAdd(archsimd.LoadFloat32x8(b[i+16:]), acc2)
		acc3 = archsimd.LoadFloat32x8(a[i+24:]).MulAdd(archsimd.LoadFloat32x8(b[i+24:]), acc3)
	}
	for ; i+8 <= len(a); i += 8 {
		acc0 = archsimd.LoadFloat32x8(a[i:]).MulAdd(archsimd.LoadFloat32x8(b[i:]), acc0)
	}
	acc := acc0.Add(acc1).Add(acc2.Add(acc3))
	var out [8]float32
	acc.Store(out[:])
	// What follows uses SSE instructions: with the upper half of the registers dirty, each
	// one would wait for the AVX ones (lesson 11-07: from 58 to 145 ns).
	archsimd.ClearAVXUpperBits()
	s := out[0] + out[1] + out[2] + out[3] + out[4] + out[5] + out[6] + out[7]
	for ; i < len(a); i++ {
		s += a[i] * b[i]
	}
	return s
}
