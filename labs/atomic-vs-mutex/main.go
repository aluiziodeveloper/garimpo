// Command atomic-vs-mutex compares five ways to share a counter between goroutines. The
// numbers are in the benchmarks of main_test.go; this command checks that every counter is
// exact and how long each took.
//
//	docker compose run --rm bench go run ./labs/atomic-vs-mutex
//	docker compose run --rm bench go test -run '^$' -bench . -cpu 1,2 -count 10 ./labs/atomic-vs-mutex
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	workers := flag.Int("workers", 2, "goroutines adding to the counter")
	n := flag.Int("n", 10_000_000, "how many times each goroutine adds 1")
	flag.Parse()

	for _, kind := range kinds {
		c, err := newCounter(kind, *workers)
		if err != nil {
			fmt.Fprintln(os.Stderr, "atomic-vs-mutex:", err)
			os.Exit(1)
		}
		start := time.Now()
		var wg sync.WaitGroup
		for slot := range *workers {
			wg.Go(func() {
				for range *n {
					c.Add(slot, 1)
				}
			})
		}
		wg.Wait()
		fmt.Printf("%-8s total=%d in %v\n", kind, c.Load(), time.Since(start).Round(time.Millisecond))
	}
}

// counter is shared by goroutines. Add takes the slot of the calling goroutine, which only
// the sharded counters use.
type counter interface {
	Add(slot int, delta int64)
	Load() int64
}

var (
	kinds      = []string{"atomic", "mutex", "cas", "packed", "padded"}
	errCounter = errors.New("unknown counter")
)

func newCounter(kind string, slots int) (counter, error) {
	switch kind {
	case "atomic":
		return &atomicCounter{}, nil
	case "mutex":
		return &mutexCounter{}, nil
	case "cas":
		return &casCounter{}, nil
	case "packed":
		return &packedCounter{slots: make([]atomic.Int64, slots)}, nil
	case "padded":
		return &paddedCounter{slots: make([]paddedSlot, slots)}, nil
	default:
		return nil, fmt.Errorf("%q: %w", kind, errCounter)
	}
}

// atomicCounter is one atomic.Int64: the add is a single instruction (LOCK XADD on amd64).
type atomicCounter struct{ n atomic.Int64 }

func (c *atomicCounter) Add(_ int, d int64) { c.n.Add(d) }
func (c *atomicCounter) Load() int64        { return c.n.Load() }

// mutexCounter guards a plain int64 with a sync.Mutex.
type mutexCounter struct {
	mu sync.Mutex
	n  int64
}

func (c *mutexCounter) Add(_ int, d int64) {
	c.mu.Lock()
	c.n += d
	c.mu.Unlock()
}

func (c *mutexCounter) Load() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.n
}

// casCounter reads, adds and tries to swap, and starts over when another goroutine changed
// the value in between: the loop a lock-free update needs when Add does not fit.
type casCounter struct{ n atomic.Int64 }

func (c *casCounter) Add(_ int, d int64) {
	for {
		v := c.n.Load()
		if c.n.CompareAndSwap(v, v+d) {
			return
		}
	}
}

func (c *casCounter) Load() int64 { return c.n.Load() }

// packedCounter gives each goroutine its own slot, and Load adds them up. The slots are
// 8 bytes apart, so neighbors share a 64-byte cache line.
type packedCounter struct{ slots []atomic.Int64 }

func (c *packedCounter) Add(slot int, d int64) { c.slots[slot%len(c.slots)].Add(d) }

func (c *packedCounter) Load() int64 {
	var t int64
	for i := range c.slots {
		t += c.slots[i].Load()
	}
	return t
}

// paddedSlot fills a 64-byte cache line, so no two slots share one.
type paddedSlot struct {
	n atomic.Int64
	_ [56]byte
}

// paddedCounter is packedCounter with one cache line per slot.
type paddedCounter struct{ slots []paddedSlot }

func (c *paddedCounter) Add(slot int, d int64) { c.slots[slot%len(c.slots)].n.Add(d) }

func (c *paddedCounter) Load() int64 {
	var t int64
	for i := range c.slots {
		t += c.slots[i].n.Load()
	}
	return t
}
