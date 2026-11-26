package embeddertest

import (
	"context"
	"crypto/sha256"
	"fmt"
	"slices"
	"sync"

	"github.com/aluiziodeveloper/garimpo/internal/app/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// FakeDim is the dimension of the Fake's vectors.
const FakeDim = 8

// Fake is an Embedder that needs no model: the vector of a text comes from its SHA-256,
// so equal texts get equal vectors and different texts, almost surely, different ones.
// It records the texts of each call. The zero Fake is ready to use.
type Fake struct {
	// Context is the most bytes a text may have, standing for the model's context in
	// tokens; a longer text fails the call with ErrTextTooLong. Zero means no limit.
	Context int

	mu    sync.Mutex
	calls [][]string
}

// Embed returns one vector per text, ctx's error if ctx is done, or ErrTextTooLong.
func (f *Fake) Embed(ctx context.Context, texts []string) ([]shared.Vector, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(texts) == 0 {
		return nil, nil
	}
	for i, text := range texts {
		if f.Context > 0 && len(text) > f.Context {
			return nil, fmt.Errorf("%w: text %d has %d bytes, the context %d", ingestion.ErrTextTooLong, i, len(text), f.Context)
		}
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
