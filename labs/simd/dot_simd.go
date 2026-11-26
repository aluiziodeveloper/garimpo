//go:build goexperiment.simd && amd64

package main

import (
	"simd"
	"simd/archsimd"
)

// dims is the dimension of all-minilm, the model of the course: 48 registers of 8 float32.
const dims = 384

// versions are the dot products the lab compares, by name.
var versions = []struct {
	name string
	dot  func(a, b []float32) float32
}{
	{"scalar", dotScalar},
	{"avx2", dotAVX2},
	{"avx2-no-clear", dotAVX2NoClear},
	{"portable", dotPortable},
	{"portable-clear", dotPortableClear},
}

// dotScalar is the loop of one float32 at a time.
func dotScalar(a, b []float32) float32 {
	var s float32
	for i, x := range a {
		s += x * b[i]
	}
	return s
}

// dotAVX2 multiplies and adds 8 float32 per instruction: MulAdd is the FMA of AVX2,
// acc + x*y in one step. It needs len(a) to be a multiple of 8, as 384 is.
func dotAVX2(a, b []float32) float32 {
	var acc archsimd.Float32x8
	for i := 0; i+8 <= len(a); i += 8 {
		acc = archsimd.LoadFloat32x8(a[i:]).MulAdd(archsimd.LoadFloat32x8(b[i:]), acc)
	}
	var out [8]float32
	acc.Store(out[:])
	// The sum below uses the older SSE instructions, which share the registers: with the
	// upper half of the registers still dirty, each one waits for the AVX ones before it.
	archsimd.ClearAVXUpperBits()
	return out[0] + out[1] + out[2] + out[3] + out[4] + out[5] + out[6] + out[7]
}

// dotAVX2NoClear is dotAVX2 without ClearAVXUpperBits.
func dotAVX2NoClear(a, b []float32) float32 {
	var acc archsimd.Float32x8
	for i := 0; i+8 <= len(a); i += 8 {
		acc = archsimd.LoadFloat32x8(a[i:]).MulAdd(archsimd.LoadFloat32x8(b[i:]), acc)
	}
	var out [8]float32
	acc.Store(out[:])
	return out[0] + out[1] + out[2] + out[3] + out[4] + out[5] + out[6] + out[7]
}

// dotPortable uses the portable Float32s, as wide as the machine allows: 8 float32 with
// AVX2, 16 with AVX-512. It needs len(a) to be a multiple of that width.
func dotPortable(a, b []float32) float32 {
	var acc simd.Float32s
	n := acc.Len()
	for i := 0; i+n <= len(a); i += n {
		acc = simd.LoadFloat32s(a[i:]).MulAdd(simd.LoadFloat32s(b[i:]), acc)
	}
	var out [16]float32
	acc.Store(out[:])
	var s float32
	for _, x := range out[:n] {
		s += x
	}
	return s
}

// dotPortableClear is dotPortable with ClearAVXUpperBits before the sum.
func dotPortableClear(a, b []float32) float32 {
	var acc simd.Float32s
	n := acc.Len()
	for i := 0; i+n <= len(a); i += n {
		acc = simd.LoadFloat32s(a[i:]).MulAdd(simd.LoadFloat32s(b[i:]), acc)
	}
	var out [16]float32
	acc.Store(out[:])
	archsimd.ClearAVXUpperBits()
	var s float32
	for _, x := range out[:n] {
		s += x
	}
	return s
}
