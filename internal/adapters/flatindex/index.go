// Package flatindex is the exact index of Garimpo: it scores the query against every
// vector it keeps. It is the baseline of recall that every approximate index is measured
// against (module 13).
package flatindex

import (
	"context"
	"fmt"
	"sync"

	"github.com/aluiziodeveloper/garimpo/internal/app/search"
	"github.com/aluiziodeveloper/garimpo/internal/domain/retrieval"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
	"github.com/aluiziodeveloper/garimpo/internal/topk"
	"github.com/aluiziodeveloper/garimpo/internal/vec"
)

// Index is an exact search.VectorIndex in memory. The vectors sit one after the other in
// one slice, so a search reads the memory in order, the way the processor fetches it best.
//
// Add and Search are safe for concurrent calls: an Add waits for the searches under way,
// and the searches wait for an Add.
type Index struct {
	dim int

	mu     sync.RWMutex
	ids    []shared.ChunkID
	values []float32 // the vector of ids[i] is values[i*dim : (i+1)*dim]
	has    map[shared.ChunkID]bool
}

var _ search.VectorIndex = (*Index)(nil)

// New returns an empty index for vectors of dim dimensions. It panics if dim is below 1:
// the dimension is the model's, a constant of the program.
func New(dim int) *Index {
	if dim < 1 {
		panic(fmt.Sprintf("flatindex.New: dimension %d", dim))
	}
	return &Index{dim: dim, has: make(map[shared.ChunkID]bool)}
}

// Add keeps v as the vector of the chunk id.
func (x *Index) Add(id shared.ChunkID, v shared.Vector) error {
	switch {
	case id.IsZero():
		return search.ErrZeroChunk
	case v.Dim() != x.dim:
		return fmt.Errorf("%w: vector of %d, the index has %d", search.ErrWrongDimension, v.Dim(), x.dim)
	}
	values := v.Values()
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.has[id] {
		return fmt.Errorf("%w: %s", search.ErrDuplicateChunk, id)
	}
	x.has[id] = true
	x.ids = append(x.ids, id)
	x.values = append(x.values, values...)
	return nil
}

// Search returns the k vectors closest to q. It scores every vector and keeps the k best
// in a heap (topk), without sorting them all.
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
	defer x.mu.RUnlock()
	if len(x.ids) == 0 {
		return []retrieval.Hit{}, nil
	}
	// A heap of k, but never larger than the index: k comes from the caller.
	best := topk.New(min(k, len(x.ids)), retrieval.Compare)
	for i, id := range x.ids {
		if i%search.ScanCheck == search.ScanCheck-1 {
			if err := ctx.Err(); err != nil {
				return nil, err // the caller gave up: the rest of the scan would be for no one
			}
		}
		best.Offer(retrieval.Hit{Chunk: id, Score: vec.Dot(query, x.values[i*x.dim:(i+1)*x.dim])})
	}
	return best.Sorted(), nil
}

// Len returns how many vectors the index keeps.
func (x *Index) Len() int {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return len(x.ids)
}
