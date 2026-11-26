package retrieval

import (
	"slices"
	"testing"

	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

func chunk(t *testing.T, offset int) shared.ChunkID {
	t.Helper()
	id, err := shared.NewChunkID("sort/sort.go", offset, 1, 9, "package sort")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestCompareRanksTheHigherScoreFirst(t *testing.T) {
	a, b := Hit{Chunk: chunk(t, 0), Score: 0.9}, Hit{Chunk: chunk(t, 700), Score: 0.4}
	if Compare(a, b) >= 0 || Compare(b, a) <= 0 {
		t.Errorf("Compare(0.9, 0.4) = %d and Compare(0.4, 0.9) = %d, want the 0.9 first", Compare(a, b), Compare(b, a))
	}
}

// Equal scores are ranked by ChunkID, so every index gives the same ranking.
func TestCompareBreaksTiesByChunkID(t *testing.T) {
	x, y := chunk(t, 0), chunk(t, 700)
	if x.Compare(y) > 0 {
		x, y = y, x
	}
	hits := []Hit{{Chunk: y, Score: 0.5}, {Chunk: x, Score: 0.5}}
	slices.SortFunc(hits, Compare)
	if hits[0].Chunk != x {
		t.Errorf("equal scores ranked %s before %s, want the lower ChunkID first", hits[0].Chunk, hits[1].Chunk)
	}
	if Compare(hits[0], hits[0]) != 0 {
		t.Errorf("a hit does not compare equal to itself")
	}
}
