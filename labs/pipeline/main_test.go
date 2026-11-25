package main

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"testing"
	"time"
)

func double(_ context.Context, v int) (int, error) { return 2 * v, nil }

func TestEveryItemIsWorkedOnce(t *testing.T) {
	for _, workers := range []int{1, 4} {
		res, err := run(t.Context(), 100, workers, double)
		if err != nil {
			t.Fatal(err)
		}
		if res.Count != 100 || res.Sum != 9900 {
			t.Errorf("workers=%d: %+v, want 100 items summing 9900", workers, res)
		}
	}
}

var errBroken = errors.New("broken item")

func TestTheFirstErrorStopsThePipeline(t *testing.T) {
	failAt10 := func(_ context.Context, v int) (int, error) {
		if v == 10 {
			return 0, fmt.Errorf("item %d: %w", v, errBroken)
		}
		return v, nil
	}
	res, err := run(t.Context(), 1_000_000, 4, failAt10)
	if !errors.Is(err, errBroken) {
		t.Fatalf("err = %v, want errBroken", err)
	}
	if res.Produced >= 1_000_000 {
		t.Fatalf("the producer made all %d items after the error", res.Produced)
	}
}

func TestCancelingTheContextStopsThePipeline(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	cancelAt5 := func(_ context.Context, v int) (int, error) {
		if v == 5 {
			cancel()
		}
		return v, nil
	}
	res, err := run(ctx, 1_000_000, 4, cancelAt5)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if res.Produced >= 1_000_000 {
		t.Fatalf("the producer made all %d items after the cancel", res.Produced)
	}
}

// settled returns the number of goroutines once it falls to want, or after a second. A
// goroutine that has closed its channel may still be returning, and counted, for a moment;
// one that leaked never returns.
func settled(want int) int {
	n := runtime.NumGoroutine()
	for deadline := time.Now().Add(time.Second); n > want && time.Now().Before(deadline); {
		time.Sleep(time.Millisecond)
		n = runtime.NumGoroutine()
	}
	return n
}

func TestNoGoroutineIsLeftBehind(t *testing.T) {
	before := runtime.NumGoroutine()
	failAt10 := func(_ context.Context, v int) (int, error) {
		if v == 10 {
			return 0, errBroken
		}
		return v, nil
	}
	for range 10 {
		if _, err := run(t.Context(), 1000, 4, failAt10); !errors.Is(err, errBroken) {
			t.Fatal(err)
		}
	}
	if after := settled(before); after != before {
		t.Fatalf("%d goroutines before, %d after ten failed runs", before, after)
	}
}

// BenchmarkPipeline runs 64 items through the pipeline, with a stage that waits 1 ms per
// item, as a call to a model would, or one that keeps a CPU busy for about as long.
func BenchmarkPipeline(b *testing.B) {
	stages := []struct {
		kind string
		work func(context.Context, int) (int, error)
	}{
		{"wait", func(_ context.Context, v int) (int, error) { time.Sleep(time.Millisecond); return v, nil }},
		{"cpu", func(_ context.Context, v int) (int, error) { return v + burn(), nil }},
	}
	for _, s := range stages {
		for _, workers := range []int{1, 2, 4, 8} {
			b.Run(fmt.Sprintf("kind=%s/workers=%d", s.kind, workers), func(b *testing.B) {
				for b.Loop() {
					if _, err := run(b.Context(), 64, workers, s.work); err != nil {
						b.Fatal(err)
					}
				}
				b.ReportMetric(64*float64(b.N)/b.Elapsed().Seconds(), "items/s")
			})
		}
	}
}
