package search

import (
	"context"
	"errors"

	"github.com/aluiziodeveloper/garimpo/internal/domain/retrieval"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

var (
	// ErrWrongDimension is wrapped when a vector or a query does not have the index's
	// dimension: vectors of two models in one index, which no score can compare.
	ErrWrongDimension = errors.New("wrong dimension for the index")

	// ErrDuplicateChunk is wrapped when Add gets a chunk the index already has. A chunk
	// has one vector; a second one would make it two hits.
	ErrDuplicateChunk = errors.New("chunk already in the index")

	// ErrZeroChunk is wrapped when Add gets the zero ChunkID, which identifies no chunk.
	ErrZeroChunk = errors.New("zero chunk ID")

	// ErrInvalidK is wrapped when Search is asked for fewer than one hit, or, by the use
	// case, for more than MaxK.
	ErrInvalidK = errors.New("k must be at least 1")
)

// ScanCheck is how many vectors an exact index may score between two checks of ctx: often
// enough to stop a search of the corpus within a fraction of a millisecond of its deadline,
// seldom enough that the checks cost nothing next to the dot products.
const ScanCheck = 1024

// VectorIndex keeps the vectors of the chunks and finds the ones closest to a query: the
// port to every vector index of Garimpo, the exact one of module 12 and the HNSW of module
// 13.
//
// Add keeps v as the vector of the chunk id. It fails, and keeps nothing, with an error
// that wraps ErrZeroChunk for the zero ChunkID, ErrWrongDimension for a vector without the
// index's dimension and ErrDuplicateChunk for a chunk the index already has.
//
// Search returns at most k hits, best first in the order of retrieval.Compare, each with
// the dot product of q and the chunk's vector as its score. With fewer than k vectors in
// the index, it returns them all. It fails with an error that wraps ErrInvalidK when k is
// below 1, ErrWrongDimension when q does not have the index's dimension, and ctx's error
// when ctx is done before the search starts. An exact index returns the k best; an
// approximate one may miss some, as measured by recall.
//
// An exact index scans every vector, which takes milliseconds on the corpus, so it also
// checks ctx during the scan, at least once every ScanCheck vectors, and fails with ctx's
// error, and no hits, when ctx is done (INC-15). An approximate index, which visits few
// vectors, may check it only before it starts.
//
// Len returns how many vectors the index keeps. Add, Search and Len are safe for
// concurrent calls. indextest.Contract checks all of this.
type VectorIndex interface {
	Add(id shared.ChunkID, v shared.Vector) error
	Search(ctx context.Context, q shared.Vector, k int) ([]retrieval.Hit, error)
	Len() int
}

// KeywordIndex keeps the terms of the chunks' texts and finds the chunks whose terms best
// match the terms of a query: the port to the keyword index of the hybrid search (BM25).
//
// Add keeps text as the text of the chunk id. It fails, and keeps nothing, with an error
// that wraps ErrZeroChunk for the zero ChunkID and ErrDuplicateChunk for a chunk the index
// already has.
//
// Search returns at most k hits, best first in the order of retrieval.Compare: the chunks
// that hold at least one term of the query, each with its score. A query with no term has
// no hits. It fails with an error that wraps ErrInvalidK when k is below 1, and ctx's
// error when ctx is done before the search starts.
//
// Len returns how many chunks the index keeps. Add, Search and Len are safe for concurrent
// calls.
type KeywordIndex interface {
	Add(id shared.ChunkID, text string) error
	Search(ctx context.Context, query string, k int) ([]retrieval.Hit, error)
	Len() int
}
