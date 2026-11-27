package main

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Every scan of every kind sees a whole version, a list of 0 to n-1, while one goroutine
// appends and four scan, with -race watching; and no append is lost.
func TestEveryScanSeesAWholeVersion(t *testing.T) {
	for _, kind := range kinds {
		t.Run(kind, func(t *testing.T) {
			const size = 1000
			l, err := newList(kind, size)
			if err != nil {
				t.Fatal(err)
			}
			r, err := run(l, 4, 10*time.Microsecond, 100*time.Millisecond)
			if err != nil {
				t.Fatal(err)
			}
			if n, _ := l.Scan(); n != size+len(r.appends) {
				t.Errorf("the list has %d values after %d appends to %d, want %d", n, len(r.appends), size, size+len(r.appends))
			}
		})
	}
}

// Why share takes one appender: two appends to the same slice, with room in its array,
// write to the same place. The second overwrites what the first one published.
func TestAppendToAnOldSliceOverwritesTheNewOne(t *testing.T) {
	old := make([]int32, 2, 4)
	first := append(old, 7)  //nolint:gocritic // two appends to old: the trap this test shows
	second := append(old, 8) //nolint:gocritic // the same
	if first[2] != 8 || second[2] != 8 {
		t.Fatalf("first[2] = %d and second[2] = %d, want both 8: they share the array", first[2], second[2])
	}
}

// benchSize is the length of the list in the benchmarks: 4 MB of int32.
const benchSize = 1_000_000

// BenchmarkScan measures one scan of the list while another goroutine appends to it, one
// value every 100 µs.
//
//	go test -run '^$' -bench Scan ./labs/cow
func BenchmarkScan(b *testing.B) {
	for _, kind := range kinds {
		b.Run("kind="+kind, func(b *testing.B) {
			l, err := newList(kind, benchSize)
			if err != nil {
				b.Fatal(err)
			}
			stop := background(func() {
				l.Append()
				time.Sleep(100 * time.Microsecond)
			})
			defer stop()
			for b.Loop() {
				l.Scan()
			}
		})
	}
}

// background calls f in a loop in a new goroutine until the function it returns is called.
func background(f func()) (stop func()) {
	var done atomic.Bool
	var wg sync.WaitGroup
	wg.Go(func() {
		for !done.Load() {
			f()
		}
	})
	return func() {
		done.Store(true)
		wg.Wait()
	}
}
