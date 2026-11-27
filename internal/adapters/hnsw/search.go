package hnsw

import (
	"context"
	"fmt"

	"github.com/aluiziodeveloper/garimpo/internal/app/search"
	"github.com/aluiziodeveloper/garimpo/internal/domain/retrieval"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
	"github.com/aluiziodeveloper/garimpo/internal/topk"
	"github.com/aluiziodeveloper/garimpo/internal/vec"
)

var _ search.VectorIndex = (*Index)(nil)

// Search returns the k vectors closest to q that the graph leads to (algorithm 5 of the
// paper): a greedy walk down to level 1, then a search of max(EfSearch, k) candidates on
// level 0. Each score is the dot product of q and the chunk's vector, as in the exact
// index, and the k best of the candidates come out in the order of retrieval.Compare.
func (x *Index) Search(ctx context.Context, q shared.Vector, k int) ([]retrieval.Hit, error) {
	switch {
	case k < 1:
		return nil, fmt.Errorf("%w: %d", search.ErrInvalidK, k)
	case q.Dim() != x.dim:
		return nil, fmt.Errorf("%w: query of %d, the index has %d", search.ErrWrongDimension, q.Dim(), x.dim)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	query := q.Values()
	x.mu.RLock()
	entry, top := x.entry, x.top
	x.mu.RUnlock()
	if entry < 0 {
		return []retrieval.Hit{}, nil
	}
	ep := []candidate{{entry, vec.Dot(query, x.node(entry).vec)}}
	for l := top; l > 0; l-- {
		ep = x.searchLayer(query, ep, 1, l)
	}
	found := x.searchLayer(query, ep, max(x.cfg.EfSearch, k), 0)
	best := topk.New(min(k, len(found)), retrieval.Compare)
	x.mu.RLock()
	for _, c := range found {
		best.Offer(retrieval.Hit{Chunk: x.ids[c.id], Score: c.score})
	}
	x.mu.RUnlock()
	return best.Sorted(), nil
}
