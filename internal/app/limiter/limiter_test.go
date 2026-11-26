package limiter

import (
	"context"
	"errors"
	"testing"
	"time"
)

const ms = time.Millisecond

func newLimiter(t *testing.T, cfg Config) *Limiter {
	t.Helper()
	l, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// fill acquires n permits; none of them may wait.
func fill(t *testing.T, l *Limiter, n int) []Permit {
	t.Helper()
	ps := make([]Permit, n)
	for i := range ps {
		ctx, cancel := context.WithCancel(t.Context())
		cancel() // a permit that has to wait fails at once
		p, err := l.Acquire(ctx)
		if err != nil {
			t.Fatalf("permit %d of %d waited: %v", i+1, n, err)
		}
		ps[i] = p
	}
	return ps
}

func TestNewRejectsABadConfig(t *testing.T) {
	for name, cfg := range map[string]Config{
		"no max":           {},
		"min over max":     {Min: 5, Max: 4},
		"initial over max": {Max: 4, Initial: 5},
		"tolerance of 1":   {Max: 4, Tolerance: 1},
		"backoff of 1":     {Max: 4, Backoff: 1},
		"negative backoff": {Max: 4, Backoff: -0.5},
	} {
		if _, err := New(cfg); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("%s: err = %v, want ErrInvalidConfig", name, err)
		}
	}
}

func TestTheLimitStartsAtInitialOrMin(t *testing.T) {
	if got := newLimiter(t, Config{Max: 8}).Limit(); got != 1 {
		t.Errorf("no Min nor Initial: limit %d, want 1", got)
	}
	if got := newLimiter(t, Config{Min: 2, Max: 8}).Limit(); got != 2 {
		t.Errorf("Min 2: limit %d, want 2", got)
	}
	if got := newLimiter(t, Config{Max: 8, Initial: 4}).Limit(); got != 4 {
		t.Errorf("Initial 4: limit %d, want 4", got)
	}
}

// RN04: a call starts only while the calls in flight are under the limit.
func TestAcquireWaitsAtTheLimit(t *testing.T) {
	l := newLimiter(t, Config{Max: 8, Initial: 2})
	ps := fill(t, l, 2)

	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := l.Acquire(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("third permit: err = %v, want it to wait and see the canceled context", err)
	}
	if got := l.InFlight(); got != 2 {
		t.Fatalf("in flight %d after a canceled wait, want 2", got)
	}

	got := make(chan error)
	go func() {
		p, err := l.Acquire(t.Context())
		if err == nil {
			p.Discard()
		}
		got <- err
	}()
	ps[0].Discard() // a slot frees up: the waiting call starts
	if err := <-got; err != nil {
		t.Fatal(err)
	}
}

// Fast calls with the limit in use raise it by one for each limit's worth of calls: from
// 1 to 2 after 1 call, to 3 after 2 more, to 4 after 3 more.
func TestFastCallsRaiseTheLimit(t *testing.T) {
	l := newLimiter(t, Config{Max: 4})
	for _, want := range []int{2, 2, 3, 3, 3, 4} {
		ps := fill(t, l, l.Limit())
		ps[0].finish(10 * ms)
		for _, p := range ps[1:] {
			p.Discard()
		}
		if got := l.Limit(); got != want {
			t.Fatalf("limit %d, want %d", got, want)
		}
	}
	ps := fill(t, l, 4)
	for _, p := range ps {
		p.finish(10 * ms)
	}
	if got := l.Limit(); got != 4 {
		t.Fatalf("limit %d over Max 4", got)
	}
}

// A call with free slots says nothing about a higher limit: the limit stays.
func TestFastCallsUnderTheLimitKeepIt(t *testing.T) {
	l := newLimiter(t, Config{Max: 8, Initial: 4})
	for range 20 {
		fill(t, l, 1)[0].finish(10 * ms)
	}
	if got := l.Limit(); got != 4 {
		t.Fatalf("limit %d after calls with 3 free slots, want 4", got)
	}
}

// The latency is smoothed (a mean that weighs the last call by 1/8): one slow call does
// not cut the limit; slow calls in a row do, once the mean is over Tolerance times the
// fastest mean seen. 10 ms, then 30 ms: the mean goes 12.5, 14.7, 16.6 ms.
func TestSlowCallsInARowCutTheLimit(t *testing.T) {
	l := newLimiter(t, Config{Max: 16, Initial: 10, Tolerance: 1.5, Backoff: 0.5})
	fill(t, l, 1)[0].finish(10 * ms)
	for i, want := range []int{10, 10, 5} {
		fill(t, l, 1)[0].finish(30 * ms)
		if got := l.Limit(); got != want {
			t.Fatalf("slow call %d: limit %d, want %d", i+1, got, want)
		}
	}
}

// The calls already in flight when the limit is cut were admitted under the old limit:
// their latency is of the old limit, and they do not cut it again.
func TestCallsAdmittedBeforeACutDoNotCutAgain(t *testing.T) {
	l := newLimiter(t, Config{Max: 16, Initial: 8, Tolerance: 1.5, Backoff: 0.5})
	fill(t, l, 1)[0].finish(10 * ms)
	ps := fill(t, l, 8)
	for _, p := range ps {
		p.finish(100 * ms)
	}
	if got := l.Limit(); got != 4 {
		t.Fatalf("limit %d after 8 slow calls admitted together, want one cut: 4", got)
	}
}

func TestTheLimitNeverGoesUnderMin(t *testing.T) {
	l := newLimiter(t, Config{Min: 2, Max: 16, Initial: 3, Tolerance: 1.5, Backoff: 0.5})
	fill(t, l, 1)[0].finish(10 * ms)
	for range 20 {
		fill(t, l, 1)[0].finish(time.Second)
	}
	if got := l.Limit(); got != 2 {
		t.Fatalf("limit %d, want Min 2", got)
	}
}

// A failed or canceled call says nothing about the model's latency: a text over the
// context fails in a few milliseconds, and taking it as a sample would make every other
// call look slow.
func TestDiscardFreesTheSlotWithoutASample(t *testing.T) {
	l := newLimiter(t, Config{Max: 16, Initial: 4, Tolerance: 1.5})
	fill(t, l, 1)[0].finish(100 * ms)
	for range 20 {
		fill(t, l, 1)[0].Discard()
	}
	for range 5 {
		fill(t, l, 1)[0].finish(100 * ms)
	}
	if got, in := l.Limit(), l.InFlight(); got != 4 || in != 0 {
		t.Fatalf("limit %d and %d in flight, want 4 and 0", got, in)
	}
}
