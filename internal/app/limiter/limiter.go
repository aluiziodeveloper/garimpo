// Package limiter holds the adaptive concurrency limiter: how many calls may go to a
// slow service, such as the embedding model, at the same time. The limit follows the
// latency of the calls (AIMD): it goes up by one while the calls stay fast, and down by a
// factor when they slow down, which is the sign of a queue in front of the service (RN04,
// INC-09).
package limiter

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"time"
)

// ErrInvalidConfig is wrapped when New gets a Config it cannot run with.
var ErrInvalidConfig = errors.New("invalid limiter config")

// The defaults of the Config fields left at zero.
const (
	DefaultTolerance = 1.5
	DefaultBackoff   = 0.9
)

// smoothing is the weight of the last call in the mean latency, as in TCP's smoothed
// round-trip time: one slow call moves the mean by an eighth of the difference.
const smoothing = 1.0 / 8

// Config is what a Limiter runs with.
type Config struct {
	Min     int // the lowest limit; zero means 1
	Max     int // the highest limit; required
	Initial int // the limit at start; zero means Min
	// Tolerance is how many times the fastest mean latency seen the mean may reach before
	// the limit goes down: a slower mean means a queue. Above 1; zero means
	// DefaultTolerance.
	Tolerance float64
	// Backoff multiplies the limit when it goes down. Between 0 and 1; zero means
	// DefaultBackoff.
	Backoff float64
}

// Limiter lets at most Limit calls run at the same time and moves the limit with their
// latency. It is safe for concurrent use.
type Limiter struct {
	cfg Config

	mu       sync.Mutex
	limit    int
	inFlight int
	waiting  []chan struct{} // calls waiting for a slot, first come first served
	fast     int             // fast calls with the limit in use since the last change
	mean     time.Duration   // smoothed latency of the calls
	base     time.Duration   // the lowest mean seen: the latency with no queue
	cuts     int             // how many times the limit went down
}

// New returns a Limiter with the limit at cfg.Initial.
func New(cfg Config) (*Limiter, error) {
	if cfg.Min == 0 {
		cfg.Min = 1
	}
	if cfg.Initial == 0 {
		cfg.Initial = cfg.Min
	}
	if cfg.Tolerance == 0 {
		cfg.Tolerance = DefaultTolerance
	}
	if cfg.Backoff == 0 {
		cfg.Backoff = DefaultBackoff
	}
	if cfg.Min < 1 || cfg.Max < cfg.Min || cfg.Initial < cfg.Min || cfg.Initial > cfg.Max ||
		cfg.Tolerance <= 1 || cfg.Backoff <= 0 || cfg.Backoff >= 1 {
		return nil, fmt.Errorf("%w: min %d, max %d, initial %d, tolerance %g, backoff %g",
			ErrInvalidConfig, cfg.Min, cfg.Max, cfg.Initial, cfg.Tolerance, cfg.Backoff)
	}
	return &Limiter{cfg: cfg, limit: cfg.Initial}, nil
}

// Limit returns the current limit.
func (l *Limiter) Limit() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.limit
}

// InFlight returns how many permits are out.
func (l *Limiter) InFlight() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.inFlight
}

// Acquire returns a permit for one call, waiting while the calls in flight are at the
// limit (RN04). A free slot is taken even if ctx is done; a wait ends with ctx's error.
// The caller must end the permit with Done or Discard.
func (l *Limiter) Acquire(ctx context.Context) (Permit, error) {
	l.mu.Lock()
	if l.inFlight < l.limit && len(l.waiting) == 0 {
		l.inFlight++
		p := l.permitLocked()
		l.mu.Unlock()
		return p, nil
	}
	ready := make(chan struct{})
	l.waiting = append(l.waiting, ready)
	l.mu.Unlock()

	select {
	case <-ready: // the slot was taken for us
		l.mu.Lock()
		p := l.permitLocked()
		l.mu.Unlock()
		return p, nil
	case <-ctx.Done():
		l.mu.Lock()
		defer l.mu.Unlock()
		if i := slices.Index(l.waiting, ready); i >= 0 {
			l.waiting = slices.Delete(l.waiting, i, i+1)
		} else { // the slot was taken for us in the meantime: give it back
			l.inFlight--
			l.admitLocked()
		}
		return Permit{}, ctx.Err()
	}
}

// permitLocked returns a permit that starts now, under the current limit.
func (l *Limiter) permitLocked() Permit {
	return Permit{l: l, cuts: l.cuts, start: time.Now()}
}

// admitLocked gives the free slots to the calls waiting, in order.
func (l *Limiter) admitLocked() {
	for len(l.waiting) > 0 && l.inFlight < l.limit {
		l.inFlight++
		close(l.waiting[0])
		l.waiting = l.waiting[1:]
	}
}

// Permit is the right to make one call. End it once, with Done or Discard.
type Permit struct {
	l     *Limiter
	cuts  int // the limiter's cuts when the call started
	start time.Time
}

// Done ends a call that worked: its latency moves the limit.
func (p Permit) Done() {
	p.finish(time.Since(p.start))
}

// Discard ends a call that failed or was canceled: it frees the slot and leaves the limit
// alone, because the latency of a failure says nothing about the service's queue.
func (p Permit) Discard() {
	l := p.l
	l.mu.Lock()
	defer l.mu.Unlock()
	l.inFlight--
	l.admitLocked()
}

// finish ends a call that took rtt. AIMD: the limit goes up by one after a limit's worth
// of fast calls made while it was in use, and goes down by Backoff when the mean latency
// passes Tolerance times the base.
func (p Permit) finish(rtt time.Duration) {
	l := p.l
	l.mu.Lock()
	defer l.mu.Unlock()
	full := l.inFlight >= l.limit // the limit was in use: this call says if it can grow
	l.inFlight--
	defer l.admitLocked()

	if l.mean == 0 {
		l.mean = rtt
	} else {
		l.mean += time.Duration(float64(rtt-l.mean) * smoothing)
	}
	if l.base == 0 || l.mean < l.base {
		l.base = l.mean
	}
	if p.cuts != l.cuts {
		return // started before the last cut: its latency is of the old limit
	}
	switch {
	case float64(l.mean) > l.cfg.Tolerance*float64(l.base):
		l.limit = max(l.cfg.Min, int(float64(l.limit)*l.cfg.Backoff))
		l.cuts++
		l.fast = 0
	case full:
		l.fast++
		if l.fast >= l.limit {
			l.limit = min(l.cfg.Max, l.limit+1)
			l.fast = 0
		}
	}
}
