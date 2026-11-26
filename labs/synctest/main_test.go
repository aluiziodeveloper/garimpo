package main

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

const base = 250 * time.Millisecond

// TestRetryOnTheRealClock waits for real: 250 + 500 + 1000 ms.
func TestRetryOnTheRealClock(t *testing.T) {
	start := time.Now()
	if err := retry(t.Context(), 5, base, failing(3)); err != nil {
		t.Fatal(err)
	}
	if took := time.Since(start); took < 1750*time.Millisecond {
		t.Fatalf("took %v, want at least 1.75s", took)
	}
}

// TestRetryOnTheFakeClock waits the same, on the bubble's clock: the test takes no time,
// and the elapsed time is exact.
func TestRetryOnTheFakeClock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		if err := retry(t.Context(), 5, base, failing(3)); err != nil {
			t.Fatal(err)
		}
		if took := time.Since(start); took != 1750*time.Millisecond {
			t.Fatalf("took %v, want exactly 1.75s", took)
		}
	})
}

func TestRetryGivesUpAfterTheLastAttempt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		start := time.Now()
		err := retry(t.Context(), 3, base, failing(10))
		if !errors.Is(err, errFlaky) {
			t.Fatalf("err = %v, want errFlaky", err)
		}
		if took := time.Since(start); took != 750*time.Millisecond {
			t.Fatalf("took %v, want 750ms: no wait after the last attempt", took)
		}
	})
}

func TestRetryStopsWhenTheContextEnds(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(t.Context(), 600*time.Millisecond)
		defer cancel()
		start := time.Now()
		err := retry(ctx, 5, base, failing(10))
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v, want DeadlineExceeded", err)
		}
		if took := time.Since(start); took != 600*time.Millisecond {
			t.Fatalf("took %v, want 600ms: the wait ends with the context", took)
		}
	})
}

// TestWaitShowsWhereTheGoroutineIs uses synctest.Wait to look at a retry in the middle:
// Wait returns once every other goroutine of the bubble is durably blocked. Wait says
// nothing about memory, so the count is atomic: with a plain int, -race reports a race.
func TestWaitShowsWhereTheGoroutineIs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var calls atomic.Int64
		op := failing(10)
		go func() {
			_ = retry(t.Context(), 3, base, func() error { calls.Add(1); return op() })
		}()
		synctest.Wait() // the goroutine made the first call and waits on its timer
		if n := calls.Load(); n != 1 {
			t.Fatalf("after Wait: %d calls, want 1", n)
		}
		time.Sleep(base)
		synctest.Wait()
		if n := calls.Load(); n != 2 {
			t.Fatalf("after %v: %d calls, want 2", base, n)
		}
	})
}
