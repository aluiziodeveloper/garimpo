package retrieval

import (
	"math"
	"slices"
	"testing"

	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// ranked returns the hits of ids, in that order, with falling scores from top.
func ranked(top float32, ids ...shared.ChunkID) []Hit {
	hits := make([]Hit, len(ids))
	for i, id := range ids {
		hits[i] = Hit{Chunk: id, Score: top - float32(i)}
	}
	return hits
}

func ids(hits []Hit) []shared.ChunkID {
	out := make([]shared.ChunkID, len(hits))
	for i, h := range hits {
		out[i] = h.Chunk
	}
	return out
}

// A chunk two rankings found beats one only a ranking found, even first there.
func TestFuseRanksFirstWhatBothRankingsFound(t *testing.T) {
	x, y, z := chunk(t, 0), chunk(t, 100), chunk(t, 200)
	got := ids(Fuse(10, ranked(0.9, x, y), ranked(12, z, x)))
	if want := []shared.ChunkID{x, z, y}; !slices.Equal(got, want) {
		t.Errorf("Fuse = %v, want %v", got, want)
	}
}

// The score of a chunk is the sum, over the rankings that hold it, of 1/(RRFK + its rank),
// the first rank being 1.
func TestFuseScoresTheSumOfTheReciprocalRanks(t *testing.T) {
	x, y, z := chunk(t, 0), chunk(t, 100), chunk(t, 200)
	want := map[shared.ChunkID]float64{
		x: 1.0/(RRFK+1) + 1.0/(RRFK+2),
		y: 1.0 / (RRFK + 2),
		z: 1.0 / (RRFK + 1),
	}
	for _, h := range Fuse(10, ranked(0.9, x, y), ranked(12, z, x)) {
		if math.Abs(float64(h.Score)-want[h.Chunk]) > 1e-7 {
			t.Errorf("chunk %v scores %v, want %v", h.Chunk, h.Score, want[h.Chunk])
		}
	}
}

// Only the ranks count: cosines and BM25 scores are on scales that cannot be added, and
// multiplying the scores of a ranking changes nothing.
func TestFuseReadsOnlyTheRanks(t *testing.T) {
	x, y, z := chunk(t, 0), chunk(t, 100), chunk(t, 200)
	a, b := ranked(0.9, x, y, z), ranked(0.5, z, y)
	scaled := slices.Clone(b)
	for i := range scaled {
		scaled[i].Score *= 1000
	}
	if got, want := Fuse(10, a, scaled), Fuse(10, a, b); !slices.Equal(got, want) {
		t.Errorf("Fuse with scores times 1000 = %v, want %v", got, want)
	}
}

// A chunk twice in one ranking counts once there, at its best rank.
func TestFuseCountsAChunkOncePerRanking(t *testing.T) {
	x, y := chunk(t, 0), chunk(t, 100)
	got := Fuse(10, []Hit{{Chunk: x, Score: 3}, {Chunk: y, Score: 2}, {Chunk: x, Score: 1}})
	if len(got) != 2 || got[0].Chunk != x || math.Abs(float64(got[0].Score)-1.0/(RRFK+1)) > 1e-7 {
		t.Errorf("Fuse = %v, want x once, at 1/(RRFK+1), then y", got)
	}
}

// Equal fused scores are ranked by ChunkID, as every ranking (Compare), and n bounds the
// hits.
func TestFuseKeepsTheNBestInOrder(t *testing.T) {
	x, y, z := chunk(t, 0), chunk(t, 100), chunk(t, 200)
	got := Fuse(2, ranked(1, z), ranked(1, y), ranked(1, x))
	want := []shared.ChunkID{x, y, z}
	slices.SortFunc(want, shared.ChunkID.Compare) // a ChunkID is a hash: not the offset's order
	if want = want[:2]; !slices.Equal(ids(got), want) {
		t.Errorf("Fuse(2) of three chunks first once each = %v, want %v, by ChunkID", ids(got), want)
	}
	if got := Fuse(0, ranked(1, x)); got == nil || len(got) != 0 {
		t.Errorf("Fuse(0) = %v, want an empty slice", got)
	}
	if got := Fuse(5); got == nil || len(got) != 0 {
		t.Errorf("Fuse of no ranking = %v, want an empty slice", got)
	}
}
