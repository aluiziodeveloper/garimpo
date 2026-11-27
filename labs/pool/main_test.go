package main

import (
	"testing"
)

// The pool changes where the buffer comes from, never the report: both builders give the
// same checksums, from any number of goroutines, with -race watching.
func TestPooledBuildsTheSameReports(t *testing.T) {
	p, q := plain{}, newPooled()
	for n := range 50 {
		if got, want := q.Build(n), p.Build(n); got != want {
			t.Fatalf("report %d: pooled checksum %08x, plain %08x", n, got, want)
		}
	}
	if a, b := run(plain{}, 4, 200), run(newPooled(), 4, 200); a.sum != b.sum {
		t.Errorf("200 reports by 4 goroutines: plain %08x, pooled %08x", a.sum, b.sum)
	}
}

// A buffer back from the pool still holds the last report: Build must empty it, or a
// report would carry the one before it.
func TestABufferFromThePoolStartsEmpty(t *testing.T) {
	q := newPooled()
	first := q.Build(1)
	if again := q.Build(1); again != first {
		t.Errorf("report 1 built twice: %08x, then %08x", first, again)
	}
}

// The GC empties a pool in two cycles: the first moves what it holds to the victim cache,
// where Get still finds it, and the second drops the victim cache. The race detector drops
// some Puts on purpose, so this test runs without it.
func TestTheGCEmptiesThePoolInTwoCycles(t *testing.T) {
	if raceEnabled {
		t.Skip("sync.Pool drops a Put in four at random under -race")
	}
	if got := survivors(0); got < 90 {
		t.Errorf("no GC: Get found %d of 100 buffers, want about all", got)
	}
	if got := survivors(1); got < 90 {
		t.Errorf("one GC: Get found %d of 100 buffers, want about all, from the victim cache", got)
	}
	if got := survivors(2); got != 0 {
		t.Errorf("two GCs: Get found %d of 100 buffers, want 0", got)
	}
}

// BenchmarkBuild builds one report with each builder.
//
//	go test -run '^$' -bench Build -benchmem ./labs/pool
func BenchmarkBuild(b *testing.B) {
	for _, k := range []struct {
		name string
		b    builder
	}{{"plain", plain{}}, {"pooled", newPooled()}} {
		b.Run("kind="+k.name, func(b *testing.B) {
			n := 0
			for b.Loop() {
				k.b.Build(n)
				n++
			}
		})
	}
}
