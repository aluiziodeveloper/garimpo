// Program cow shares a growing list between goroutines that scan it and one goroutine that
// appends to it, the way the searches of Garimpo read the index while the ingestion adds
// to it (module 14), in three ways:
//
//   - rwmutex: one sync.RWMutex; a scan holds it shared, an append holds it alone;
//
//   - copy: an atomic.Pointer to a slice that never changes; an append copies the slice,
//     adds the value to the copy and publishes the copy;
//
//   - share: an atomic.Pointer to a slice; an append writes the value just past the end of
//     the slice published, in the same array while it has room, and publishes a slice one
//     longer. No scan reaches past the length of the slice it loaded, so none reads the
//     value being written; it is safe with one goroutine appending, and only then.
//
// The value appended is always the length of the list before it, so a list of n values
// holds 0 to n-1, and a scan checks that it saw a whole version: a sum of n·(n-1)/2.
//
//	go run ./labs/cow -size 1000000 -scanners 4 -every 100us -for 3s
package main

import (
	"flag"
	"fmt"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"time"
)

// list is a growing list of int32 that goroutines scan while one appends to it.
type list interface {
	// Scan reads one version of the list and returns its length and the sum of its values.
	Scan() (n int, sum int64)
	// Append adds the length of the list to its end. Only one goroutine may call it.
	Append()
}

type rwmutexList struct {
	mu   sync.RWMutex
	vals []int32
}

func (l *rwmutexList) Scan() (int, int64) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.vals), sum(l.vals)
}

func (l *rwmutexList) Append() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.vals = append(l.vals, int32(len(l.vals)))
}

type copyList struct{ cur atomic.Pointer[[]int32] }

func (l *copyList) Scan() (int, int64) {
	vals := *l.cur.Load()
	return len(vals), sum(vals)
}

func (l *copyList) Append() {
	old := *l.cur.Load()
	vals := make([]int32, len(old), len(old)+1)
	copy(vals, old)
	vals = append(vals, int32(len(old)))
	l.cur.Store(&vals)
}

type shareList struct{ cur atomic.Pointer[[]int32] }

func (l *shareList) Scan() (int, int64) {
	vals := *l.cur.Load()
	return len(vals), sum(vals)
}

func (l *shareList) Append() {
	old := *l.cur.Load()
	vals := append(old, int32(len(old))) //nolint:gocritic // in the same array as old while cap(old) > len(old): the point of share
	l.cur.Store(&vals)
}

func sum(vals []int32) int64 {
	var s int64
	for _, v := range vals {
		s += int64(v)
	}
	return s
}

// kinds are the kinds of list, in the order the program prints them.
var kinds = []string{"rwmutex", "copy", "share"}

// newList returns a list of the given kind that already holds 0 to size-1.
func newList(kind string, size int) (list, error) {
	vals := make([]int32, size)
	for i := range vals {
		vals[i] = int32(i)
	}
	switch kind {
	case "rwmutex":
		return &rwmutexList{vals: vals}, nil
	case "copy":
		l := &copyList{}
		l.cur.Store(&vals)
		return l, nil
	case "share":
		l := &shareList{}
		l.cur.Store(&vals)
		return l, nil
	}
	return nil, fmt.Errorf("no list %q", kind)
}

// result is what run measured: the time of each scan and of each append.
type result struct{ scans, appends []time.Duration }

// run scans l from scanners goroutines, as fast as each can, and appends to it from one,
// pausing every between appends, for d. A scan that sees a sum that is not n·(n-1)/2 saw
// part of a version, and run fails.
func run(l list, scanners int, every, d time.Duration) (result, error) {
	var stop atomic.Bool
	var wg sync.WaitGroup
	scans := make([][]time.Duration, scanners)
	var torn atomic.Int64
	for g := range scanners {
		wg.Go(func() {
			for !stop.Load() {
				start := time.Now()
				n, s := l.Scan()
				scans[g] = append(scans[g], time.Since(start))
				if s != int64(n)*int64(n-1)/2 {
					torn.Add(1)
				}
			}
		})
	}
	var appends []time.Duration
	wg.Go(func() {
		for !stop.Load() {
			start := time.Now()
			l.Append()
			appends = append(appends, time.Since(start))
			time.Sleep(every)
		}
	})
	time.Sleep(d)
	stop.Store(true)
	wg.Wait()
	r := result{scans: slices.Concat(scans...), appends: appends}
	if n := torn.Load(); n > 0 {
		return r, fmt.Errorf("%d scans saw part of a version", n)
	}
	return r, nil
}

// pct returns the p-th percentile of d, the largest for 100.
func pct(d []time.Duration, p int) time.Duration {
	if len(d) == 0 {
		return 0
	}
	s := slices.Clone(d)
	slices.Sort(s)
	return s[min(len(s)-1, len(s)*p/100)]
}

func main() {
	size := flag.Int("size", 1_000_000, "how many values the list starts with")
	scanners := flag.Int("scanners", 4, "how many goroutines scan the list")
	every := flag.Duration("every", 100*time.Microsecond, "the pause between two appends")
	d := flag.Duration("for", 3*time.Second, "how long each kind runs")
	flag.Parse()
	fmt.Printf("%d values (%.1f MiB), %d scanners, one append every %v, %v each\n", *size, float64(*size*4)/(1<<20), *scanners, *every, *d)
	fmt.Printf("%-8s %8s %10s %10s %10s | %9s %10s %10s %10s\n",
		"kind", "scans", "p50", "p99", "max", "appends", "p50", "p99", "max")
	for _, kind := range kinds {
		l, err := newList(kind, *size)
		if err != nil {
			fmt.Fprintln(os.Stderr, "cow:", err)
			os.Exit(2)
		}
		r, err := run(l, *scanners, *every, *d)
		if err != nil {
			fmt.Fprintf(os.Stderr, "cow: %s: %v\n", kind, err)
			os.Exit(1)
		}
		round := func(d time.Duration) time.Duration { return d.Round(time.Microsecond) }
		fmt.Printf("%-8s %8d %10v %10v %10v | %9d %10v %10v %10v\n", kind,
			len(r.scans), round(pct(r.scans, 50)), round(pct(r.scans, 99)), round(pct(r.scans, 100)),
			len(r.appends), round(pct(r.appends, 50)), round(pct(r.appends, 99)), round(pct(r.appends, 100)))
	}
}
