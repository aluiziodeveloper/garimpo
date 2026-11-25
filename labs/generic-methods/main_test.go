package main

import (
	"slices"
	"strconv"
	"testing"
)

func even(n int) bool { return n%2 == 0 }

func TestChainReadsLeftToRight(t *testing.T) {
	got := slices.Collect(Numbers(10).Filter(even).Map(strconv.Itoa).Take(3).All())
	if want := []string{"0", "2", "4"}; !slices.Equal(got, want) {
		t.Errorf("chain = %q, want %q", got, want)
	}
}

func TestMethodAndFunctionAgree(t *testing.T) {
	method := slices.Collect(Numbers(10).Filter(even).Map(strconv.Itoa).All())
	function := slices.Collect(Map(Filter(Numbers(10), even), strconv.Itoa).All())
	if !slices.Equal(method, function) {
		t.Errorf("method chain = %q, function calls = %q", method, function)
	}
}

func TestTheChainIsLazy(t *testing.T) {
	calls := 0
	square := func(n int) int { calls++; return n * n }
	for range Numbers(1_000_000).Map(square).Take(2) {
	}
	if calls != 2 {
		t.Errorf("Map called f %d times for 2 values, want 2", calls)
	}
}

func BenchmarkChain(b *testing.B) {
	b.Run("form=method", func(b *testing.B) {
		for b.Loop() {
			s := 0
			for v := range Numbers(1000).Filter(even).Map(double) {
				s += v
			}
			sink = s
		}
	})
	b.Run("form=function", func(b *testing.B) {
		for b.Loop() {
			s := 0
			for v := range Map(Filter(Numbers(1000), even), double) {
				s += v
			}
			sink = s
		}
	})
}

func double(n int) int { return 2 * n }

var sink int
