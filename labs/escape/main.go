// Program escape shows where Go keeps a value: on the goroutine's stack, gone for free
// when the function returns, or on the heap, where only the garbage collector frees it.
// The compiler decides, and says why with -gcflags=-m; testing.AllocsPerRun counts what
// each decision costs at run time.
//
//	go build -o /dev/null -gcflags=-m ./labs/escape
//	go run ./labs/escape
package main

import (
	"fmt"
	"testing"
)

type point struct{ x, y int }

// The functions are never inlined: inlined into the caller, each one would be decided
// again with the caller's code, and the lab shows the decision of the function itself.

// byValue returns a copy: p lives in byValue's frame, and the caller gets its value.
//
//go:noinline
func byValue() point {
	p := point{1, 2}
	return p
}

// byPointer returns the address of p, which must outlive the frame: p moves to the heap.
//
//go:noinline
func byPointer() *point {
	p := point{1, 2}
	return &p
}

// boxed puts p in an interface, which holds a pointer to a copy: the copy goes to the heap.
//
//go:noinline
func boxed(p point) any {
	return p
}

// fixedBuffer makes a buffer of a size the compiler knows and keeps it to itself: stack.
//
//go:noinline
func fixedBuffer() {
	b := make([]byte, 64)
	b[0] = 1
}

// sizedBuffer makes a buffer of a size known only at run time. It does not escape, and
// still: a small one fits in a buffer the compiler reserves on the stack, a large one
// goes to the heap.
//
//go:noinline
func sizedBuffer(n int) {
	b := make([]byte, n)
	b[0] = 1
}

// counter returns a closure that keeps n between calls: n and the closure go to the heap.
//
//go:noinline
func counter() func() int {
	n := 0
	return func() int {
		n++
		return n
	}
}

// localClosure uses a closure only inside the function: nothing outlives the frame.
//
//go:noinline
func localClosure() int {
	n := 0
	inc := func() { n++ }
	inc()
	return n
}

// cases are the calls the lab measures, by name.
var cases = []struct {
	name string
	call func()
}{
	{"byValue", func() { _ = byValue() }},
	{"byPointer", func() { _ = byPointer() }},
	{"boxed", func() { _ = boxed(point{1, 2}) }},
	{"fixedBuffer", fixedBuffer},
	{"sizedBuffer(32)", func() { sizedBuffer(32) }},
	{"sizedBuffer(33)", func() { sizedBuffer(33) }},
	{"counter", func() { _ = counter() }},
	{"localClosure", func() { _ = localClosure() }},
}

func main() {
	fmt.Printf("%-16s %s\n", "call", "allocations")
	for _, c := range cases {
		fmt.Printf("%-16s %.0f\n", c.name, testing.AllocsPerRun(1000, c.call))
	}
}
