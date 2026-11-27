package retrieval

import (
	"cmp"

	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// Hit is a chunk a search found and its score. Higher is closer, on the scale of the search
// that made it: for a vector index, the dot product of the query and the chunk's vector,
// which is their cosine similarity (RN07), between -1 and 1 give or take the rounding of
// float32; for the keyword index, the BM25 score of the chunk, 0 or more; for Fuse, the sum
// of the reciprocal ranks.
//
// It is a value with no invariant of its own: the index makes it from a chunk and a vector
// or a text that were valid when added.
type Hit struct {
	Chunk shared.ChunkID
	Score float32
}

// Compare is the order of a ranking: it returns a negative number when a comes before b.
// The higher score comes first and, between equal scores, the lower ChunkID, so a search
// ranks its hits the same way in every run and in every index. It is the cmp function
// slices.SortFunc takes.
func Compare(a, b Hit) int {
	if c := cmp.Compare(b.Score, a.Score); c != 0 {
		return c
	}
	return a.Chunk.Compare(b.Chunk)
}
