package main

import "testing"

// allocs returns the heap allocations of one call, as the average of many.
func allocs(t *testing.T, name string) float64 {
	t.Helper()
	for _, c := range cases {
		if c.name == name {
			return testing.AllocsPerRun(1000, c.call)
		}
	}
	t.Fatalf("no case %q", name)
	return 0
}

// What stays in the frame costs nothing at run time.
func TestWhatStaysOnTheStackAllocatesNothing(t *testing.T) {
	for _, name := range []string{"byValue", "fixedBuffer", "localClosure"} {
		if n := allocs(t, name); n != 0 {
			t.Errorf("%s: %v allocations per call, want 0", name, n)
		}
	}
}

// A value that outlives the frame, by pointer or inside an interface, is one allocation.
func TestWhatOutlivesTheFrameGoesToTheHeap(t *testing.T) {
	for _, name := range []string{"byPointer", "boxed"} {
		if n := allocs(t, name); n != 1 {
			t.Errorf("%s: %v allocations per call, want 1", name, n)
		}
	}
}

// A closure that outlives its function takes the variable it captured with it: the
// variable and the closure are two allocations.
func TestAClosureThatOutlivesItsFunctionTakesItsVariables(t *testing.T) {
	if n := allocs(t, "counter"); n != 2 {
		t.Fatalf("counter: %v allocations per call, want 2", n)
	}
}

// "does not escape" is not "does not allocate": a buffer of a size known only at run time
// fits on the stack up to 32 bytes, and goes to the heap past that.
func TestASizedBufferFitsOnTheStackUpTo32Bytes(t *testing.T) {
	if n := allocs(t, "sizedBuffer(32)"); n != 0 {
		t.Errorf("sizedBuffer(32): %v allocations per call, want 0", n)
	}
	if n := allocs(t, "sizedBuffer(33)"); n != 1 {
		t.Errorf("sizedBuffer(33): %v allocations per call, want 1", n)
	}
}
