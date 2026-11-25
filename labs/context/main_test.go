package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

var texts = []string{"one", "two", "three", "four", "five"}

func TestEmbedAllFinishesWithoutADeadline(t *testing.T) {
	n, err := embedAll(t.Context(), texts, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if n != len(texts) {
		t.Errorf("embedded %d texts, want %d", n, len(texts))
	}
}

func TestEmbedAllStopsAtTheDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 120*time.Millisecond)
	defer cancel()

	start := time.Now()
	n, err := embedAll(ctx, texts, 50*time.Millisecond)
	elapsed := time.Since(start)

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
	if n != 2 {
		t.Errorf("embedded %d texts before the deadline, want 2", n)
	}
	if elapsed > 200*time.Millisecond {
		t.Errorf("embedAll returned after %v; the deadline is 120ms", elapsed)
	}
}

func TestEmbedAllStopsWhenCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancel()

	n, err := embedAll(ctx, texts, 50*time.Millisecond)

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if n != 0 {
		t.Errorf("embedded %d texts after cancel, want 0", n)
	}
}
