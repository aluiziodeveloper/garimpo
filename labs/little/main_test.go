package main

import (
	"testing"
	"testing/synctest"
	"time"
)

const service = 100 * time.Millisecond

// run measures n clients, 10 calls each, against a server with two slots, in a bubble:
// the fake clock makes every latency exact.
func run(t *testing.T, clients int) result {
	t.Helper()
	var r result
	synctest.Test(t, func(t *testing.T) {
		srv := newServer(2, service)
		r = measure(t.Context(), srv.call, clients, 10)
	})
	return r
}

func TestOneClientWaitsOnlyForTheService(t *testing.T) {
	r := run(t, 1)
	if r.throughput() != 10 || r.percentile(0.99) != service {
		t.Fatalf("throughput %v/s, p99 %v; want 10/s and %v", r.throughput(), r.percentile(0.99), service)
	}
}

func TestClientsUpToTheCapacityRaiseTheThroughput(t *testing.T) {
	r := run(t, 2)
	if r.throughput() != 20 || r.percentile(0.99) != service {
		t.Fatalf("throughput %v/s, p99 %v; want 20/s and %v", r.throughput(), r.percentile(0.99), service)
	}
}

// Past the capacity, a client more is a client more in the queue: the throughput stays,
// and the latency grows with the clients.
func TestClientsOverTheCapacityOnlyWait(t *testing.T) {
	r := run(t, 8)
	if r.throughput() != 20 || r.percentile(0.50) != 4*service {
		t.Fatalf("throughput %v/s, p50 %v; want 20/s and %v", r.throughput(), r.percentile(0.50), 4*service)
	}
}

// Little's law: the calls in the system (L) are the throughput (λ) times the mean time
// each one spends in it (W). In a closed loop, L is at most the number of clients.
func TestLittlesLawHolds(t *testing.T) {
	for _, clients := range []int{1, 2, 4, 8, 16} {
		r := run(t, clients)
		if l := r.inSystem(); l > float64(clients) || l < 0.9*float64(clients) {
			t.Errorf("%d clients: L = λ·W = %.2f, want between %.1f and %d", clients, l, 0.9*float64(clients), clients)
		}
	}
}
