package seq

import (
	"errors"
	"iter"
	"slices"
	"strings"
	"testing"
)

// from yields the values, then the error if it is not nil.
func from[T any](values []T, err error) Results[T] {
	return func(yield func(T, error) bool) {
		for _, v := range values {
			if !yield(v, nil) {
				return
			}
		}
		if err != nil {
			var zero T
			yield(zero, err)
		}
	}
}

func letters(s string) iter.Seq[string] {
	return func(yield func(string) bool) {
		for _, r := range s {
			if !yield(string(r)) {
				return
			}
		}
	}
}

func TestFlatMapYieldsEveryValueInOrder(t *testing.T) {
	var got []string
	err := from([]string{"ab", "", "c"}, nil).FlatMap(letters).Each(func(s string) {
		got = append(got, s)
	})
	if err != nil || !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Errorf("FlatMap(letters) = %q, %v; want [a b c], nil", got, err)
	}
}

func TestEachReturnsTheErrorAfterTheValues(t *testing.T) {
	broken := errors.New("unreadable file")
	var got []string
	err := from([]string{"ab"}, broken).FlatMap(letters).Each(func(s string) {
		got = append(got, s)
	})
	if !errors.Is(err, broken) || !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("Each() = %q, %v; want [a b], %v", got, err, broken)
	}
}

func TestPeekSeesEachValueOnce(t *testing.T) {
	var seen []string
	n := 0
	err := from([]string{"ab", "cd"}, nil).Peek(func(s string) {
		seen = append(seen, s)
	}).FlatMap(letters).Each(func(string) { n++ })
	if err != nil || !slices.Equal(seen, []string{"ab", "cd"}) || n != 4 {
		t.Errorf("Peek saw %q and Each %d values, error %v", seen, n, err)
	}
}

func TestBreakStopsEveryStage(t *testing.T) {
	pulled := 0
	words := Results[string](func(yield func(string, error) bool) {
		for _, w := range strings.Fields("ab cd ef") {
			pulled++
			if !yield(w, nil) {
				return
			}
		}
	})
	for _, err := range words.FlatMap(letters).All() {
		if err != nil {
			t.Fatal(err)
		}
		break
	}
	if pulled != 1 {
		t.Errorf("the loop broke at the first letter and the source yielded %d words", pulled)
	}
}
