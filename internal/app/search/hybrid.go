package search

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"

	"github.com/aluiziodeveloper/garimpo/internal/domain/retrieval"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// Embedder turns texts into vectors: here, the query into the vector the vector index is
// searched with. It is ingestion.Embedder's method, declared again where it is consumed, so
// the search does not depend on the ingestion context (ADR 0002) and one adapter of the
// model serves both.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([]shared.Vector, error)
}

// DefaultDepth is how many hits Hybrid asks each index for when Depth is 0.
const DefaultDepth = 50

// Hybrid searches by meaning and by terms at once: the vector index with the vector of the
// query, and the keyword index with its text. It fuses the two rankings with RRF
// (retrieval.Fuse): a chunk both find comes first, and a chunk only one finds still counts.
//
// The keyword search starts at once; the vector search starts when the query has its
// vector. The first error cancels the other search and is the error of Search. A Hybrid is
// safe for concurrent calls when its embedder and its indexes are.
type Hybrid struct {
	Embedder Embedder
	Vectors  VectorIndex
	Keywords KeywordIndex
	// Depth is how many hits each index is asked for, at least k: RRF needs more than the
	// k it returns, to find the chunks both rankings hold. 0 means DefaultDepth.
	Depth int
}

// Search returns the k chunks that best match query, by vector and by keyword. It fails with
// an error that wraps ErrInvalidK when k is below 1, and ctx's error when ctx is done.
func (h *Hybrid) Search(ctx context.Context, query string, k int) ([]retrieval.Hit, error) {
	if k < 1 {
		return nil, fmt.Errorf("%w: %d", ErrInvalidK, k)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	depth := h.Depth
	if depth == 0 {
		depth = DefaultDepth
	}
	depth = max(depth, k)

	g, ctx := errgroup.WithContext(ctx) // the first error cancels this ctx, for the other
	var byVector, byKeyword []retrieval.Hit
	g.Go(func() error {
		vs, err := h.Embedder.Embed(ctx, []string{query})
		if err != nil {
			return fmt.Errorf("embedding the query: %w", err)
		}
		if len(vs) != 1 {
			return fmt.Errorf("embedding the query: %d vectors for one text", len(vs))
		}
		if byVector, err = h.Vectors.Search(ctx, vs[0], depth); err != nil {
			return fmt.Errorf("searching by vector: %w", err)
		}
		return nil
	})
	g.Go(func() error {
		var err error
		if byKeyword, err = h.Keywords.Search(ctx, query, depth); err != nil {
			return fmt.Errorf("searching by keyword: %w", err)
		}
		return nil
	})
	if err := g.Wait(); err != nil { // Wait makes both writes above visible here
		return nil, err
	}
	return retrieval.Fuse(k, byVector, byKeyword), nil
}
