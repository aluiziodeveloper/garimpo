package main

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aluiziodeveloper/garimpo/internal/adapters/corpus"
	ingest "github.com/aluiziodeveloper/garimpo/internal/app/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// BenchmarkRunIngestion runs the ingestion over the whole corpus with an embedder that
// waits 20 µs per chunk on 8 workers, slower than the cutter, for four limits of pending
// chunks (RN03). 65536 is more than the corpus has: no limit at all. Besides time, it
// reports peak-live-B, the largest heap a collection found alive, and peak-pending, the
// most chunks in flight an embed call saw.
func BenchmarkRunIngestion(b *testing.B) {
	if _, err := os.Stat(library); err != nil {
		b.Skipf("the corpus is not here: %v", err)
	}
	fsys := os.DirFS(library)
	for _, limit := range []int{64, 256, 4096, 65536} {
		b.Run(fmt.Sprintf("max-pending=%d", limit), func(b *testing.B) {
			var peak atomic.Int64
			var progress *ingest.Progress
			embedder := &slowEmbedder{wait: 20 * time.Microsecond, seen: func() {
				for p := progress.Snapshot().Pending(); ; { // keep the largest, without a lock (05-05)
					old := peak.Load()
					if p <= old || peak.CompareAndSwap(old, p) {
						break
					}
				}
			}}
			b.ReportAllocs()
			runtime.GC()
			stop := watchLiveHeap()
			for b.Loop() {
				run, err := ingestion.NewRun("bench")
				if err != nil {
					b.Fatal(err)
				}
				progress = new(ingest.Progress)
				cfg := ingest.Config{Workers: 8, MaxPending: limit, Embedder: embedder, Progress: progress}
				if err := ingest.RunIngestion(b.Context(), run, corpus.Walk(fsys), cfg); err != nil {
					b.Fatal(err)
				}
			}
			b.ReportMetric(float64(stop()), "peak-live-B")
			b.ReportMetric(float64(peak.Load()), "peak-pending")
		})
	}
}

// slowEmbedder waits before returning one copy of the same vector per text, and calls
// seen first: a model with a fixed cost and no CPU of its own.
type slowEmbedder struct {
	wait time.Duration
	seen func()
}

var benchVector, _ = shared.NewVector([]float32{1}, 1)

func (e *slowEmbedder) Embed(ctx context.Context, texts []string) ([]shared.Vector, error) {
	e.seen()
	select {
	case <-time.After(e.wait):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	vs := make([]shared.Vector, len(texts))
	for i := range vs {
		vs[i] = benchVector
	}
	return vs, nil
}
