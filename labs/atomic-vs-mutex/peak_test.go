package main

import (
	"runtime"
	"sync"
	"testing"
)

// observeAll has 8 goroutines observe 0 to 79,999, each its own share, and returns the peak.
func observeAll(p interface{ Observe(int64) }) {
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			for i := range 10000 {
				p.Observe(int64(i*8 + g))
			}
		})
	}
	wg.Wait()
}

func TestPeakKeepsTheLargest(t *testing.T) {
	for range 50 {
		var p peak
		observeAll(&p)
		if got := p.Load(); got != 79999 {
			t.Fatalf("Load() = %d, want 79999", got)
		}
	}
}

func TestPeakOfNothingIsZero(t *testing.T) {
	var p peak
	if got := p.Load(); got != 0 {
		t.Errorf("Load() = %d, want 0", got)
	}
}

func TestPeakIgnoresSmallerValues(t *testing.T) {
	var p peak
	for _, v := range []int64{3, 9, 4, 9, 1} {
		p.Observe(v)
	}
	if got := p.Load(); got != 9 {
		t.Errorf("Load() = %d, want 9", got)
	}
}

// TestNaivePeakLosesTheLargest shows the check-then-act bug: in 200 runs, the naive peak
// must lose the largest value at least once. With one P the goroutines rarely interleave
// between the Load and the Store, so the test needs two.
func TestNaivePeakLosesTheLargest(t *testing.T) {
	if runtime.GOMAXPROCS(0) < 2 {
		t.Skip("needs GOMAXPROCS of 2 or more")
	}
	lost := 0
	for range 200 {
		var p naivePeak
		observeAll(&p)
		if p.Load() != 79999 {
			lost++
		}
	}
	t.Logf("naive peak lost the largest value in %d of 200 runs", lost)
	if lost == 0 {
		t.Error("the naive peak never lost: the goroutines did not interleave")
	}
}
