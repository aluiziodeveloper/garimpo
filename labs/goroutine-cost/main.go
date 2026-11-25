// Command goroutine-cost measures what a goroutine costs: the stack memory of a parked
// goroutine here, and the time of more workers than CPUs in the benchmarks of main_test.go.
//
//	docker compose run --rm bench go run ./labs/goroutine-cost
//	docker compose run --rm bench go test -run '^$' -bench . -count 10 ./labs/goroutine-cost
package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
)

func main() {
	n := flag.Int("n", 100_000, "goroutines to park")
	flag.Parse()
	b := stackBytesPerGoroutine(*n)
	fmt.Printf("%d parked goroutines: %d bytes of stack each, %.1f MiB in all\n",
		*n, b, float64(b)*float64(*n)/(1<<20))
}

// pool runs work(0) to work(jobs-1) on workers goroutines. Each worker takes the next job
// number from a shared counter until none is left.
func pool(jobs, workers int, work func(int)) {
	var next atomic.Int64
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for {
				i := int(next.Add(1)) - 1
				if i >= jobs {
					return
				}
				work(i)
			}
		})
	}
	wg.Wait()
}

// hash is one CPU-bound job: a chain of rounds SHA-256 sums.
func hash(rounds int) [sha256.Size]byte {
	var h [sha256.Size]byte
	for range rounds {
		h = sha256.Sum256(h[:])
	}
	return h
}

// stackBytesPerGoroutine parks n goroutines on a channel and returns how much the stack
// memory in use (runtime.MemStats.StackInuse) grew, divided by n.
func stackBytesPerGoroutine(n int) uint64 {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)

	release := make(chan struct{})
	var started, wg sync.WaitGroup
	started.Add(n)
	for range n {
		wg.Go(func() {
			started.Done()
			<-release
		})
	}
	started.Wait()
	runtime.ReadMemStats(&after)
	close(release)
	wg.Wait()

	return (after.StackInuse - before.StackInuse) / uint64(n)
}
