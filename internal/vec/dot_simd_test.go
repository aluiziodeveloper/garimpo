//go:build goexperiment.simd && amd64

package vec

import (
	"math"
	"math/rand/v2"
	"simd/archsimd"
	"testing"
)

// The AVX2 version adds the products in another order than the scalar one: they agree within
// the error of a float32 sum. The lengths cover the tail that is not a multiple of 8, and the
// offset a slice that does not start at the beginning of its array.
func TestSIMDAgreesWithTheScalarVersion(t *testing.T) {
	if !archsimd.X86.AVX2() || !archsimd.X86.FMA() {
		t.Skip("this processor has no AVX2 with FMA")
	}
	r := rand.New(rand.NewPCG(1, 2))
	for n := range 100 {
		for _, off := range []int{0, 1, 3} {
			a, b := make([]float32, n+off), make([]float32, n+off)
			var size float64
			for i := range a {
				a[i], b[i] = r.Float32()*2-1, r.Float32()*2-1
				size += math.Abs(float64(a[i]) * float64(b[i]))
			}
			a, b = a[off:], b[off:]
			got, want := float64(dotAVX2(a, b)), float64(dotScalar(a, b))
			if tol := float64(n+1) * 0x1p-23 * size; math.Abs(got-want) > tol {
				t.Errorf("n=%d off=%d: AVX2 = %v, scalar = %v, want within %.3g", n, off, got, want, tol)
			}
		}
	}
}
