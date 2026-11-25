// Command iterators shows the two shapes of a Go iterator. A push iterator, iter.Seq,
// calls yield for each value; the loop body runs inside it. A pull iterator, from
// iter.Pull, hands out one value per call to next, so two sequences can be walked side
// by side. It also shows the contract of yield: once yield returns false, the iterator
// must stop.
//
//	docker compose run --rm lab go run ./labs/iterators
//	docker compose run --rm lab go run ./labs/iterators -case deaf
package main

import (
	"errors"
	"flag"
	"fmt"
	"iter"
	"os"
	"slices"
)

func main() {
	name := flag.String("case", "push", "what to show: push, merge, cleanup or deaf")
	flag.Parse()
	if err := run(*name); err != nil {
		fmt.Fprintln(os.Stderr, "iterators:", err)
		os.Exit(1)
	}
}

var errCase = errors.New("unknown case")

func run(name string) error {
	switch name {
	case "push":
		for v := range take(evens(numbers(100)), 4) {
			fmt.Println("value", v)
		}
	case "merge":
		a := slices.Values([]int{1, 4, 6, 9})
		b := slices.Values([]int{2, 3, 7})
		fmt.Println(slices.Collect(merge(a, b)))
	case "cleanup":
		var r resource
		for v := range r.values(10) {
			if v == 2 {
				break
			}
			fmt.Println("value", v)
		}
		fmt.Println("closed:", r.closed)
	case "deaf":
		for v := range deaf(10) {
			if v == 2 {
				break
			}
			fmt.Println("value", v)
		}
	default:
		return fmt.Errorf("%w %q: want push, merge, cleanup or deaf", errCase, name)
	}
	return nil
}

// numbers yields 0, 1, ..., n-1.
func numbers(n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := range n {
			if !yield(i) {
				return
			}
		}
	}
}

// evens yields the even values of seq. It pulls nothing ahead: each value goes through
// as soon as seq yields it.
func evens(seq iter.Seq[int]) iter.Seq[int] {
	return func(yield func(int) bool) {
		for v := range seq {
			if v%2 == 0 && !yield(v) {
				return
			}
		}
	}
}

// take yields the first n values of seq and stops it.
func take(seq iter.Seq[int], n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		if n <= 0 {
			return
		}
		i := 0
		for v := range seq {
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

// merge yields the values of two sorted sequences in order. A push iterator cannot do
// this: each sequence would want to run the loop. iter.Pull turns both into next calls.
func merge(a, b iter.Seq[int]) iter.Seq[int] {
	return func(yield func(int) bool) {
		nextA, stopA := iter.Pull(a)
		defer stopA()
		nextB, stopB := iter.Pull(b)
		defer stopB()
		va, okA := nextA()
		vb, okB := nextB()
		for okA || okB {
			if okA && (!okB || va <= vb) {
				if !yield(va) {
					return
				}
				va, okA = nextA()
				continue
			}
			if !yield(vb) {
				return
			}
			vb, okB = nextB()
		}
	}
}

// resource stands for a file or a connection that an iterator opens and must close.
type resource struct {
	closed bool
}

// values yields 0 to n-1 and closes r when it returns, also when the loop stops early.
func (r *resource) values(n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		defer func() { r.closed = true }()
		for i := range n {
			if !yield(i) {
				return
			}
		}
	}
}

// deaf ignores what yield returns: the classic bug. When the loop body breaks, the next
// call to yield panics.
func deaf(n int) iter.Seq[int] {
	return func(yield func(int) bool) {
		for i := range n {
			yield(i)
		}
	}
}
