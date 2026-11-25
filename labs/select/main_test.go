package main

import (
	"errors"
	"runtime"
	"testing"
	"time"
)

// replica answers name after delay, like a slow server.
func replica(name string, delay time.Duration) func() string {
	return func() string {
		time.Sleep(delay)
		return name
	}
}

func TestFirstReturnsTheFastestAnswer(t *testing.T) {
	replicas := []func() string{
		replica("slow", 60*time.Millisecond),
		replica("fast", 5*time.Millisecond),
	}

	got, err := first(replicas, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if got != "fast" {
		t.Errorf("first = %q, want %q", got, "fast")
	}
}

func TestFirstGivesUpAtTheTimeout(t *testing.T) {
	replicas := []func() string{replica("late", 200*time.Millisecond)}

	start := time.Now()
	_, err := first(replicas, 20*time.Millisecond)
	elapsed := time.Since(start)

	if !errors.Is(err, errTimeout) {
		t.Fatalf("err = %v, want errTimeout", err)
	}
	if elapsed > 150*time.Millisecond {
		t.Errorf("first returned after %v; the timeout is 20ms", elapsed)
	}
}

func TestFirstLeavesNoGoroutineBehind(t *testing.T) {
	before := runtime.NumGoroutine()
	replicas := []func() string{
		replica("a", 5*time.Millisecond),
		replica("b", 10*time.Millisecond),
		replica("c", 15*time.Millisecond),
	}

	if _, err := first(replicas, time.Second); err != nil {
		t.Fatal(err)
	}

	// The losers finish their work in a few milliseconds. Then they must be gone.
	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := runtime.NumGoroutine() - before; n > 0 {
		t.Errorf("%d goroutines still running a second later", n)
	}
}
