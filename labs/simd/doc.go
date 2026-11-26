// Program simd measures the dot product with the SIMD of Go 1.27: the package simd/archsimd,
// which gives the vector registers of amd64 as types (Float32x8 is 8 float32 in a 256-bit
// register), and the portable package simd, whose Float32s has the width of the machine.
// Both exist only with GOEXPERIMENT=simd, and simd/archsimd only on amd64.
//
//	GOEXPERIMENT=simd go test -bench . ./labs/simd
package main
