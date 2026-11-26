// Package vec holds the arithmetic of the vectors Garimpo searches with: the dot product,
// called once for every vector of the index in every search. It has no business rules and
// imports only the standard library, so any layer may use it.
//
// Built with GOEXPERIMENT=simd on amd64, Dot uses the AVX2 instructions when the processor
// has them (dot_simd.go); otherwise, the scalar version (dot_other.go).
package vec

import "fmt"

// Dot returns the dot product of a and b. It panics if their lengths differ.
func Dot(a, b []float32) float32 {
	if len(a) != len(b) {
		panic(fmt.Sprintf("vec.Dot: lengths %d and %d", len(a), len(b)))
	}
	return dot(a, b)
}

// dotScalar is the dot product one float32 at a time, for any processor. The caller has
// checked that the lengths are equal.
func dotScalar(a, b []float32) float32 {
	b = b[:len(a)]
	// Four sums instead of one: each addition waits only for the one four products back,
	// so the processor runs four at a time. The order of the additions changes, and so
	// does the rounding, within the error a float32 sum has anyway.
	var s0, s1, s2, s3 float32
	i := 0
	for ; i+4 <= len(a); i += 4 {
		s0 += a[i] * b[i]
		s1 += a[i+1] * b[i+1]
		s2 += a[i+2] * b[i+2]
		s3 += a[i+3] * b[i+3]
	}
	for ; i < len(a); i++ {
		s0 += a[i] * b[i]
	}
	return s0 + s1 + s2 + s3
}
