package main

import (
	"context"
	"os"
	"slices"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/aluiziodeveloper/garimpo/internal/adapters/corpus"
	"github.com/aluiziodeveloper/garimpo/internal/adapters/ollama"
	ingest "github.com/aluiziodeveloper/garimpo/internal/app/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/app/limiter"
	"github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// timedEmbedder records how long each call to next that worked took.
type timedEmbedder struct {
	next ingest.Embedder
	mu   sync.Mutex
	took []time.Duration
}

func (t *timedEmbedder) Embed(ctx context.Context, texts []string) ([]shared.Vector, error) {
	start := time.Now()
	vs, err := t.next.Embed(ctx, texts)
	if err == nil {
		took := time.Since(start)
		t.mu.Lock()
		t.took = append(t.took, took)
		t.mu.Unlock()
	}
	return vs, err
}

// percentile returns the latency that a fraction q of the calls did not pass, in ms.
func (t *timedEmbedder) percentile(q float64) float64 {
	t.mu.Lock()
	defer t.mu.Unlock()
	slices.Sort(t.took)
	return float64(t.took[int(float64(len(t.took)-1)*q)]) / float64(time.Millisecond)
}

// BenchmarkAdaptive runs the real ingestion of encoding/json (1,038 chunks) on a real
// Ollama with 16 workers, with and without the adaptive limiter (RN04), and with one
// worker as the reference. Besides time, it reports chunks/s, the p50 and p99 latency of
// a call to the model and, with the limiter, its limit at the end. It needs
// GARIMPO_OLLAMA_URL (the dev, lab and bench containers set it).
func BenchmarkAdaptive(b *testing.B) {
	url := os.Getenv("GARIMPO_OLLAMA_URL")
	if url == "" {
		b.Skip("GARIMPO_OLLAMA_URL is not set")
	}
	const dir = "/usr/local/go/src/encoding/json"
	threads, _ := strconv.Atoi(os.Getenv("GARIMPO_OLLAMA_NUM_THREAD"))
	for _, c := range []struct {
		name     string
		workers  int
		adaptive bool
	}{
		{"workers=1", 1, false},
		{"workers=16", 16, false},
		{"workers=16/adaptive", 16, true},
	} {
		b.Run(c.name, func(b *testing.B) {
			e := &timedEmbedder{next: &ollama.Embedder{
				URL: url, Model: "all-minilm:22m", Dim: 384, Context: 512, NumThread: threads,
			}}
			var l *limiter.Limiter
			chunks := 0
			for b.Loop() {
				if c.adaptive {
					var err error
					if l, err = limiter.New(limiter.Config{Max: c.workers}); err != nil {
						b.Fatal(err)
					}
				}
				run, err := ingestion.NewRun("bench")
				if err != nil {
					b.Fatal(err)
				}
				progress := new(ingest.Progress)
				cfg := ingest.Config{Workers: c.workers, Embedder: e, Limiter: l, Progress: progress}
				if err := ingest.RunIngestion(b.Context(), run, corpus.Walk(os.DirFS(dir)), cfg); err != nil {
					b.Fatal(err)
				}
				chunks += int(progress.Snapshot().Embedded)
			}
			b.ReportMetric(float64(chunks)/b.Elapsed().Seconds(), "chunks/s")
			b.ReportMetric(e.percentile(0.50), "p50-ms")
			b.ReportMetric(e.percentile(0.99), "p99-ms")
			if l != nil {
				b.ReportMetric(float64(l.Limit()), "limit")
			}
		})
	}
}
