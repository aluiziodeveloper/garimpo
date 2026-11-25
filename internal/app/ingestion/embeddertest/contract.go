// Package embeddertest holds what every Embedder is tested with: the contract suite and a
// fake that passes it.
package embeddertest

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/aluiziodeveloper/garimpo/internal/app/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// same is how close the dot product of two vectors of one text must be to 1. A model may
// round a batch differently from a single text, so the contract does not ask for equal
// bits.
const same = 1e-4

// Contract runs the tests every ingestion.Embedder must pass. newEmbedder returns the
// Embedder under test; each subtest asks for its own.
func Contract(t *testing.T, newEmbedder func(t *testing.T) ingestion.Embedder) {
	t.Run("OneVectorPerTextInOrder", func(t *testing.T) {
		vs := embed(t, newEmbedder(t), "func main() {}", "the corpus", "func main() {}")
		if len(vs) != 3 {
			t.Fatalf("Embed returned %d vectors for 3 texts", len(vs))
		}
		if vs[0].Dim() == 0 || vs[1].Dim() != vs[0].Dim() || vs[2].Dim() != vs[0].Dim() {
			t.Fatalf("dimensions %d, %d and %d, want one dimension above 0", vs[0].Dim(), vs[1].Dim(), vs[2].Dim())
		}
		if d := vs[0].Dot(vs[2]); d < 1-same {
			t.Errorf("texts 0 and 2 are equal and their vectors differ: dot %v", d)
		}
		if d := vs[0].Dot(vs[1]); d > 1-same {
			t.Errorf("texts 0 and 1 differ and got the same vector: dot %v", d)
		}
	})

	t.Run("ABatchIsTheSameAsOneByOne", func(t *testing.T) {
		e := newEmbedder(t)
		batch := embed(t, e, "first text", "second text")
		for i, text := range []string{"first text", "second text"} {
			one := embed(t, e, text)
			if d := batch[i].Dot(one[0]); d < 1-same {
				t.Errorf("text %d: the batch and the single call disagree, dot %v", i, d)
			}
		}
	})

	t.Run("NoTextsNoVectors", func(t *testing.T) {
		vs, err := newEmbedder(t).Embed(t.Context(), nil)
		if err != nil || len(vs) != 0 {
			t.Fatalf("Embed(nil) = %d vectors, %v; want none and no error", len(vs), err)
		}
	})

	t.Run("ACanceledContextIsReturned", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		if _, err := newEmbedder(t).Embed(ctx, []string{"text"}); !errors.Is(err, context.Canceled) {
			t.Fatalf("Embed with a canceled context: err = %v, want context.Canceled", err)
		}
	})

	t.Run("SafeForConcurrentCalls", func(t *testing.T) {
		e := newEmbedder(t)
		var wg sync.WaitGroup
		for range 8 {
			wg.Go(func() {
				if vs, err := e.Embed(t.Context(), []string{"a", "b"}); err != nil || len(vs) != 2 {
					t.Errorf("concurrent Embed = %d vectors, %v", len(vs), err)
				}
			})
		}
		wg.Wait()
	})
}

// embed calls e.Embed and stops the test on an error.
func embed(t *testing.T, e ingestion.Embedder, texts ...string) []shared.Vector {
	t.Helper()
	vs, err := e.Embed(t.Context(), texts)
	if err != nil {
		t.Fatalf("Embed(%q): %v", texts, err)
	}
	return vs
}
