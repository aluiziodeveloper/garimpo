// Command threads starts many goroutines that wait and counts the OS threads behind them.
// A goroutine waiting on a channel or a timer gives its thread back; a goroutine inside a
// blocking system call keeps it until the call returns (INC-04).
//
//	docker compose run --rm lab go run ./labs/threads -wait chan
//	docker compose run --rm lab go run ./labs/threads -wait syscall
//	docker compose run --rm lab go run ./labs/threads -wait syscall -limit 500 -hold 200ms
package main

import (
	"bufio"
	"bytes"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

func main() {
	n := flag.Int("n", 10000, "goroutines to start")
	wait := flag.String("wait", "chan", "how each goroutine waits: chan, sleep or syscall")
	limit := flag.Int("limit", 0, "at most this many goroutines waiting at once (0: no limit)")
	hold := flag.Duration("hold", 2*time.Second, "how long each goroutine waits")
	flag.Parse()

	s, err := measure(*n, *wait, *limit, *hold)
	if err != nil {
		fmt.Fprintln(os.Stderr, "threads:", err)
		os.Exit(1)
	}
	fmt.Printf("wait=%s limit=%d goroutines=%d threads=%d GOMAXPROCS=%d\n",
		*wait, *limit, s.Goroutines, s.Threads, runtime.GOMAXPROCS(0))
	fmt.Printf("all %d done in %v\n", *n, s.Elapsed.Round(time.Millisecond))
}

// stats is what measure saw while the goroutines were waiting.
type stats struct {
	Goroutines int
	Threads    int
	Elapsed    time.Duration
}

// waiter returns what each goroutine does to wait for hold.
func waiter(kind string, hold time.Duration) (func(), error) {
	switch kind {
	case "chan":
		done := make(chan struct{})
		time.AfterFunc(hold, func() { close(done) })
		return func() { <-done }, nil
	case "sleep":
		return func() { time.Sleep(hold) }, nil
	case "syscall":
		// A blocking system call, standing in for a read from a slow disk or a cgo call:
		// the runtime cannot park it, so the goroutine keeps its thread while it waits.
		ts := syscall.NsecToTimespec(hold.Nanoseconds())
		return func() { _ = syscall.Nanosleep(&ts, nil) }, nil
	}
	return nil, fmt.Errorf("unknown wait %q (want chan, sleep or syscall)", kind)
}

// measure starts n goroutines that wait the given way, at most limit of them at once
// (0 means no limit), and counts goroutines and threads halfway through the first wait.
func measure(n int, kind string, limit int, hold time.Duration) (stats, error) {
	wait, err := waiter(kind, hold)
	if err != nil {
		return stats{}, err
	}
	var slots chan struct{}
	if limit > 0 {
		slots = make(chan struct{}, limit)
	}

	start := time.Now()
	var wg sync.WaitGroup
	for range n {
		wg.Go(func() {
			if slots != nil {
				slots <- struct{}{}
				defer func() { <-slots }()
			}
			wait()
		})
	}

	time.Sleep(hold / 2)
	s := stats{Goroutines: runtime.NumGoroutine()}
	s.Threads, err = threads()
	wg.Wait()
	s.Elapsed = time.Since(start)
	return s, err
}

// threads returns how many OS threads this process has, from /proc/self/status (Linux).
func threads() (int, error) {
	b, err := os.ReadFile("/proc/self/status")
	if err != nil {
		return 0, err
	}
	return parseThreads(bytes.NewReader(b))
}

// parseThreads reads the "Threads:" line of a /proc/<pid>/status file.
func parseThreads(r io.Reader) (int, error) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		if v, ok := strings.CutPrefix(sc.Text(), "Threads:"); ok {
			return strconv.Atoi(strings.TrimSpace(v))
		}
	}
	if err := sc.Err(); err != nil {
		return 0, err
	}
	return 0, errors.New("no Threads line")
}
