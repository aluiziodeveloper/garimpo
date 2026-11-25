package main

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"
)

var texts = []string{"go", "chan", "select", "context", "error", "test", "generic", "wait"}

// slowLen stands in for the model: it takes d and returns the length of the text, unless
// the context ends first.
func slowLen(d time.Duration) Embedder {
	return func(ctx context.Context, text string) (int, error) {
		select {
		case <-time.After(d):
			return len(text), nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
}

func TestEmbedBatchKeepsTheInputOrder(t *testing.T) {
	got, err := embedBatch(t.Context(), texts, 3, slowLen(time.Millisecond))
	if err != nil {
		t.Fatal(err)
	}
	want := []int{2, 4, 6, 7, 5, 4, 7, 4}
	if !slices.Equal(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
}

func TestEmbedBatchRunsAtMostWorkersAtOnce(t *testing.T) {
	// slots has room for 3 calls. A call that finds it full is a fourth one running
	// at the same time, and it records its text in overflow.
	slots := make(chan struct{}, 3)
	overflow := make(chan string, len(texts))
	embed := func(ctx context.Context, text string) (int, error) {
		select {
		case slots <- struct{}{}:
			defer func() { <-slots }()
		default:
			overflow <- text
		}
		return slowLen(5*time.Millisecond)(ctx, text)
	}

	if _, err := embedBatch(t.Context(), texts, 3, embed); err != nil {
		t.Fatal(err)
	}
	if n := len(overflow); n > 0 {
		t.Errorf("%d calls ran while 3 others were running", n)
	}
}

func TestEmbedBatchStopsAtTheFirstError(t *testing.T) {
	errModel := errors.New("model unavailable")
	embed := func(ctx context.Context, text string) (int, error) {
		if text == "select" {
			return 0, errModel
		}
		return slowLen(time.Second)(ctx, text)
	}

	start := time.Now()
	_, err := embedBatch(t.Context(), texts, 3, embed)

	if !errors.Is(err, errModel) {
		t.Fatalf("err = %v, want one that wraps errModel", err)
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Errorf("embedBatch returned after %v; the other calls should stop at once", elapsed)
	}
}

func TestEmbedBatchHonorsTheDeadline(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Millisecond)
	defer cancel()

	_, err := embedBatch(ctx, texts, 2, slowLen(20*time.Millisecond))

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err = %v, want context.DeadlineExceeded", err)
	}
}
