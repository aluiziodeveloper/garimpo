// Command pipeline is the lab of module 07: a pipeline of three stages under an errgroup.
// A producer makes the items, several workers do the slow work on them (fan-out), and the
// caller collects the results (fan-in). The first error, or a canceled context, stops
// every stage, and no goroutine is left behind.
//
//	go run ./labs/pipeline -n 64 -workers 4 -kind wait
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"
)

// Result is what a run of the pipeline did.
type Result struct {
	Produced int // items the producer made
	Count    int // items collected
	Sum      int // sum of the collected results
}

// run sends the items 0 to n-1 through work, on workers goroutines, and adds up the
// results. It returns the first error of any stage, or the context's.
func run(ctx context.Context, n, workers int, work func(context.Context, int) (int, error)) (Result, error) {
	g, ctx := errgroup.WithContext(ctx)
	items := make(chan int)
	results := make(chan int)

	var res Result
	g.Go(func() error { // stage 1: produce
		defer close(items)
		for i := range n {
			select {
			case items <- i:
				res.Produced++
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})

	var working sync.WaitGroup
	for range workers { // stage 2: fan-out
		working.Add(1)
		g.Go(func() error {
			defer working.Done()
			for v := range items {
				r, err := work(ctx, v)
				if err != nil {
					return err
				}
				select {
				case results <- r:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			return nil
		})
	}
	go func() { // the last worker to finish closes results
		working.Wait()
		close(results)
	}()

	for r := range results { // stage 3: fan-in
		res.Count++
		res.Sum += r
	}
	err := g.Wait() // every stage has returned: res.Produced is safe to read
	return res, err
}

// burn keeps a CPU busy for about a millisecond and returns a number that depends on the
// work, so the compiler cannot drop it.
func burn() int {
	x := uint64(1)
	for range 1_500_000 {
		x = x*6364136223846793005 + 1442695040888963407
	}
	return int(x >> 63)
}

func main() {
	n := flag.Int("n", 64, "items to send through the pipeline")
	workers := flag.Int("workers", 4, "goroutines in the middle stage")
	kind := flag.String("kind", "wait", "middle stage: wait (sleeps 1 ms) or cpu (busy for about 1 ms)")
	flag.Parse()

	work := func(_ context.Context, v int) (int, error) {
		time.Sleep(time.Millisecond)
		return v, nil
	}
	if *kind == "cpu" {
		work = func(_ context.Context, v int) (int, error) { return v + burn(), nil }
	}
	start := time.Now()
	res, err := run(context.Background(), *n, *workers, work)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pipeline:", err)
		os.Exit(1)
	}
	elapsed := time.Since(start)
	fmt.Printf("kind=%s workers=%d items=%d elapsed=%v items/s=%.0f\n",
		*kind, *workers, res.Count, elapsed.Round(time.Millisecond), float64(res.Count)/elapsed.Seconds())
}
