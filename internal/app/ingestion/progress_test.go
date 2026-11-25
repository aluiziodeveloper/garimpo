package ingestion

import (
	"sync"
	"testing"
)

func TestProgressCountsFromManyGoroutines(t *testing.T) {
	var p Progress
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			p.DocumentRead()
			p.ChunksCut(100)
			for range 99 {
				p.ChunkEmbedded()
			}
			p.ChunkFailed()
		})
	}
	wg.Wait()
	want := Counts{Documents: 8, Chunks: 800, Embedded: 792, Failed: 8}
	if got := p.Snapshot(); got != want {
		t.Errorf("Snapshot() = %v, want %v", got, want)
	}
}

func TestZeroProgressIsReady(t *testing.T) {
	var p Progress
	if got := p.Snapshot(); got != (Counts{}) || got.Pending() != 0 {
		t.Errorf("Snapshot() = %v, want zero counts and nothing pending", got)
	}
}

func TestCountsString(t *testing.T) {
	c := Counts{Documents: 2, Chunks: 10, Embedded: 6, Failed: 1}
	want := "documents=2 chunks=10 embedded=6 failed=1 pending=3"
	if got := c.String(); got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
}

// BenchmarkProgress is what one worker does per chunk, from GOMAXPROCS goroutines.
func BenchmarkProgress(b *testing.B) {
	var p Progress
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			p.ChunksCut(1)
			p.ChunkEmbedded()
		}
	})
}
