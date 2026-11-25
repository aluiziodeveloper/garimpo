// Command race-detector runs small programs that share memory between goroutines, two of
// them with a data race. The race detector finds a race in the code that runs; it does not
// read the code that does not run, and it does not find a logic error made of atomics.
//
//	docker compose run --rm lab go run ./labs/race-detector -case sleep
//	docker compose run --rm lab go run -race ./labs/race-detector -case sleep
//	docker compose run --rm lab go run -race ./labs/race-detector -case flag
package main

import (
	"crypto/sha256"
	"errors"
	"flag"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	name := flag.String("case", "sleep", "what to run: sleep, chan, map, flag or once")
	flag.Parse()

	got, err := run(*name)
	if err != nil {
		fmt.Fprintln(os.Stderr, "race-detector:", err)
		os.Exit(1)
	}
	fmt.Printf("case=%s: %s\n", *name, got)
}

var errCase = errors.New("unknown case")

// run runs one case and returns what it saw.
func run(name string) (string, error) {
	switch name {
	case "sleep":
		return handOffWithSleep(), nil
	case "chan":
		return handOffWithChan(), nil
	case "map":
		return fmt.Sprintf("%d keys", fillMap()), nil
	case "flag":
		return fmt.Sprintf("load ran %d times", loadWithFlag(8)), nil
	case "once":
		return fmt.Sprintf("load ran %d times", loadWithOnce(8)), nil
	default:
		return "", fmt.Errorf("%q: %w", name, errCase)
	}
}

// handOffWithSleep has a data race: the sleep makes the write come first on almost every
// run, but sleeping is not synchronization, and nothing orders the write before the read.
func handOffWithSleep() string {
	var msg string
	go func() { msg = "hello" }()
	time.Sleep(10 * time.Millisecond)
	return msg
}

// handOffWithChan is the same hand-off, with the close of a channel synchronized before the
// receive.
func handOffWithChan() string {
	var msg string
	done := make(chan struct{})
	go func() {
		msg = "hello"
		close(done)
	}()
	<-done
	return msg
}

// fillMap has two goroutines writing to one map, which is not safe for concurrent use.
func fillMap() int {
	m := make(map[int]int)
	var wg sync.WaitGroup
	for g := range 2 {
		wg.Go(func() {
			for i := range 100000 {
				m[2*i+g] = i
			}
		})
	}
	wg.Wait()
	return len(m)
}

// loadWithFlag has no data race, every access is atomic, and is still wrong: two goroutines
// can both see false before either stores true, and both run the load.
func loadWithFlag(workers int) int64 {
	var loaded atomic.Bool
	var runs atomic.Int64
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			if !loaded.Load() {
				runs.Add(1)
				load()
				loaded.Store(true)
			}
		})
	}
	wg.Wait()
	return runs.Load()
}

// loadWithOnce runs the load once, and every Do returns after it finished.
func loadWithOnce(workers int) int64 {
	var once sync.Once
	var runs atomic.Int64
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			once.Do(func() {
				runs.Add(1)
				load()
			})
		})
	}
	wg.Wait()
	return runs.Load()
}

// load stands in for an expensive start, such as loading a model: about a millisecond of
// hashing.
func load() {
	sum := sha256.Sum256(nil)
	for range 20000 {
		sum = sha256.Sum256(sum[:])
	}
}
