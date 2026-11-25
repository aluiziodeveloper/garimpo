package main

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/aluiziodeveloper/garimpo/internal/adapters/corpus"
	ingest "github.com/aluiziodeveloper/garimpo/internal/app/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
)

// BenchmarkCutters runs the whole corpus with 1 and 4 cutters, against an embedder that
// waits 20 µs per chunk on 8 workers (the model is the bottleneck) and one that returns at
// once (the cutting is).
func BenchmarkCutters(b *testing.B) {
	if _, err := os.Stat(library); err != nil {
		b.Skipf("the corpus is not here: %v", err)
	}
	fsys := os.DirFS(library)
	embedders := []struct {
		name  string
		embed ingest.Embed
	}{
		{"slow", func(ctx context.Context, _ ingestion.Chunk) error {
			select {
			case <-time.After(20 * time.Microsecond):
				return nil
			case <-ctx.Done():
				return ctx.Err()
			}
		}},
		{"instant", func(context.Context, ingestion.Chunk) error { return nil }},
	}
	for _, e := range embedders {
		for _, cutters := range []int{1, 4} {
			b.Run(fmt.Sprintf("embedder=%s/cutters=%d", e.name, cutters), func(b *testing.B) {
				for b.Loop() {
					run, err := ingestion.NewRun("bench")
					if err != nil {
						b.Fatal(err)
					}
					cfg := ingest.Config{Cutters: cutters, Workers: 8, Embed: e.embed}
					if err := ingest.RunIngestion(b.Context(), run, corpus.Walk(fsys), cfg); err != nil {
						b.Fatal(err)
					}
				}
			})
		}
	}
}
