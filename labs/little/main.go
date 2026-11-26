// Program little measures a server that serves a few calls at a time, as the Ollama on the
// CPU does, against more and more clients in a closed loop: each client calls, waits for
// the answer and calls again. It prints the throughput, the latency and Little's law.
//
//	go run ./labs/little -capacity 2 -service 100ms -clients 1,2,4,8,16
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

// server serves at most cap(slots) calls at a time, each for service; the others queue.
type server struct {
	slots   chan struct{}
	service time.Duration
}

func newServer(capacity int, service time.Duration) *server {
	return &server{slots: make(chan struct{}, capacity), service: service}
}

// call waits for a free slot and then for the service. The time in the queue counts in
// the latency the client sees, as much as the time of the service.
func (s *server) call(ctx context.Context) error {
	select {
	case s.slots <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	defer func() { <-s.slots }()
	t := time.NewTimer(s.service)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// result is what a measure saw: how long it took and the latency of each call.
type result struct {
	elapsed   time.Duration
	latencies []time.Duration // sorted
}

// throughput is λ, the calls answered per second.
func (r result) throughput() float64 {
	return float64(len(r.latencies)) / r.elapsed.Seconds()
}

// percentile returns the latency that a fraction q of the calls did not pass.
func (r result) percentile(q float64) time.Duration {
	return r.latencies[int(float64(len(r.latencies)-1)*q)]
}

// mean is W, the mean time a call spends in the system, queue included.
func (r result) mean() time.Duration {
	var sum time.Duration
	for _, l := range r.latencies {
		sum += l
	}
	return sum / time.Duration(len(r.latencies))
}

// inSystem is L = λ·W, Little's law: how many calls were in the system on average.
func (r result) inSystem() float64 {
	return r.throughput() * r.mean().Seconds()
}

// measure runs clients goroutines that make calls calls each, one after the other, and
// returns the latency of each call. It stops at the first error.
func measure(ctx context.Context, call func(context.Context) error, clients, calls int) result {
	var (
		mu        sync.Mutex
		latencies []time.Duration
		wg        sync.WaitGroup
	)
	start := time.Now()
	for range clients {
		wg.Go(func() {
			for range calls {
				t := time.Now()
				if call(ctx) != nil {
					return
				}
				took := time.Since(t)
				mu.Lock()
				latencies = append(latencies, took)
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	slices.Sort(latencies)
	return result{elapsed: time.Since(start), latencies: latencies}
}

func main() {
	capacity := flag.Int("capacity", 2, "the calls the server serves at a time")
	service := flag.Duration("service", 100*time.Millisecond, "how long the server takes for a call")
	list := flag.String("clients", "1,2,4,8,16", "the numbers of clients to measure, comma separated")
	calls := flag.Int("calls", 20, "the calls each client makes")
	flag.Parse()

	srv := newServer(*capacity, *service)
	fmt.Printf("capacity %d, service %v, %d calls per client\n", *capacity, *service, *calls)
	fmt.Printf("%-8s %-10s %-9s %-9s %-9s %s\n", "clients", "calls/s", "p50", "p99", "W", "L = λ·W")
	for f := range strings.SplitSeq(*list, ",") {
		clients, err := strconv.Atoi(f)
		if err != nil || clients < 1 {
			fmt.Fprintf(os.Stderr, "little: bad number of clients %q\n", f)
			os.Exit(2)
		}
		r := measure(context.Background(), srv.call, clients, *calls)
		fmt.Printf("%-8d %-10.1f %-9v %-9v %-9v %.2f\n", clients, r.throughput(),
			r.percentile(0.50).Round(time.Millisecond), r.percentile(0.99).Round(time.Millisecond),
			r.mean().Round(time.Millisecond), r.inSystem())
	}
}
