package shared

import (
	"errors"
	"fmt"
	"math"
	"slices"

	"github.com/aluiziodeveloper/garimpo/internal/vec"
)

// ErrInvalidVector is wrapped by every error NewVector returns.
var ErrInvalidVector = errors.New("invalid vector")

// Vector is an embedding with the model's dimension and norm 1 (RN07). With norm 1, the
// dot product of two vectors is their cosine similarity, and every score is between -1
// and 1.
//
// A Vector never shares memory with its caller: NewVector copies its input and Values
// returns a copy. The zero value has dimension 0 and means "no vector".
type Vector struct {
	v []float32
}

// NewVector returns values scaled to norm 1. It fails if len(values) is not dim, if a
// value is NaN or infinite, or if every value is zero, since a zero vector has no
// direction to keep.
func NewVector(values []float32, dim int) (Vector, error) {
	if len(values) != dim {
		return Vector{}, fmt.Errorf("%w: %d dimensions, the model has %d", ErrInvalidVector, len(values), dim)
	}
	var sum float64
	for i, x := range values {
		f := float64(x)
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return Vector{}, fmt.Errorf("%w: value %d is %v", ErrInvalidVector, i, x)
		}
		sum += f * f
	}
	if sum == 0 {
		return Vector{}, fmt.Errorf("%w: every value is zero", ErrInvalidVector)
	}
	n := math.Sqrt(sum)
	v := make([]float32, dim)
	for i, x := range values {
		v[i] = float32(float64(x) / n)
	}
	return Vector{v: v}, nil
}

// Dim returns the number of dimensions, 0 for the zero Vector.
func (v Vector) Dim() int {
	return len(v.v)
}

// IsZero reports whether v is the zero Vector.
func (v Vector) IsZero() bool {
	return v.v == nil
}

// Values returns a copy of the values.
func (v Vector) Values() []float32 {
	return slices.Clone(v.v)
}

// Dot returns the dot product of v and w, which is their cosine similarity.
// It panics if the dimensions differ: vectors of two models in one search are a bug in
// the program, not bad input, and a silent score would hide it.
func (v Vector) Dot(w Vector) float32 {
	if len(v.v) != len(w.v) {
		panic(fmt.Sprintf("shared.Vector.Dot: dimensions %d and %d", len(v.v), len(w.v)))
	}
	return vec.Dot(v.v, w.v)
}
