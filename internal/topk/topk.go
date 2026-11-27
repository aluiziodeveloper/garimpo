// Package topk keeps the k best of a stream of values, with a heap of k: the selection
// every index of Garimpo makes, the exact one and the approximate one.
//
// Like internal/seq and internal/vec, it is a support package outside the layers (ADR
// 0002): no business rules, only the standard library, any layer may import it.
package topk

import (
	"container/heap"
	"fmt"
	"slices"
)

// Keeper keeps the k best values it is offered, in the order of cmp: cmp(a, b) is
// negative when a ranks before b, as for slices.SortFunc.
//
// It holds the k values in a heap with the worst of them at the root. A value better
// than the root takes its place and sinks to where it belongs: log₂(k) comparisons, and
// one comparison with the root for each value that is not better.
type Keeper[T any] struct {
	h worstFirst[T]
	k int
}

// New returns a Keeper of the k best values. It panics if k is below 1.
func New[T any](k int, cmp func(a, b T) int) *Keeper[T] {
	if k < 1 {
		panic(fmt.Sprintf("topk.New: k %d", k))
	}
	return &Keeper[T]{h: worstFirst[T]{s: make([]T, 0, k), cmp: cmp}, k: k}
}

// Offer keeps v if it is among the k best so far, and reports whether it did. A value
// equal to the worst one kept is not better, and is not kept.
//
// It never calls heap.Push or heap.Pop, which take and return an any: a value in an any
// may go to the heap memory, once per call (labs/topk). A new value goes at the end of the
// slice, or over the root, and heap.Fix moves it to where it belongs.
func (kp *Keeper[T]) Offer(v T) bool {
	h := &kp.h
	if len(h.s) < kp.k {
		h.s = append(h.s, v)
		heap.Fix(h, len(h.s)-1)
		return true
	}
	if h.cmp(v, h.s[0]) < 0 {
		h.s[0] = v
		heap.Fix(h, 0)
		return true
	}
	return false
}

// Len returns how many values the Keeper holds, at most k.
func (kp *Keeper[T]) Len() int {
	return len(kp.h.s)
}

// Worst returns the value that ranks last among the ones kept, the one a new value has to
// beat once the Keeper is full, and false if it holds none.
func (kp *Keeper[T]) Worst() (T, bool) {
	if len(kp.h.s) == 0 {
		var zero T
		return zero, false
	}
	return kp.h.s[0], true
}

// Sorted returns the values kept, best first, in a new slice.
func (kp *Keeper[T]) Sorted() []T {
	s := slices.Clone(kp.h.s)
	slices.SortFunc(s, kp.h.cmp)
	return s
}

// worstFirst is a heap.Interface whose root is the value that ranks last.
type worstFirst[T any] struct {
	s   []T
	cmp func(a, b T) int
}

func (h *worstFirst[T]) Len() int           { return len(h.s) }
func (h *worstFirst[T]) Less(i, j int) bool { return h.cmp(h.s[i], h.s[j]) > 0 }
func (h *worstFirst[T]) Swap(i, j int)      { h.s[i], h.s[j] = h.s[j], h.s[i] }

// Push and Pop complete heap.Interface; Keeper never calls heap.Push or heap.Pop.
func (h *worstFirst[T]) Push(x any) { h.s = append(h.s, x.(T)) }

func (h *worstFirst[T]) Pop() any {
	x := h.s[len(h.s)-1]
	h.s = h.s[:len(h.s)-1]
	return x
}
