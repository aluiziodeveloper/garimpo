package topk

import (
	"cmp"
	"math/rand/v2"
	"slices"
	"testing"
)

// desc ranks the larger int first.
func desc(a, b int) int { return cmp.Compare(b, a) }

func TestKeeperKeepsTheKBestInOrder(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 1))
	values := make([]int, 1000)
	for i := range values {
		values[i] = r.IntN(500) // repeated values too
	}
	for _, k := range []int{1, 2, 10, 999, 1000, 5000} {
		keep := New(k, desc)
		for _, v := range values {
			keep.Offer(v)
		}
		want := slices.SortedFunc(slices.Values(values), desc)[:min(k, len(values))]
		if got := keep.Sorted(); !slices.Equal(got, want) {
			t.Errorf("k=%d: kept %d values, not the %d best in order", k, len(got), len(want))
		}
	}
}

func TestAKeeperOfferedNothingKeepsNothing(t *testing.T) {
	if got := New(10, desc).Sorted(); len(got) != 0 {
		t.Errorf("Sorted() = %v, want nothing", got)
	}
}

func TestSortedLeavesTheKeeperAsItWas(t *testing.T) {
	keep := New(3, desc)
	for _, v := range []int{5, 1, 9, 7} {
		keep.Offer(v)
	}
	first := keep.Sorted()
	first[0] = -1
	keep.Offer(8)
	if got, want := keep.Sorted(), []int{9, 8, 7}; !slices.Equal(got, want) {
		t.Errorf("after Sorted and Offer(8): %v, want %v", got, want)
	}
}

// Offer allocates nothing: the values never pass through an any, as they would with
// heap.Push and heap.Pop (labs/topk). The values grow, so every one is better than the
// ones kept and replaces the root: at random, few would, and the mean would round to 0.
func TestOfferDoesNotAllocate(t *testing.T) {
	keep := New(10, desc)
	v := 1000
	allocs := testing.AllocsPerRun(1000, func() {
		v++
		keep.Offer(v)
	})
	if allocs != 0 {
		t.Errorf("Offer allocated %v times per call, want 0", allocs)
	}
}

func TestNewPanicsOnKBelowOne(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Errorf("New(0, ...) did not panic")
		}
	}()
	New(0, desc)
}
