// Command self-assessment is the final exercise of the Base firme module: embed a batch
// of texts with a fixed number of workers, keep the input order, stop at the first error
// and honor the context. The tests in main_test.go are the statement.
//
//	docker compose exec dev go run ./labs/self-assessment
//	docker compose exec dev go run ./labs/self-assessment -fail select
//	docker compose exec dev go test -race ./labs/self-assessment
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"sync"
	"time"
)

// An Embedder turns one text into a result, like a call to the model.
type Embedder func(ctx context.Context, text string) (int, error)

// embedBatch calls embed for every text with at most workers calls at once and returns
// the results in the order of texts. The first error cancels the calls still running
// and is returned; so is the end of ctx.
func embedBatch(ctx context.Context, texts []string, workers int, embed Embedder) ([]int, error) {
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	results := make([]int, len(texts))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for i := range jobs {
				n, err := embed(ctx, texts[i])
				if err != nil {
					cancel(fmt.Errorf("embed %q: %w", texts[i], err)) // only the first cause counts
					continue
				}
				results[i] = n
			}
		})
	}

feed:
	for i := range texts {
		select {
		case jobs <- i:
		case <-ctx.Done():
			break feed
		}
	}
	close(jobs)
	wg.Wait()

	if err := context.Cause(ctx); err != nil {
		return nil, err
	}
	return results, nil
}

func main() {
	workers := flag.Int("workers", 3, "calls at once")
	fail := flag.String("fail", "", "a text whose call fails")
	flag.Parse()

	texts := []string{"go", "chan", "select", "context", "error", "test", "generic", "wait"}
	embed := func(ctx context.Context, text string) (int, error) {
		if text == *fail {
			return 0, errors.New("model unavailable")
		}
		select {
		case <-time.After(20 * time.Millisecond):
			return len(text), nil
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}

	start := time.Now()
	results, err := embedBatch(context.Background(), texts, *workers, embed)
	elapsed := time.Since(start).Round(time.Millisecond)
	if err != nil {
		fmt.Fprintf(os.Stderr, "self-assessment: %v (after %v)\n", err, elapsed)
		os.Exit(1)
	}
	fmt.Printf("%v in %v (workers: %d)\n", results, elapsed, *workers)
}
