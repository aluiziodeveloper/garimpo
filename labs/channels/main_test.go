package main

import (
	"slices"
	"testing"
)

func TestLinesSendsEveryLineInOrder(t *testing.T) {
	text := "alpha\nbeta\ngamma\n"
	want := []string{"alpha", "beta", "gamma"}

	for _, buf := range []int{0, 2} {
		var got []string
		for line := range lines(text, buf) {
			got = append(got, line)
		}
		if !slices.Equal(got, want) {
			t.Errorf("buffer %d: got %q, want %q", buf, got, want)
		}
	}
}

func TestLinesClosesTheChannel(t *testing.T) {
	ch := lines("only\n", 1)

	if line := <-ch; line != "only" {
		t.Fatalf("first receive = %q, want %q", line, "only")
	}
	if line, ok := <-ch; ok {
		t.Fatalf("second receive = %q, true; want a closed channel", line)
	}
}
