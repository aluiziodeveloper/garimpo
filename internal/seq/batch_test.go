package seq

import (
	"errors"
	"slices"
	"testing"
)

func batches(t *testing.T, r Results[[]int]) ([][]int, error) {
	t.Helper()
	var got [][]int
	err := r.Each(func(b []int) { got = append(got, b) })
	return got, err
}

func TestBatchGroupsInOrder(t *testing.T) {
	got, err := batches(t, Batch(from([]int{1, 2, 3, 4, 5}, nil), 2))
	want := [][]int{{1, 2}, {3, 4}, {5}}
	if err != nil || !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("Batch(2) = %v, %v; want %v, nil", got, err, want)
	}
}

func TestBatchOfNothingYieldsNothing(t *testing.T) {
	got, err := batches(t, Batch(from([]int(nil), nil), 3))
	if err != nil || len(got) != 0 {
		t.Errorf("Batch(3) of nothing = %v, %v", got, err)
	}
}

func TestBatchDeliversWhatCameBeforeTheError(t *testing.T) {
	broken := errors.New("unreadable file")
	got, err := batches(t, Batch(from([]int{1, 2, 3}, broken), 2))
	want := [][]int{{1, 2}, {3}}
	if !errors.Is(err, broken) || !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("Batch(2) = %v, %v; want %v, %v", got, err, want, broken)
	}
}

func TestBatchesDoNotShareMemory(t *testing.T) {
	got, _ := batches(t, Batch(from([]int{1, 2, 3, 4}, nil), 2))
	got[1][0] = 99
	if got[0][0] != 1 || got[0][1] != 2 {
		t.Errorf("changing the second batch changed the first: %v", got)
	}
}

func TestBatchBelowOnePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Batch(r, 0) did not panic")
		}
	}()
	Batch(from([]int{1}, nil), 0)
}
