// Command preemption runs a loop that never calls a function beside a goroutine that only
// wants to sleep 10 ms, on one P. Since Go 1.14 the runtime interrupts the loop with a
// signal; with GODEBUG=asyncpreemptoff=1 it cannot, and the sleeper waits for the whole loop.
//
//	docker compose run --rm lab sh -c 'go build -o /tmp/preemption ./labs/preemption &&
//	    /tmp/preemption && GODEBUG=asyncpreemptoff=1 /tmp/preemption'
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"time"
)

func main() {
	loop := flag.String("loop", "spin", "the busy loop: spin (no calls), leaf (calls a NOSPLIT function) or call")
	n := flag.Int64("n", 2_000_000_000, "at most this many steps")
	sleep := flag.Duration("sleep", 10*time.Millisecond, "how long the other goroutine sleeps")
	flag.Parse()

	r, err := run(*loop, *n, *sleep)
	if err != nil {
		fmt.Fprintln(os.Stderr, "preemption:", err)
		os.Exit(1)
	}
	fmt.Printf("loop=%s GODEBUG=%q\n", *loop, os.Getenv("GODEBUG"))
	fmt.Printf("asked to sleep %v, woke after %v; the loop ran %v\n",
		*sleep, r.Woke.Round(time.Millisecond), r.Loop.Round(time.Millisecond))
}

// result is what run measured, both from the moment the loop was started.
type result struct {
	Woke time.Duration // when the sleeper ran again
	Loop time.Duration // when the loop ended
}

// sink keeps the compiler from dropping the loop's work.
var sink atomic.Int64

// leaf is the body of the leaf loop. noinline keeps the call, but a small function that
// calls nothing is marked NOSPLIT: the compiler leaves out the stack check at its start,
// and with it the point where the scheduler could stop the loop.
//
//go:noinline
func leaf(x, i int64) int64 { return x + (i ^ (i >> 3)) }

// step is the body of the call loop. It calls leaf, so it is not a leaf itself and starts
// with a stack check: a point where the runtime can preempt without a signal.
//
//go:noinline
func step(x, i int64) int64 { return leaf(x, i) }

// run starts the busy loop and, on the same P, sleeps for sleep. When the sleeper runs
// again, it tells the loop to stop; the loop also stops after n steps.
func run(loop string, n int64, sleep time.Duration) (result, error) {
	if loop != "spin" && loop != "leaf" && loop != "call" {
		return result{}, fmt.Errorf("unknown loop %q (want spin, leaf or call)", loop)
	}
	defer runtime.GOMAXPROCS(runtime.GOMAXPROCS(1))

	var stop atomic.Bool
	ended := make(chan time.Duration)
	start := time.Now()
	go func() {
		var x int64
		switch loop {
		case "spin":
			for i := int64(0); i < n && !stop.Load(); i++ {
				x += i ^ (i >> 3)
			}
		case "leaf":
			for i := int64(0); i < n && !stop.Load(); i++ {
				x = leaf(x, i)
			}
		case "call":
			for i := int64(0); i < n && !stop.Load(); i++ {
				x = step(x, i)
			}
		}
		sink.Store(x)
		ended <- time.Since(start)
	}()

	time.Sleep(sleep)
	r := result{Woke: time.Since(start)}
	stop.Store(true)
	r.Loop = <-ended
	return r, nil
}
