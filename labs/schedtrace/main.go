// Command schedtrace keeps a few goroutines busy so the scheduler trace has something to
// show. Build it first and set GODEBUG only on the binary: with go run, the go command
// and the compiler print their own traces too.
//
//	docker compose run --rm lab sh -c 'go build -o /tmp/schedtrace ./labs/schedtrace &&
//	    GODEBUG=schedtrace=500 /tmp/schedtrace -workers 8 -kind cpu'
package main

import (
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

func main() {
	workers := flag.Int("workers", 8, "goroutines to start")
	kind := flag.String("kind", "cpu", "what each goroutine does: cpu (hash) or wait (sleep)")
	length := flag.Duration("for", 2*time.Second, "how long the work lasts")
	flag.Parse()

	n, err := run(*workers, *kind, *length)
	if err != nil {
		fmt.Fprintln(os.Stderr, "schedtrace:", err)
		os.Exit(1)
	}
	fmt.Printf("workers=%d kind=%s GOMAXPROCS=%d hashes=%d\n", *workers, *kind, runtime.GOMAXPROCS(0), n)
}

// run starts workers goroutines that hash (cpu) or sleep (wait) for length, and returns
// how many hashes they computed together.
func run(workers int, kind string, length time.Duration) (int64, error) {
	if kind != "cpu" && kind != "wait" {
		return 0, fmt.Errorf("unknown kind %q (want cpu or wait)", kind)
	}
	deadline := time.Now().Add(length)
	var total atomic.Int64
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			if kind == "wait" {
				for time.Now().Before(deadline) {
					time.Sleep(10 * time.Millisecond)
				}
				return
			}
			var h [sha256.Size]byte
			var n int64
			for time.Now().Before(deadline) {
				for range 1000 {
					h = sha256.Sum256(h[:])
				}
				n += 1000
			}
			total.Add(n)
		})
	}
	wg.Wait()
	return total.Load(), nil
}
