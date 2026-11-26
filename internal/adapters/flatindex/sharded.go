package flatindex

import (
	"context"
	"fmt"
	"hash/maphash"
	"sync"

	"github.com/aluiziodeveloper/garimpo/internal/app/search"
	"github.com/aluiziodeveloper/garimpo/internal/domain/retrieval"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
	"github.com/aluiziodeveloper/garimpo/internal/topk"
)

// Sharded is an exact search.VectorIndex split into shards, each an Index of its own. A
// search runs on every shard at once, one goroutine each (scatter), and merges the k best
// of each shard into the k best of all (gather). The k best of all are among the k best of
// some shard, so the merge loses nothing: the index stays exact.
//
// A chunk goes to the shard a hash of its ID picks, so the same chunk always goes to the
// same shard, and that shard refuses it the second time.
type Sharded struct {
	dim    int
	seed   maphash.Seed
	shards []*Index
}

var _ search.VectorIndex = (*Sharded)(nil)

// NewSharded returns an empty index of n shards for vectors of dim dimensions. It panics
// if dim or n is below 1.
func NewSharded(dim, n int) *Sharded {
	if n < 1 {
		panic(fmt.Sprintf("flatindex.NewSharded: %d shards", n))
	}
	x := &Sharded{dim: dim, seed: maphash.MakeSeed(), shards: make([]*Index, n)}
	for i := range x.shards {
		x.shards[i] = New(dim)
	}
	return x
}

// Add keeps v as the vector of the chunk id, in the shard of id.
func (x *Sharded) Add(id shared.ChunkID, v shared.Vector) error {
	if id.IsZero() {
		return search.ErrZeroChunk
	}
	shard := maphash.Comparable(x.seed, id) % uint64(len(x.shards))
	return x.shards[shard].Add(id, v)
}

// Search returns the k vectors closest to q, searching every shard at once.
func (x *Sharded) Search(ctx context.Context, q shared.Vector, k int) ([]retrieval.Hit, error) {
	switch {
	case k < 1:
		return nil, fmt.Errorf("%w: %d", search.ErrInvalidK, k)
	case q.Dim() != x.dim:
		return nil, fmt.Errorf("%w: query of %d, the index has %d", search.ErrWrongDimension, q.Dim(), x.dim)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	// Scatter: each goroutine writes only its own slot, and wg.Wait makes the writes
	// visible here.
	hits := make([][]retrieval.Hit, len(x.shards))
	errs := make([]error, len(x.shards))
	var wg sync.WaitGroup
	for i, s := range x.shards {
		wg.Go(func() { hits[i], errs[i] = s.Search(ctx, q, k) })
	}
	wg.Wait()
	n := 0
	for i, err := range errs {
		if err != nil {
			return nil, err
		}
		n += len(hits[i])
	}
	if n == 0 {
		return []retrieval.Hit{}, nil
	}
	// Gather: the k best of the k best of each shard.
	best := topk.New(min(k, n), retrieval.Compare)
	for _, shard := range hits {
		for _, h := range shard {
			best.Offer(h)
		}
	}
	return best.Sorted(), nil
}

// Len returns how many vectors the shards keep together. During an Add, it may count the
// shards before and after it.
func (x *Sharded) Len() int {
	n := 0
	for _, s := range x.shards {
		n += s.Len()
	}
	return n
}
