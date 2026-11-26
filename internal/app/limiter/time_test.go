package limiter_test

import (
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/aluiziodeveloper/garimpo/internal/app/limiter"
)

// service serves capacity calls at a time, each for took; the others queue, as on the
// Ollama on the CPU (INC-09). It records the most calls it saw at the same time.
type service struct {
	slots chan struct{}
	took  time.Duration

	mu      sync.Mutex
	now     int
	maxSeen int
}

// newService returns a service whose calls take 30 ms, about what all-minilm takes for a
// chunk on 4 CPUs with one caller (INC-09: p50 of 29 ms).
func newService(capacity int) *service {
	return &service{slots: make(chan struct{}, capacity), took: 30 * time.Millisecond}
}

// slowDown makes every call from now on take took.
func (s *service) slowDown(took time.Duration) {
	s.mu.Lock()
	s.took = took
	s.mu.Unlock()
}

func (s *service) call() {
	s.mu.Lock()
	s.now++
	s.maxSeen = max(s.maxSeen, s.now)
	s.mu.Unlock()
	s.slots <- struct{}{}
	s.mu.Lock()
	took := s.took
	s.mu.Unlock()
	time.Sleep(took)
	<-s.slots
	s.mu.Lock()
	s.now--
	s.mu.Unlock()
}

// stats is what a load saw: how long it took, the mean latency of a call and the limits
// the limiter had.
type stats struct {
	elapsed  time.Duration
	mean     time.Duration
	maxLimit int
}

// The load of every test: 16 workers, as many as the ingestion's, making 50 calls each.
const workers, calls = 16, 50

// load runs the workers through l, as the ingestion's workers do; with no l, every worker
// calls at will. It runs inside a bubble: the
// latencies are exact and the test takes no time.
func load(t *testing.T, l *limiter.Limiter, svc *service) stats {
	t.Helper()
	var (
		mu  sync.Mutex
		st  stats
		sum time.Duration
		wg  sync.WaitGroup
	)
	start := time.Now()
	for range workers {
		wg.Go(func() {
			for range calls {
				var p limiter.Permit
				if l != nil {
					var err error
					if p, err = l.Acquire(t.Context()); err != nil {
						t.Error(err)
						return
					}
					mu.Lock()
					st.maxLimit = max(st.maxLimit, l.Limit())
					mu.Unlock()
				}
				began := time.Now()
				svc.call()
				took := time.Since(began)
				if l != nil {
					p.Done()
				}
				mu.Lock()
				sum += took
				mu.Unlock()
			}
		})
	}
	wg.Wait()
	st.elapsed = time.Since(start)
	st.mean = sum / time.Duration(workers*calls)
	return st
}

func newLimiter(t *testing.T, cfg limiter.Config) *limiter.Limiter {
	t.Helper()
	l, err := limiter.New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// Sixteen workers on a service that serves one call at a time, 30 ms each: each call
// waits for the other fifteen.
func TestWithoutALimitEveryCallWaitsInTheQueue(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		svc := newService(1)
		st := load(t, nil, svc)
		t.Logf("%v, mean %v, %d at the service at most", st.elapsed, st.mean, svc.maxSeen)
		if st.mean < 15*svc.took {
			t.Errorf("mean latency %v, want the queue of the other fifteen: at least %v", st.mean, 15*svc.took)
		}
	})
}

// The same load through a limiter: the queue stays short and the service as busy as
// before.
func TestTheLimitStaysNearTheCapacity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := newLimiter(t, limiter.Config{Max: 16})
		svc := newService(1)
		st := load(t, l, svc)

		busy := time.Duration(workers*calls) * svc.took // the service never stopped
		t.Logf("%v for %v of service, mean %v, limit at most %d, %d at the service at most",
			st.elapsed, busy, st.mean, st.maxLimit, svc.maxSeen)
		if st.elapsed != busy {
			t.Errorf("took %v, want %v: the service sat idle", st.elapsed, busy)
		}
		if st.mean > 3*svc.took {
			t.Errorf("mean latency %v, want at most %v", st.mean, 3*svc.took)
		}
		if svc.maxSeen > st.maxLimit {
			t.Errorf("%d calls at the service, over the highest limit %d (RN04)", svc.maxSeen, st.maxLimit)
		}
	})
}

// A service that serves eight calls at a time: the limit climbs past eight, until the
// queue makes the latency pass the tolerance, and the service stays busy.
func TestTheLimitGrowsWithTheCapacity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := newLimiter(t, limiter.Config{Max: 16})
		svc := newService(8)
		st := load(t, l, svc)

		t.Logf("%v, mean %v, limit at most %d, %d at the service at most",
			st.elapsed, st.mean, st.maxLimit, svc.maxSeen)
		if st.maxLimit < 8 {
			t.Errorf("limit reached %d, want at least 8: the service had room", st.maxLimit)
		}
		if ideal := time.Duration(workers*calls/8) * svc.took; st.elapsed > ideal*11/10 {
			t.Errorf("took %v, want at most 10%% over %v", st.elapsed, ideal)
		}
		if st.mean > 3*svc.took {
			t.Errorf("mean latency %v, want at most %v", st.mean, 3*svc.took)
		}
	})
}

// The base is the lowest mean latency ever seen, so a service that gets slower for good
// looks like a queue that never drains: the limit goes down to Min and stays. It is safe
// (fewer calls, never more), and it is the limit of this AIMD (the module's challenge).
func TestASlowerServicePinsTheLimitAtMin(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		l := newLimiter(t, limiter.Config{Max: 16})
		svc := newService(8)
		go func() {
			time.Sleep(time.Second)
			svc.slowDown(120 * time.Millisecond)
		}()
		st := load(t, l, svc)
		t.Logf("%v, mean %v, limit at most %d, at the end %d", st.elapsed, st.mean, st.maxLimit, l.Limit())
		if got := l.Limit(); got != 1 {
			t.Errorf("limit %d at the end, want Min 1", got)
		}
	})
}
