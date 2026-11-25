// Command generics-basics uses one generic function with a constraint (Max) and one
// generic type with a function over it (Scored and TopK).
//
//	docker compose exec dev go run ./labs/generics-basics
package main

import (
	"cmp"
	"fmt"
	"slices"
)

// Max returns the largest of xs, and false when xs is empty. T can be any type that
// supports <, which is what the constraint cmp.Ordered allows.
func Max[T cmp.Ordered](xs ...T) (T, bool) {
	var best T
	if len(xs) == 0 {
		return best, false
	}
	best = xs[0]
	for _, x := range xs[1:] {
		if x > best {
			best = x
		}
	}
	return best, true
}

// Scored is any item with a score, such as a search result.
type Scored[T any] struct {
	Item  T
	Score float64
}

// TopK returns the k items with the highest scores, best first, without changing
// items. Items with the same score keep their order.
func TopK[T any](items []Scored[T], k int) []Scored[T] {
	sorted := slices.Clone(items)
	slices.SortStableFunc(sorted, func(a, b Scored[T]) int {
		return cmp.Compare(b.Score, a.Score) // descending
	})
	return sorted[:min(k, len(sorted))]
}

func main() {
	n, _ := Max(3, 9, 4)
	s, _ := Max("chan", "select", "go")
	fmt.Println("Max ints:", n)
	fmt.Println("Max strings:", s)

	results := []Scored[string]{
		{Item: "sync/waitgroup.go", Score: 0.62},
		{Item: "context/context.go", Score: 0.91},
		{Item: "errors/wrap.go", Score: 0.48},
		{Item: "time/sleep.go", Score: 0.77},
	}
	for _, r := range TopK(results, 2) {
		fmt.Printf("%.2f  %s\n", r.Score, r.Item)
	}
}
