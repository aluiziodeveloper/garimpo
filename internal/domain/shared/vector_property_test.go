package shared

import (
	"math"
	"testing"
	"testing/quick"
)

// Properties of NewVector, checked on 1,000 random inputs each. testing/quick draws
// float32 values from the whole range, up to about 3.4e38, and slices of 0 to 49 values.

var quickConfig = &quick.Config{MaxCount: 1000}

func TestPropertyNormIsOne(t *testing.T) {
	f := func(values []float32) bool {
		v, err := NewVector(values, len(values))
		if err != nil {
			return true // invalid input: TestNewVectorRejectsInvalidInput covers it
		}
		return math.Abs(norm(v)-1) <= tolerance
	}
	if err := quick.Check(f, quickConfig); err != nil {
		t.Error(err)
	}
}

func TestPropertyNormalizingTwiceChangesNothing(t *testing.T) {
	f := func(values []float32) bool {
		v, err := NewVector(values, len(values))
		if err != nil {
			return true
		}
		w, err := NewVector(v.Values(), v.Dim())
		if err != nil {
			return false
		}
		for i, x := range w.Values() {
			if math.Abs(float64(x-v.Values()[i])) > tolerance {
				return false
			}
		}
		return true
	}
	if err := quick.Check(f, quickConfig); err != nil {
		t.Error(err)
	}
}

func TestPropertyScoreIsBetweenMinusOneAndOne(t *testing.T) {
	f := func(a, b [8]float32) bool {
		v, errV := NewVector(a[:], 8)
		w, errW := NewVector(b[:], 8)
		if errV != nil || errW != nil {
			return true
		}
		s := float64(v.Dot(w))
		return s >= -1-tolerance && s <= 1+tolerance
	}
	if err := quick.Check(f, quickConfig); err != nil {
		t.Error(err)
	}
}
