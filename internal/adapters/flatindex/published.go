package flatindex

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/aluiziodeveloper/garimpo/internal/app/search"
	"github.com/aluiziodeveloper/garimpo/internal/domain/indexing"
	"github.com/aluiziodeveloper/garimpo/internal/domain/retrieval"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
	"github.com/aluiziodeveloper/garimpo/internal/topk"
)

// Published is an exact search.VectorIndex whose searches take no lock: they read the
// snapshot an Add last published (indexing.IndexSnapshot), and an Add never waits for them
// (INC-14). It is the lightweight CQRS of Garimpo: Add is the command side and publishes
// versions; Search is the query side and only reads them.
//
// Adds take a lock among themselves, to make the next version from the last one and to
// tell a chunk already held; a search never takes it. A search sees every Add that
// returned before it started, and maybe some that were under way.
type Published struct {
	dim int
	cur atomic.Pointer[indexing.IndexSnapshot] // the last version published

	mu  sync.Mutex // the Adds, one at a time
	has map[shared.ChunkID]bool
}

var _ search.VectorIndex = (*Published)(nil)

// NewPublished returns an empty index for vectors of dim dimensions. It panics if dim is
// below 1, like New.
func NewPublished(dim int) *Published {
	empty, err := indexing.Empty(dim)
	if err != nil {
		panic(fmt.Sprintf("flatindex.NewPublished: %v", err))
	}
	x := &Published{dim: dim, has: make(map[shared.ChunkID]bool)}
	x.cur.Store(&empty)
	return x
}

// Add keeps v as the vector of the chunk id, and publishes the version that holds it.
func (x *Published) Add(id shared.ChunkID, v shared.Vector) error {
	switch {
	case id.IsZero():
		return search.ErrZeroChunk
	case v.Dim() != x.dim:
		return fmt.Errorf("%w: vector of %d, the index has %d", search.ErrWrongDimension, v.Dim(), x.dim)
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.has[id] {
		return fmt.Errorf("%w: %s", search.ErrDuplicateChunk, id)
	}
	next, err := x.cur.Load().With(id, v)
	if err != nil {
		return err // With fails only for the zero chunk and the dimension, checked above
	}
	x.has[id] = true
	x.cur.Store(&next)
	return nil
}

// Snapshot returns the last version published: a search of it never changes, however many
// Adds come after. It is for reading only: it shares its arrays with the index, and a With
// on it, while an Add runs, would break the rule of one caller at a time of With.
func (x *Published) Snapshot() indexing.IndexSnapshot {
	return *x.cur.Load()
}

// Search returns the k vectors closest to q in the last version published.
func (x *Published) Search(ctx context.Context, q shared.Vector, k int) ([]retrieval.Hit, error) {
	switch {
	case k < 1:
		return nil, fmt.Errorf("%w: %d", search.ErrInvalidK, k)
	case q.Dim() != x.dim:
		return nil, fmt.Errorf("%w: query of %d, the index has %d", search.ErrWrongDimension, q.Dim(), x.dim)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	snap := x.cur.Load()
	if snap.Len() == 0 {
		return []retrieval.Hit{}, nil
	}
	best := topk.New(min(k, snap.Len()), retrieval.Compare)
	for id, score := range snap.Scores(q) {
		best.Offer(retrieval.Hit{Chunk: id, Score: score})
	}
	return best.Sorted(), nil
}

// Len returns how many vectors the last version published holds.
func (x *Published) Len() int {
	return x.cur.Load().Len()
}
