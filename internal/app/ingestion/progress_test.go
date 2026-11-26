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

// A snapshot must be a moment the run went through. Each worker cuts one chunk and embeds
// it, so at any moment 0 to workers chunks are pending.
func TestSnapshotIsAMomentOfTheRun(t *testing.T) {
	const workers = 4
	var p Progress
	stop := make(chan struct{})
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				p.ChunksCut(1)
				p.ChunkEmbedded()
			}
		})
	}
	defer func() {
		close(stop)
		wg.Wait()
	}()
	for i := range 100_000 {
		if c := p.Snapshot(); c.Pending() < 0 || c.Pending() > workers {
			t.Fatalf("snapshot %d: %v, want pending from 0 to %d", i, c, workers)
		}
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
	want := "documents=2 chunks=10 embedded=6 failed=1 pending=3 skipped=0"
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
