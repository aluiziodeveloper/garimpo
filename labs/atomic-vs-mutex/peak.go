package main

import "sync/atomic"

// peak keeps the largest value observed by many goroutines, such as the slowest embedding
// of a run. The zero value is ready and holds 0.
type peak struct{ v atomic.Int64 }

// Observe records v if it is larger than the peak so far.
func (p *peak) Observe(v int64) {
	for {
		cur := p.v.Load()
		if v <= cur {
			return
		}
		// Swap only if no one raised the peak since the Load; otherwise read it again.
		if p.v.CompareAndSwap(cur, v) {
			return
		}
	}
}

// Load returns the largest value observed.
func (p *peak) Load() int64 { return p.v.Load() }

// naivePeak is the check-then-act version: every access is atomic, and a larger value can
// still be overwritten by a smaller one between the Load and the Store.
type naivePeak struct{ v atomic.Int64 }

func (p *naivePeak) Observe(v int64) {
	if v > p.v.Load() {
		p.v.Store(v)
	}
}

func (p *naivePeak) Load() int64 { return p.v.Load() }
