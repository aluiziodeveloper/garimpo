// Command racy-counter adds to one counter from many goroutines, the way the first progress
// counter of the ingestion did. With a plain int64 the total comes out short and changes on
// every run (INC-05); with atomic.Int64 it is exact.
//
//	docker compose run --rm lab go run ./labs/racy-counter
//	docker compose run --rm lab go run -race ./labs/racy-counter
//	docker compose run --rm lab go run ./labs/racy-counter -counter atomic
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
)

func main() {
	kind := flag.String("counter", "plain", "how the goroutines share the counter: plain or atomic")
	workers := flag.Int("workers", 8, "goroutines adding to the counter")
	n := flag.Int("n", 100000, "how many times each goroutine adds 1")
	flag.Parse()

	got, err := count(*kind, *workers, *n)
	if err != nil {
		fmt.Fprintln(os.Stderr, "racy-counter:", err)
		os.Exit(1)
	}
	want := int64(*workers) * int64(*n)
	fmt.Printf("counter=%s workers=%d GOMAXPROCS=%d: want %d, got %d (%d lost)\n",
		*kind, *workers, runtime.GOMAXPROCS(0), want, got, want-got)
}

var errCounter = errors.New("unknown counter")

// count starts workers goroutines that each add 1 to a shared counter n times, and returns
// the total after all of them finish.
func count(kind string, workers, n int) (int64, error) {
	var add func()
	var total func() int64
	switch kind {
	case "plain":
		// total++ is a read, an add and a write. Two goroutines can read the same value,
		// and one of the two writes is lost.
		var c int64
		add = func() { c++ }
		total = func() int64 { return c }
	case "atomic":
		var c atomic.Int64
		add = func() { c.Add(1) }
		total = c.Load
	default:
		return 0, fmt.Errorf("%q: %w", kind, errCounter)
	}

	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for range n {
				add()
			}
		})
	}
	// Wait is synchronized after every Done: the read below sees all the writes.
	wg.Wait()
	return total(), nil
}
