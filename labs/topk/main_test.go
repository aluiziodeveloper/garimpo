package main

import (
	"fmt"
	"slices"
	"testing"
)

func TestEverySelectorAgreesWithSort(t *testing.T) {
	for _, order := range []string{"random", "ascending"} {
		scores, err := scoresOf(1000, order, 7)
		if err != nil {
			t.Fatal(err)
		}
		for _, k := range []int{1, 10, 999, 1000, 2000} {
			want := slices.Clone(scores)
			slices.SortFunc(want, func(a, b float32) int { return -cmpFloat(a, b) })
			want = want[:min(k, len(want))]
			for _, sel := range selectors {
				if got := sel.pick(scores, k, new(counter)); !slices.Equal(got, want) {
					t.Errorf("%s, %s order, k=%d: got %d scores, not the %d best in order", sel.name, order, k, len(got), len(want))
				}
			}
		}
	}
}

func cmpFloat(a, b float32) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// In ascending order every score is better than the ones kept: the heap pays log₂(k) for
// each, the sorted slice pays k.
func TestAscendingOrderIsTheWorstCaseOfInsertion(t *testing.T) {
	scores, err := scoresOf(10_000, "ascending", 1)
	if err != nil {
		t.Fatal(err)
	}
	var h, ins counter
	byHeap(scores, 1000, &h)
	byInsertion(scores, 1000, &ins)
	if ins.n < 10*h.n {
		t.Errorf("k=1000, ascending: insertion made %d comparisons and the heap %d, want insertion 10 times more", ins.n, h.n)
	}
}

func BenchmarkSelect(b *testing.B) {
	for _, order := range []string{"random", "ascending"} {
		scores, err := scoresOf(41_980, order, 1)
		if err != nil {
			b.Fatal(err)
		}
		for _, k := range []int{10, 100, 1000} {
			for _, sel := range selectors {
				b.Run(fmt.Sprintf("%s/k=%d/%s", order, k, sel.name), func(b *testing.B) {
					var c counter
					for b.Loop() {
						sel.pick(scores, k, &c)
					}
				})
			}
		}
	}
}
