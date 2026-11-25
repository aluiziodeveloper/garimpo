package main

import (
	"fmt"
	"sync/atomic"
	"testing"
)

func TestPoolRunsEveryJobOnce(t *testing.T) {
	for _, workers := range []int{1, 2, 7, 64} {
		var seen [100]atomic.Int32
		pool(len(seen), workers, func(i int) { seen[i].Add(1) })
		for i := range seen {
			if n := seen[i].Load(); n != 1 {
				t.Errorf("workers=%d: job %d ran %d times, want 1", workers, i, n)
			}
		}
	}
}

func TestStackBytesPerGoroutine(t *testing.T) {
	b := stackBytesPerGoroutine(1000)
	if b < 1024 || b > 64*1024 {
		t.Errorf("stack per parked goroutine = %d bytes, want between 1 KiB and 64 KiB", b)
	}
}

// Each job hashes rounds times: 2000 rounds is a job of a few hundred microseconds,
// 1 round is a job far cheaper than starting a goroutine.
func BenchmarkPool(b *testing.B) {
	const jobs = 512
	for _, rounds := range []int{2000, 1} {
		for _, workers := range []int{1, 2, 4, 16, jobs} {
			b.Run(fmt.Sprintf("rounds=%d/workers=%d", rounds, workers), func(b *testing.B) {
				for b.Loop() {
					pool(jobs, workers, func(int) { hash(rounds) })
				}
			})
		}
	}
}
