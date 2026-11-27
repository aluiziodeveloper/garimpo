package indexing_test

import (
	"errors"
	"fmt"
	"testing"

	"github.com/aluiziodeveloper/garimpo/internal/domain/indexing"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

const dim = 3

// chunk returns the ID of a chunk numbered n.
func chunk(t *testing.T, n int) shared.ChunkID {
	t.Helper()
	id, err := shared.NewChunkID("doc.go", n, n+1, n+1, fmt.Sprint("chunk ", n))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// vector returns a vector of norm 1 that tells chunk n apart: n+1 on the first axis and 1
// on the second, normalized.
func vector(t *testing.T, n int) shared.Vector {
	t.Helper()
	v, err := shared.NewVector([]float32{float32(n + 1), 1, 0}, dim)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func empty(t *testing.T) indexing.IndexSnapshot {
	t.Helper()
	s, err := indexing.Empty(dim)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func with(t *testing.T, s indexing.IndexSnapshot, n int) indexing.IndexSnapshot {
	t.Helper()
	next, err := s.With(chunk(t, n), vector(t, n))
	if err != nil {
		t.Fatal(err)
	}
	return next
}

// holds fails unless s holds the chunks ns, in that order, each with its own vector: the
// score of chunk n's vector against itself is where chunk n is.
func holds(t *testing.T, s indexing.IndexSnapshot, ns ...int) {
	t.Helper()
	if s.Len() != len(ns) {
		t.Fatalf("the snapshot holds %d chunks, want %d", s.Len(), len(ns))
	}
	for i, n := range ns {
		if got := s.Chunk(i); got != chunk(t, n) {
			t.Errorf("chunk %d of the snapshot is %v, want chunk %d", i, got, n)
		}
		v, j := vector(t, n), 0
		for id, score := range s.Scores(v) {
			if j == i && (id != chunk(t, n) || score != v.Dot(v)) {
				t.Errorf("score %d is %v for %v, want %v for chunk %d", i, score, id, v.Dot(v), n)
			}
			j++
		}
	}
}

func TestAnEmptySnapshotHoldsNothing(t *testing.T) {
	s := empty(t)
	if s.Len() != 0 || s.Version() != 0 || s.Dim() != dim {
		t.Errorf("Empty(%d): Len %d, Version %d, Dim %d; want 0, 0 and %d", dim, s.Len(), s.Version(), s.Dim(), dim)
	}
	if _, err := indexing.Empty(0); !errors.Is(err, indexing.ErrWrongDimension) {
		t.Errorf("Empty(0): err = %v, want one that wraps %v", err, indexing.ErrWrongDimension)
	}
}

// With returns a new version and leaves the snapshot it was called on as it was (RN05): a
// search that holds it keeps reading the same chunks.
func TestWithLeavesTheSnapshotAsItWas(t *testing.T) {
	s0 := empty(t)
	s1 := with(t, s0, 1)
	s2 := with(t, s1, 2)
	holds(t, s0)
	holds(t, s1, 1)
	holds(t, s2, 1, 2)
	if s1.Version() != 1 || s2.Version() != 2 {
		t.Errorf("versions %d and %d, want 1 and 2", s1.Version(), s2.Version())
	}
}

func TestWithRefusesWhatTheSnapshotCannotHold(t *testing.T) {
	s := empty(t)
	if _, err := s.With(shared.ChunkID{}, vector(t, 1)); !errors.Is(err, indexing.ErrZeroChunk) {
		t.Errorf("With of the zero chunk: err = %v, want one that wraps %v", err, indexing.ErrZeroChunk)
	}
	other, err := shared.NewVector([]float32{1, 0}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.With(chunk(t, 1), other); !errors.Is(err, indexing.ErrWrongDimension) {
		t.Errorf("With of a vector of 2: err = %v, want one that wraps %v", err, indexing.ErrWrongDimension)
	}
	holds(t, s)
}

// Two versions made from the same snapshot each keep what they added. The second With
// cannot write where the first one did, in the same array: that would change a snapshot
// that was already handed out.
func TestTwoVersionsOfOneSnapshotKeepWhatEachAdded(t *testing.T) {
	s1 := with(t, empty(t), 1)
	a := with(t, s1, 2)
	b := with(t, s1, 3)
	holds(t, a, 1, 2)
	holds(t, b, 1, 3)
	holds(t, with(t, a, 4), 1, 2, 4)
	holds(t, with(t, b, 5), 1, 3, 5)
	holds(t, s1, 1)
}

// A long run of With keeps every snapshot of the way as it was made, as the array under
// them grows.
func TestEverySnapshotOfAGrowingIndexStaysAsItWas(t *testing.T) {
	snaps := []indexing.IndexSnapshot{empty(t)}
	for n := range 100 {
		snaps = append(snaps, with(t, snaps[len(snaps)-1], n))
	}
	for i, s := range snaps {
		want := make([]int, i)
		for n := range want {
			want[n] = n
		}
		holds(t, s, want...)
	}
}
