package main

import (
	"slices"
	"testing"
)

func TestMaxWorksForAnyOrderedType(t *testing.T) {
	if got, ok := Max(3, 9, 4); !ok || got != 9 {
		t.Errorf("Max(3, 9, 4) = %d, %t; want 9, true", got, ok)
	}
	if got, ok := Max("chan", "select", "go"); !ok || got != "select" {
		t.Errorf(`Max("chan", "select", "go") = %q, %t; want "select", true`, got, ok)
	}
	if _, ok := Max[float64](); ok {
		t.Error("Max() of nothing: ok = true, want false")
	}
}

func TestTopKKeepsTheBestInOrder(t *testing.T) {
	results := []Scored[string]{
		{Item: "a", Score: 0.2},
		{Item: "b", Score: 0.9},
		{Item: "c", Score: 0.5},
		{Item: "d", Score: 0.7},
	}
	original := slices.Clone(results)

	got := TopK(results, 2)

	want := []Scored[string]{{Item: "b", Score: 0.9}, {Item: "d", Score: 0.7}}
	if !slices.Equal(got, want) {
		t.Errorf("TopK(results, 2) = %v, want %v", got, want)
	}
	if !slices.Equal(results, original) {
		t.Errorf("TopK changed its input: %v", results)
	}
	if n := len(TopK(results, 10)); n != len(results) {
		t.Errorf("TopK(results, 10) has %d items, want all %d", n, len(results))
	}
}
