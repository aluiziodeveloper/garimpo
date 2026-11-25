// Command generic-methods builds the same lazy pipeline twice: with generic functions,
// read from the inside out, and with the generic methods of Go 1.27, read from left to
// right. Seq is an iter.Seq with methods; Map is the method that needs a type parameter
// of its own, which a method could not have before Go 1.27.
//
//	docker compose run --rm lab go run ./labs/generic-methods
package main

import (
	"fmt"
	"iter"
	"strconv"
)

func main() {
	even := func(n int) bool { return n%2 == 0 }

	// Before Go 1.27: generic functions, applied from the inside out.
	for s := range Take(Map(Filter(Numbers(10), even), strconv.Itoa), 3) {
		fmt.Println("function", s)
	}

	// Go 1.27: generic methods, applied from left to right.
	for s := range Numbers(10).Filter(even).Map(strconv.Itoa).Take(3) {
		fmt.Println("method", s)
	}
}

// Seq is an iter.Seq with methods. A range loop accepts it as it is.
type Seq[V any] iter.Seq[V]

// All returns s as an iter.Seq, for the functions that take one, such as slices.Collect:
// type inference does not see through the defined type Seq.
func (s Seq[V]) All() iter.Seq[V] {
	return iter.Seq[V](s)
}

// Numbers yields 0, 1, ..., n-1.
func Numbers(n int) Seq[int] {
	return func(yield func(int) bool) {
		for i := range n {
			if !yield(i) {
				return
			}
		}
	}
}

// Filter yields the values of s that keep accepts.
func (s Seq[V]) Filter(keep func(V) bool) Seq[V] {
	return Filter(s, keep)
}

// Map yields f(v) for each value v of s. W is the method's own type parameter: the
// result is a Seq of another type, so Map could only be a function before Go 1.27.
func (s Seq[V]) Map[W any](f func(V) W) Seq[W] {
	return Map(s, f)
}

// Take yields the first n values of s.
func (s Seq[V]) Take(n int) Seq[V] {
	return Take(s, n)
}

// Filter yields the values of s that keep accepts.
func Filter[V any](s Seq[V], keep func(V) bool) Seq[V] {
	return func(yield func(V) bool) {
		for v := range s {
			if keep(v) && !yield(v) {
				return
			}
		}
	}
}

// Map yields f(v) for each value v of s.
func Map[V, W any](s Seq[V], f func(V) W) Seq[W] {
	return func(yield func(W) bool) {
		for v := range s {
			if !yield(f(v)) {
				return
			}
		}
	}
}

// Take yields the first n values of s.
func Take[V any](s Seq[V], n int) Seq[V] {
	return func(yield func(V) bool) {
		if n <= 0 {
			return
		}
		i := 0
		for v := range s {
			if !yield(v) {
				return
			}
			i++
			if i == n {
				return
			}
		}
	}
}
