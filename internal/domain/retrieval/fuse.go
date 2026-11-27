package retrieval

import (
	"slices"

	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// RRFK is the k of Reciprocal Rank Fusion: the larger it is, the less the first ranks count
// over the next ones. 60 is the value of the paper that proposed RRF, and of most systems.
const RRFK = 60

// Fuse returns at most n hits of the rankings fused by Reciprocal Rank Fusion (RRF), best
// first in the order of Compare: none when n is below 1. Each ranking must be in the order
// of Compare, as a search returns it.
//
// The score of a chunk is the sum, over the rankings that hold it, of 1/(RRFK + r), r being
// its rank there, from 1. Fuse reads only the ranks: the cosine of a vector search and the
// BM25 score of a keyword search are on scales that cannot be added, but a rank is a rank.
// A chunk twice in one ranking counts once there, at its best rank.
func Fuse(n int, rankings ...[]Hit) []Hit {
	scores := make(map[shared.ChunkID]float64)
	for _, ranking := range rankings {
		seen := make(map[shared.ChunkID]bool, len(ranking))
		for i, h := range ranking {
			if seen[h.Chunk] {
				continue
			}
			seen[h.Chunk] = true
			scores[h.Chunk] += 1 / float64(RRFK+i+1)
		}
	}
	fused := make([]Hit, 0, len(scores))
	for id, s := range scores {
		fused = append(fused, Hit{Chunk: id, Score: float32(s)})
	}
	slices.SortFunc(fused, Compare)
	return fused[:max(0, min(n, len(fused)))]
}
