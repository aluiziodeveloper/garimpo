package embeddertest

import (
	"context"
	"crypto/sha256"
	"slices"
	"sync"

	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// FakeDim is the dimension of the Fake's vectors.
const FakeDim = 8

// Fake is an Embedder that needs no model: the vector of a text comes from its SHA-256,
// so equal texts get equal vectors and different texts, almost surely, different ones.
// It records the texts of each call. The zero Fake is ready to use.
type Fake struct {
	mu    sync.Mutex
	calls [][]string
}

// Embed returns one vector per text, or ctx's error if ctx is done.
func (f *Fake) Embed(ctx context.Context, texts []string) ([]shared.Vector, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(texts) == 0 {
		return nil, nil
	}
	f.mu.Lock()
	f.calls = append(f.calls, slices.Clone(texts))
	f.mu.Unlock()
	vs := make([]shared.Vector, len(texts))
	for i, text := range texts {
		sum := sha256.Sum256([]byte(text))
		values := make([]float32, FakeDim)
		for j := range values {
			values[j] = float32(sum[j]) - 127.5 // never zero
		}
		v, err := shared.NewVector(values, FakeDim)
		if err != nil {
			panic(err) // no value is zero, NaN or infinite
		}
		vs[i] = v
	}
	return vs, nil
}

// Calls returns the texts of each call to Embed, in the order they arrived.
func (f *Fake) Calls() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}
