package indextest

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"

	"github.com/aluiziodeveloper/garimpo/internal/app/search"
	"github.com/aluiziodeveloper/garimpo/internal/domain/retrieval"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// naive is the plainest index that can pass the contract: a map, scanned and sorted on
// every search. It is here to check the suite, not to be used.
type naive struct {
	dim int
	mu  sync.Mutex
	m   map[shared.ChunkID]shared.Vector
}

func (n *naive) Add(id shared.ChunkID, v shared.Vector) error {
	switch {
	case id.IsZero():
		return search.ErrZeroChunk
	case v.Dim() != n.dim:
		return fmt.Errorf("%w: %d, the index has %d", search.ErrWrongDimension, v.Dim(), n.dim)
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if _, ok := n.m[id]; ok {
		return fmt.Errorf("%w: %s", search.ErrDuplicateChunk, id)
	}
	n.m[id] = v
	return nil
}

func (n *naive) Search(ctx context.Context, q shared.Vector, k int) ([]retrieval.Hit, error) {
	switch {
	case k < 1:
		return nil, fmt.Errorf("%w: %d", search.ErrInvalidK, k)
	case q.Dim() != n.dim:
		return nil, fmt.Errorf("%w: %d, the index has %d", search.ErrWrongDimension, q.Dim(), n.dim)
	case ctx.Err() != nil:
		return nil, ctx.Err()
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	hits := make([]retrieval.Hit, 0, len(n.m))
	for id, v := range n.m {
		hits = append(hits, retrieval.Hit{Chunk: id, Score: q.Dot(v)})
	}
	slices.SortFunc(hits, retrieval.Compare)
	return hits[:min(k, len(hits))], nil
}

func (n *naive) Len() int {
	n.mu.Lock()
	defer n.mu.Unlock()
	return len(n.m)
}

func TestNaivePassesTheContract(t *testing.T) {
	Contract(t, func(_ *testing.T, dim int) search.VectorIndex {
		return &naive{dim: dim, m: make(map[shared.ChunkID]shared.Vector)}
	})
}
