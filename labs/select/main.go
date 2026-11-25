// Command select asks three slow replicas the same question, keeps the first answer and
// gives up when a deadline passes.
//
//	docker compose exec dev go run ./labs/select -timeout 100ms
//	docker compose exec dev go run ./labs/select -timeout 10ms
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"time"
)

var errTimeout = errors.New("no replica answered in time")

func main() {
	timeout := flag.Duration("timeout", 100*time.Millisecond, "how long to wait for an answer")
	flag.Parse()

	if err := run(os.Stdout, *timeout); err != nil {
		fmt.Fprintln(os.Stderr, "select:", err)
		os.Exit(1)
	}
}

func run(w io.Writer, timeout time.Duration) error {
	replicas := []func() string{
		sleepy("replica-a", 80*time.Millisecond),
		sleepy("replica-b", 30*time.Millisecond),
		sleepy("replica-c", 50*time.Millisecond),
	}

	start := time.Now()
	answer, firstErr := first(replicas, timeout)
	if firstErr != nil {
		answer = firstErr.Error()
	}
	if _, err := fmt.Fprintf(w, "%v  %s\n", time.Since(start).Round(time.Millisecond), answer); err != nil {
		return err
	}

	time.Sleep(200 * time.Millisecond) // every replica has finished by now
	_, err := fmt.Fprintf(w, "goroutines running: %d (main included)\n", runtime.NumGoroutine())
	return err
}

// first calls every replica in its own goroutine and returns the first answer, or
// errTimeout when none arrives within timeout. The channel has room for every answer, so
// the replicas that lose the race can still send and finish.
func first(replicas []func() string, timeout time.Duration) (string, error) {
	answers := make(chan string, len(replicas))
	for _, ask := range replicas {
		go func() { answers <- ask() }()
	}

	select {
	case a := <-answers:
		return a, nil
	case <-time.After(timeout):
		return "", errTimeout
	}
}

func sleepy(name string, delay time.Duration) func() string {
	return func() string {
		time.Sleep(delay)
		return name
	}
}
