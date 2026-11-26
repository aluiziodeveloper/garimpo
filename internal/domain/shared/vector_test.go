package shared

import (
	"errors"
	"math"
	"testing"
)

const tolerance = 1e-6

func norm(v Vector) float64 {
	var s float64
	for _, x := range v.Values() {
		s += float64(x) * float64(x)
	}
	return math.Sqrt(s)
}

func TestNewVectorNormalizes(t *testing.T) {
	v, err := NewVector([]float32{3, 4}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if got := norm(v); math.Abs(got-1) > tolerance {
		t.Errorf("norm = %v, want 1", got)
	}
	want := []float32{0.6, 0.8}
	for i, x := range v.Values() {
		if math.Abs(float64(x-want[i])) > tolerance {
			t.Errorf("Values()[%d] = %v, want %v", i, x, want[i])
		}
	}
}

func TestNewVectorRejectsInvalidInput(t *testing.T) {
	nan := float32(math.NaN())
	inf := float32(math.Inf(1))
	tests := []struct {
		name   string
		values []float32
		dim    int
	}{
		{"other model's dimension", []float32{0.1, 0.1, 0.1, 0.1, 0.5, 0.5, 0.5, 0.5}, 4},
		{"all zeros", []float32{0, 0, 0, 0}, 4},
		{"NaN", []float32{nan, 1, 1, 1}, 4},
		{"infinity", []float32{inf, 1, 1, 1}, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v, err := NewVector(tt.values, tt.dim)
			if !errors.Is(err, ErrInvalidVector) {
				t.Errorf("err = %v, want one that wraps ErrInvalidVector", err)
			}
			if !v.IsZero() {
				t.Errorf("v = %v, want the zero Vector", v.Values())
			}
		})
	}
}

func TestVectorDoesNotShareMemory(t *testing.T) {
	in := []float32{3, 4}
	v, _ := NewVector(in, 2)
	if in[0] != 3 || in[1] != 4 {
		t.Errorf("NewVector changed its input: %v", in)
	}
	in[0] = 100
	out := v.Values()
	out[1] = 100
	if got := norm(v); math.Abs(got-1) > tolerance {
		t.Errorf("norm = %v after changing the input and the output, want 1", got)
	}
}

func TestVectorDot(t *testing.T) {
	x, _ := NewVector([]float32{1, 0}, 2)
	y, _ := NewVector([]float32{0, 5}, 2)
	if got := x.Dot(x); math.Abs(float64(got)-1) > tolerance {
		t.Errorf("x·x = %v, want 1", got)
	}
	if got := x.Dot(y); got != 0 {
		t.Errorf("x·y = %v, want 0", got)
	}
}

func TestVectorDotPanicsOnDimensionMismatch(t *testing.T) {
	x, _ := NewVector([]float32{1, 0}, 2)
	y, _ := NewVector([]float32{1, 0, 0}, 3)
	defer func() {
		if recover() == nil {
			t.Error("Dot of a 2- and a 3-dimension vector did not panic")
		}
	}()
	x.Dot(y)
}

func TestZeroVector(t *testing.T) {
	var v Vector
	if !v.IsZero() || v.Dim() != 0 {
		t.Errorf("zero Vector: IsZero() = %v, Dim() = %d", v.IsZero(), v.Dim())
	}
}

// Every search calls Dot once for each vector of the index: 384 is the dimension of
// all-minilm, the model of the course.
func BenchmarkVectorDot(b *testing.B) {
	values := make([]float32, 384)
	for i := range values {
		values[i] = float32(i%7) - 3
	}
	v, err := NewVector(values, 384)
	if err != nil {
		b.Fatal(err)
	}
	for b.Loop() {
		sinkScore = v.Dot(v)
	}
}

var sinkScore float32
