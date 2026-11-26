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

// Offer keeps v if it is among the k best so far. It never calls heap.Push or heap.Pop,
// which take and return an any: a value in an any may go to the heap memory, once per
// call (labs/topk). The slice fills first, and heap.Init and heap.Fix move values in it.
func (kp *Keeper[T]) Offer(v T) {
	h := &kp.h
	if len(h.s) < kp.k {
		h.s = append(h.s, v)
		if len(h.s) == kp.k {
			heap.Init(h)
		}
		return
	}
	if h.cmp(v, h.s[0]) < 0 {
		h.s[0] = v
		heap.Fix(h, 0)
	}
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
