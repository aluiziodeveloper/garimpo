//go:build goexperiment.simd && amd64

package main

import (
	"math"
	"math/rand/v2"
	"simd/archsimd"
	"testing"
)

func random(r *rand.Rand) []float32 {
	v := make([]float32, dims)
	for i := range v {
		v[i] = r.Float32()*2 - 1
	}
	return v
}

// needAVX2 skips the test on a processor without AVX2 and FMA: the instructions would
// stop the program.
func needAVX2(tb testing.TB) {
	tb.Helper()
	if !archsimd.X86.AVX2() || !archsimd.X86.FMA() {
		tb.Skip("this processor has no AVX2 with FMA")
	}
}

// Every version adds the products in another order, and float32 rounds at every step:
// they agree with the scalar one within the error of the sum, not to the last bit.
func TestEveryVersionIsTheDotProduct(t *testing.T) {
	needAVX2(t)
	r := rand.New(rand.NewPCG(1, 2))
	a, b := random(r), random(r)
	want := float64(dotScalar(a, b))
	for _, v := range versions {
		if got := float64(v.dot(a, b)); math.Abs(got-want) > 1e-4 {
			t.Errorf("%s = %v, scalar = %v", v.name, got, want)
		}
	}
}

var sink float32

func BenchmarkDot(b *testing.B) {
	needAVX2(b)
	r := rand.New(rand.NewPCG(1, 2))
	x, y := random(r), random(r)
	for _, v := range versions {
		b.Run(v.name, func(b *testing.B) {
			for b.Loop() {
				sink = v.dot(x, y)
			}
		})
	}
}
