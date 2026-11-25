// Command context embeds five texts, 40 ms each, under a deadline. The work checks the
// context, so it stops as soon as the deadline passes.
//
//	docker compose exec dev go run ./labs/context -timeout 1s
//	docker compose exec dev go run ./labs/context -timeout 100ms
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"
)

func main() {
	timeout := flag.Duration("timeout", time.Second, "deadline for the whole batch")
	flag.Parse()

	if err := run(os.Stdout, *timeout); err != nil {
		fmt.Fprintln(os.Stderr, "context:", err)
		os.Exit(1)
	}
}

func run(w io.Writer, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel() // releases the timer if the batch ends before the deadline

	texts := []string{"one", "two", "three", "four", "five"}
	start := time.Now()
	n, err := embedAll(ctx, texts, 40*time.Millisecond)
	elapsed := time.Since(start).Round(time.Millisecond)

	if err != nil {
		_, werr := fmt.Fprintf(w, "embedded %d of %d texts in %v, then: %v\n", n, len(texts), elapsed, err)
		return werr
	}
	_, werr := fmt.Fprintf(w, "embedded %d of %d texts in %v\n", n, len(texts), elapsed)
	return werr
}

// embedAll embeds the texts one by one and returns how many it finished. It stops at
// the first error, which is ctx.Err() when the context ends first.
func embedAll(ctx context.Context, texts []string, perText time.Duration) (int, error) {
	for i := range texts {
		if err := embed(ctx, perText); err != nil {
			return i, err
		}
	}
	return len(texts), nil
}

// embed stands in for a call to the model that takes d. It waits for whichever comes
// first: the work or the end of the context.
func embed(ctx context.Context, d time.Duration) error {
	select {
	case <-time.After(d):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
