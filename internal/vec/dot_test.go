package vec_test

import (
	"math"
	"math/rand/v2"
	"strings"
	"testing"

	"github.com/aluiziodeveloper/garimpo/internal/vec"
)

func random(n int, r *rand.Rand) []float32 {
	v := make([]float32, n)
	for i := range v {
		v[i] = r.Float32()*2 - 1
	}
	return v
}

// Dot sums float32 products, and a float32 sum rounds at every step: it is checked against
// the sum in float64, within the error that n roundings can add.
func TestDotIsTheSumOfTheProducts(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	for _, n := range []int{0, 1, 2, 3, 4, 5, 7, 8, 9, 15, 16, 17, 31, 33, 100, 384, 768} {
		a, b := random(n, r), random(n, r)
		var want, size float64
		for i := range a {
			p := float64(a[i]) * float64(b[i])
			want += p
			size += math.Abs(p)
		}
		got := float64(vec.Dot(a, b))
		if tol := float64(n+1) * 0x1p-24 * size; math.Abs(got-want) > tol {
			t.Errorf("n=%d: Dot = %v, want %v within %.3g", n, got, want, tol)
		}
	}
}

func TestDotPanicsOnDifferentLengths(t *testing.T) {
	defer func() {
		r := recover()
		if msg, _ := r.(string); !strings.Contains(msg, "lengths 3 and 2") {
			t.Fatalf("Dot of lengths 3 and 2 panicked with %v", r)
		}
	}()
	vec.Dot([]float32{1, 2, 3}, []float32{1, 2})
}

var sink float32

// 384 is the dimension of all-minilm, the model of the course.
func BenchmarkDot(b *testing.B) {
	r := rand.New(rand.NewPCG(1, 2))
	x, y := random(384, r), random(384, r)
	for b.Loop() {
		sink = vec.Dot(x, y)
	}
}
