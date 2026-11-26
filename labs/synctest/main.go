// Program synctest retries a flaky operation with exponential backoff, the kind of code
// that waits on the clock. Its tests run the same waits on a real clock and on the fake
// clock of testing/synctest.
//
//	go run ./labs/synctest -fail 3
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"
)

// retry calls op up to attempts times, waiting base, 2*base, 4*base... between calls. It
// returns nil after the first success, op's last error after the last attempt, or ctx's
// error if ctx ends first, even in the middle of a wait.
func retry(ctx context.Context, attempts int, base time.Duration, op func() error) error {
	wait := base
	for i := 1; ; i++ {
		err := op()
		if err == nil || i == attempts {
			return err
		}
		t := time.NewTimer(wait)
		select {
		case <-t.C:
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		}
		wait *= 2
	}
}

var errFlaky = errors.New("flaky")

// failing returns an operation that fails n times and then succeeds.
func failing(n int) func() error {
	return func() error {
		if n > 0 {
			n--
			return errFlaky
		}
		return nil
	}
}

func main() {
	fail := flag.Int("fail", 3, "how many times the operation fails before it works")
	attempts := flag.Int("attempts", 5, "the most calls")
	base := flag.Duration("base", 250*time.Millisecond, "the first wait")
	flag.Parse()

	start := time.Now()
	op := failing(*fail)
	calls := 0
	err := retry(context.Background(), *attempts, *base, func() error {
		calls++
		err := op()
		fmt.Printf("call %d at %v: %v\n", calls, time.Since(start).Round(time.Millisecond), err)
		return err
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "synctest:", err)
		os.Exit(1)
	}
}
