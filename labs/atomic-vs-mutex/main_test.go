package main

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
)

func TestEveryCounterIsExact(t *testing.T) {
	for _, kind := range kinds {
		t.Run(kind, func(t *testing.T) {
			c, err := newCounter(kind, 8)
			if err != nil {
				t.Fatal(err)
			}
			var wg sync.WaitGroup
			for slot := range 8 {
				wg.Go(func() {
					for range 10000 {
						c.Add(slot, 1)
					}
				})
			}
			wg.Wait()
			if got := c.Load(); got != 80000 {
				t.Errorf("Load() = %d, want 80000", got)
			}
		})
	}
}

func TestUnknownCounter(t *testing.T) {
	if _, err := newCounter("magic", 1); !errors.Is(err, errCounter) {
		t.Errorf("err = %v, want errCounter", err)
	}
}

// BenchmarkCounter adds 1 from every goroutine of RunParallel, GOMAXPROCS of them; run it
// with -cpu 1,2 to compare one goroutine with two that share the counter.
func BenchmarkCounter(b *testing.B) {
	for _, kind := range kinds {
		b.Run("kind="+kind, func(b *testing.B) {
			c, err := newCounter(kind, 64)
			if err != nil {
				b.Fatal(err)
			}
			var next atomic.Int64
			b.RunParallel(func(pb *testing.PB) {
				slot := int(next.Add(1) - 1) // each goroutine gets its own slot
				for pb.Next() {
					c.Add(slot, 1)
				}
			})
		})
	}
}
