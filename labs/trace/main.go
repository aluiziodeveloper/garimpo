// Command trace runs a small worker pool under the execution tracer and writes the trace to
// a file, for go tool trace. Each job is a region named "hash", so the trace shows it.
//
//	docker compose run --rm lab sh -c 'go build -o /tmp/tr ./labs/trace && /tmp/tr -workers 2 -o /tmp/w2.trace &&
//	    go tool trace -pprof=sched /tmp/w2.trace > /tmp/w2.sched &&
//	    go tool pprof -top /tmp/w2.sched'
package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/trace"
	"sync"
	"time"
)

func main() {
	workers := flag.Int("workers", runtime.GOMAXPROCS(0), "goroutines in the pool")
	jobs := flag.Int("jobs", 400, "CPU-bound jobs (SHA-256 chains)")
	out := flag.String("o", "lab.trace", "where to write the trace")
	latency := flag.Bool("latency", false, "also print the scheduler latency from runtime/metrics")
	flag.Parse()

	before := readSchedLatencies()
	start := time.Now()
	if err := write(*out, *workers, *jobs); err != nil {
		fmt.Fprintln(os.Stderr, "trace:", err)
		os.Exit(1)
	}
	fmt.Printf("workers=%d jobs=%d GOMAXPROCS=%d in %v, trace=%s\n",
		*workers, *jobs, runtime.GOMAXPROCS(0), time.Since(start).Round(time.Millisecond), *out)
	if *latency {
		after := readSchedLatencies()
		for _, q := range []float64{0.5, 0.99} {
			v, err := percentile(before, after, q)
			if err != nil {
				fmt.Fprintln(os.Stderr, "trace:", err)
				os.Exit(1)
			}
			fmt.Printf("scheduler latency p%g: under %v\n", q*100, time.Duration(v*float64(time.Second)))
		}
	}
}

// write creates the trace file and runs the pool, closing the file even when run fails.
func write(path string, workers, jobs int) (err error) {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, f.Close()) }()
	_, err = run(f, workers, jobs)
	return err
}

// run traces a pool of workers goroutines that share jobs hash jobs, writes the trace to w
// and returns how many jobs were done.
func run(w io.Writer, workers, jobs int) (int, error) {
	if workers < 1 {
		return 0, fmt.Errorf("workers = %d, want at least 1", workers)
	}
	if err := trace.Start(w); err != nil {
		return 0, err
	}
	defer trace.Stop()

	ctx, task := trace.NewTask(context.Background(), "pool")
	defer task.End()

	queue := make(chan int)
	var mu sync.Mutex
	done := 0
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for range queue {
				trace.WithRegion(ctx, "hash", hash)
				mu.Lock()
				done++
				mu.Unlock()
			}
		})
	}
	for i := range jobs {
		queue <- i
	}
	close(queue)
	wg.Wait()
	return done, nil
}

// hash is one job: a chain of 20,000 SHA-256 sums, about a millisecond of CPU.
func hash() {
	var h [sha256.Size]byte
	for range 20_000 {
		h = sha256.Sum256(h[:])
	}
}
