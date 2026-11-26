package ollama

import (
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/aluiziodeveloper/garimpo/internal/adapters/corpus"
	"github.com/aluiziodeveloper/garimpo/internal/app/ingestion"
	domain "github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
)

// sample returns the texts of the first n chunks of the corpus under dir.
func sample(b *testing.B, dir string, n int) []string {
	b.Helper()
	var texts []string
	var cutter domain.Cutter
	for d, err := range corpus.Walk(os.DirFS(dir)) {
		if err != nil {
			b.Fatal(err)
		}
		for c := range cutter.Cut(d) {
			texts = append(texts, c.Text())
			if len(texts) == n {
				return texts
			}
		}
	}
	b.Fatalf("%s has only %d chunks", dir, len(texts))
	return nil
}

// BenchmarkBatch embeds 256 chunks of net/http on a real Ollama, as the ingestion does:
// 32 workers, one chunk per call. batch=1 sends each chunk in its own request; larger
// batches put a Batcher in front of the Embedder. Besides time, it reports chunks/s. It
// needs GARIMPO_OLLAMA_URL (the dev, lab and bench containers set it).
func BenchmarkBatch(b *testing.B) {
	url := os.Getenv("GARIMPO_OLLAMA_URL")
	if url == "" {
		b.Skip("GARIMPO_OLLAMA_URL is not set")
	}
	texts := sample(b, "/usr/local/go/src/net/http", 256)
	const workers = 32
	for _, size := range []int{1, 8, 32} {
		b.Run(fmt.Sprintf("batch=%d", size), func(b *testing.B) {
			var e ingestion.Embedder = &Embedder{URL: url, Model: "all-minilm:22m", Dim: 384, Context: 512, NumThread: 4}
			if size > 1 {
				batcher, err := NewBatcher(e, size, 10*time.Millisecond)
				if err != nil {
					b.Fatal(err)
				}
				defer batcher.Close()
				e = batcher
			}
			for b.Loop() {
				next := make(chan string)
				var wg sync.WaitGroup
				for range workers {
					wg.Go(func() {
						for text := range next {
							if _, err := e.Embed(b.Context(), []string{text}); err != nil {
								b.Error(err)
							}
						}
					})
				}
				for _, text := range texts {
					next <- text
				}
				close(next)
				wg.Wait()
			}
			b.ReportMetric(float64(len(texts)*b.N)/b.Elapsed().Seconds(), "chunks/s")
		})
	}
}
