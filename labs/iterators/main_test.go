package main

import (
	"iter"
	"slices"
	"strings"
	"testing"
)

func TestPushPipeline(t *testing.T) {
	got := slices.Collect(take(evens(numbers(100)), 4))
	if want := []int{0, 2, 4, 6}; !slices.Equal(got, want) {
		t.Errorf("take(evens(numbers(100)), 4) = %v, want %v", got, want)
	}
}

func TestMergeKeepsTheOrder(t *testing.T) {
	a := slices.Values([]int{1, 4, 6, 9})
	b := slices.Values([]int{2, 3, 7})
	got := slices.Collect(merge(a, b))
	if want := []int{1, 2, 3, 4, 6, 7, 9}; !slices.Equal(got, want) {
		t.Errorf("merge() = %v, want %v", got, want)
	}
}

func TestBreakRunsTheCleanup(t *testing.T) {
	var r resource
	for v := range r.values(10) {
		if v == 2 {
			break
		}
	}
	if !r.closed {
		t.Error("the loop stopped early and the resource is still open")
	}
}

func TestPullNeedsStop(t *testing.T) {
	var r resource
	next, stop := iter.Pull(r.values(10))
	next()
	if r.closed {
		t.Fatal("the resource closed before stop")
	}
	stop()
	if !r.closed {
		t.Error("stop returned and the resource is still open")
	}
}

func TestDeafIteratorPanics(t *testing.T) {
	defer func() {
		msg, _ := recover().(error)
		if msg == nil || !strings.Contains(msg.Error(), "continued iteration") {
			t.Errorf("recover() = %v, want the range-over-func panic", msg)
		}
	}()
	for v := range deaf(10) {
		if v == 2 {
			break
		}
	}
	t.Error("the loop ended without a panic")
}

func BenchmarkSum(b *testing.B) {
	const n = 1000
	xs := slices.Collect(numbers(n))
	b.Run("kind=slice", func(b *testing.B) {
		for b.Loop() {
			s := 0
			for _, v := range xs {
				s += v
			}
			sink = s
		}
	})
	b.Run("kind=push", func(b *testing.B) {
		for b.Loop() {
			s := 0
			for v := range numbers(n) {
				s += v
			}
			sink = s
		}
	})
	b.Run("kind=pull", func(b *testing.B) {
		for b.Loop() {
			s := 0
			next, stop := iter.Pull(numbers(n))
			for v, ok := next(); ok; v, ok = next() {
				s += v
			}
			stop()
			sink = s
		}
	})
}

var sink int
