// Package indexing is the domain of the indexing context (ADR 0002): the snapshots of the
// index, the versions a search reads.
//
// Like the whole domain, it is pure and sequential. Publishing a snapshot to the searches
// is an adapter's job (internal/adapters/flatindex); this package only says what a
// snapshot is, and that it never changes.
package indexing

import (
	"errors"
	"fmt"
	"iter"

	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
	"github.com/aluiziodeveloper/garimpo/internal/vec"
)

var (
	// ErrWrongDimension is wrapped when a snapshot is asked for a dimension below 1, or
	// given a vector without its dimension.
	ErrWrongDimension = errors.New("wrong dimension for the snapshot")
	// ErrZeroChunk is wrapped when a snapshot is given the zero ChunkID.
	ErrZeroChunk = errors.New("zero chunk ID")
)

// IndexSnapshot is one version of the index: the chunks it held at one moment, with their
// vectors. It never changes (RN05): With returns a new version and leaves this one as it
// was, so a search that holds a snapshot reads the same chunks from start to end, however
// many are added meanwhile.
//
// A snapshot is a value, cheap to copy and to hand out. With writes the new chunk just past
// the end of the arrays of this version, when nothing else was written there, and shares
// them: the versions of a growing index cost no copy until the arrays are full. Reading a
// snapshot is safe for concurrent calls; With is not, and one caller at a time adds to the
// snapshots that share arrays.
type IndexSnapshot struct {
	version uint64
	dim     int
	ids     []shared.ChunkID
	values  []float32 // the vector of ids[i] is values[i*dim : (i+1)*dim]
	// taken is how many chunks of the arrays some version holds, shared by the versions
	// that share them: With writes in place only past it.
	taken *int
}

// Empty returns version 0 of an index of vectors of dim dimensions, holding no chunk.
func Empty(dim int) (IndexSnapshot, error) {
	if dim < 1 {
		return IndexSnapshot{}, fmt.Errorf("%w: %d", ErrWrongDimension, dim)
	}
	return IndexSnapshot{dim: dim, taken: new(int)}, nil
}

// Version returns the number of the version: 0 for an empty index, one more for each
// With.
func (s IndexSnapshot) Version() uint64 { return s.version }

// Dim returns the dimension of the vectors.
func (s IndexSnapshot) Dim() int { return s.dim }

// Len returns how many chunks the snapshot holds.
func (s IndexSnapshot) Len() int { return len(s.ids) }

// Chunk returns the ID of chunk i, in the order they were added.
func (s IndexSnapshot) Chunk(i int) shared.ChunkID { return s.ids[i] }

// Scores yields each chunk of the snapshot, in the order they were added, with the dot
// product of q and its vector. It panics if q does not have the snapshot's dimension.
func (s IndexSnapshot) Scores(q shared.Vector) iter.Seq2[shared.ChunkID, float32] {
	if q.Dim() != s.dim {
		panic(fmt.Sprintf("indexing: a query of %d dimensions for a snapshot of %d", q.Dim(), s.dim))
	}
	query := q.Values()
	return func(yield func(shared.ChunkID, float32) bool) {
		for i, id := range s.ids {
			if !yield(id, vec.Dot(query, s.values[i*s.dim:(i+1)*s.dim])) {
				return
			}
		}
	}
}

// With returns the next version: this snapshot with the chunk id and its vector v at the
// end. It fails, and returns nothing, on the zero IndexSnapshot, for the zero ChunkID or for
// a vector without the snapshot's dimension. Telling a chunk already held is the index's
// job.
func (s IndexSnapshot) With(id shared.ChunkID, v shared.Vector) (IndexSnapshot, error) {
	switch {
	case s.dim == 0:
		return IndexSnapshot{}, fmt.Errorf("%w: the zero snapshot holds no index; start from Empty", ErrWrongDimension)
	case id.IsZero():
		return IndexSnapshot{}, ErrZeroChunk
	case v.Dim() != s.dim:
		return IndexSnapshot{}, fmt.Errorf("%w: a vector of %d, the snapshot has %d", ErrWrongDimension, v.Dim(), s.dim)
	}
	next := IndexSnapshot{version: s.version + 1, dim: s.dim, ids: s.ids, values: s.values, taken: s.taken}
	if n := len(s.ids); *s.taken != n || n == cap(s.ids) {
		// Either another version was made from this one and wrote just past its end, or
		// the arrays are full: copy to new ones, with room to grow, so as never to write
		// over a chunk another version holds. Both grow together, so they are shared or
		// not shared together.
		size := max(2*n, 64)
		next.ids = append(make([]shared.ChunkID, 0, size), s.ids...)
		next.values = append(make([]float32, 0, size*s.dim), s.values...)
		next.taken = new(int)
	}
	next.ids = append(next.ids, id)
	next.values = append(next.values, v.Values()...)
	*next.taken = len(next.ids)
	return next, nil
}
