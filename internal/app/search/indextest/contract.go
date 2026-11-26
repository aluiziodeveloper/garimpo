// Package indextest holds what every VectorIndex is tested with: the contract suite.
package indextest

import (
	"context"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/aluiziodeveloper/garimpo/internal/app/search"
	"github.com/aluiziodeveloper/garimpo/internal/domain/retrieval"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// dim is the dimension of the vectors of the suite: small, so the suite is fast, and above
// the 4 of an unrolled loop and the 8 of a SIMD register, so their tails run too.
const dim = 13

// tolerance is how far a score may be from the dot product the suite computes. The index may
// add the products in another order than shared.Vector.Dot does.
const tolerance = 1e-5

// Contract runs the tests every search.VectorIndex must pass. newIndex returns a new,
// empty index for vectors of dim dimensions; each subtest asks for its own.
//
// The suite is the one of an exact index: every search must return the k best hits, the
// ones a scan of every vector finds.
func Contract(t *testing.T, newIndex func(t *testing.T, dim int) search.VectorIndex) {
	t.Run("ANewIndexFindsNothing", func(t *testing.T) {
		idx := newIndex(t, dim)
		if n := idx.Len(); n != 0 {
			t.Fatalf("Len() = %d on a new index", n)
		}
		r := rand.New(rand.NewPCG(1, 1))
		if hits := find(t, idx, Vector(t, r, dim), 10); len(hits) != 0 {
			t.Fatalf("a new index found %d hits: %s", len(hits), short(hits))
		}
	})

	t.Run("AVectorFindsItself", func(t *testing.T) {
		idx := newIndex(t, dim)
		entries := fill(t, idx, 2, 100)
		for _, e := range entries[:10] {
			hits := find(t, idx, e.v, 1)
			if len(hits) != 1 || hits[0].Chunk != e.id || math.Abs(float64(hits[0].Score)-1) > tolerance {
				t.Fatalf("searching the vector of %s found %s, want that chunk with score 1", e.id, short(hits))
			}
		}
	})

	t.Run("SearchReturnsTheKBestInOrder", func(t *testing.T) {
		idx := newIndex(t, dim)
		entries := fill(t, idx, 3, 300)
		r := rand.New(rand.NewPCG(3, 3))
		for range 20 {
			q := Vector(t, r, dim)
			same(t, find(t, idx, q, 10), best(entries, q, 10))
		}
	})

	t.Run("AnIndexWithFewerThanKReturnsThemAll", func(t *testing.T) {
		idx := newIndex(t, dim)
		entries := fill(t, idx, 4, 3)
		q := entries[0].v
		same(t, find(t, idx, q, 10), best(entries, q, 10))
	})

	t.Run("EqualScoresAreRankedByChunkID", func(t *testing.T) {
		idx := newIndex(t, dim)
		entries := fill(t, idx, 5, 50)
		v := entries[0].v
		for i := range 5 {
			id := Chunk(t, 1000+i)
			add(t, idx, id, v)
			entries = append(entries, entry{id, v})
		}
		same(t, find(t, idx, v, 6), best(entries, v, 6))
	})

	t.Run("AddRefusesWhatTheIndexCannotKeep", func(t *testing.T) {
		idx := newIndex(t, dim)
		r := rand.New(rand.NewPCG(6, 6))
		first := Vector(t, r, dim)
		add(t, idx, Chunk(t, 0), first)
		tests := []struct {
			name string
			id   shared.ChunkID
			v    shared.Vector
			want error
		}{
			{"zero chunk", shared.ChunkID{}, Vector(t, r, dim), search.ErrZeroChunk},
			{"wrong dimension", Chunk(t, 1), Vector(t, r, dim+1), search.ErrWrongDimension},
			{"zero vector", Chunk(t, 1), shared.Vector{}, search.ErrWrongDimension},
			{"duplicate chunk", Chunk(t, 0), Vector(t, r, dim), search.ErrDuplicateChunk},
		}
		for _, tt := range tests {
			if err := idx.Add(tt.id, tt.v); !errors.Is(err, tt.want) {
				t.Errorf("Add of a %s: err = %v, want one that wraps %v", tt.name, err, tt.want)
			}
		}
		if n := idx.Len(); n != 1 {
			t.Fatalf("Len() = %d after one Add and four refused, want 1", n)
		}
		if hits := find(t, idx, first, 1); len(hits) != 1 || math.Abs(float64(hits[0].Score)-1) > tolerance {
			t.Fatalf("after the refused duplicate, the first vector found %s, want its chunk with score 1", short(hits))
		}
	})

	t.Run("SearchRefusesWhatItCannotAnswer", func(t *testing.T) {
		idx := newIndex(t, dim)
		fill(t, idx, 7, 10)
		r := rand.New(rand.NewPCG(7, 7))
		tests := []struct {
			name string
			q    shared.Vector
			k    int
			want error
		}{
			{"k of 0", Vector(t, r, dim), 0, search.ErrInvalidK},
			{"negative k", Vector(t, r, dim), -1, search.ErrInvalidK},
			{"query of another dimension", Vector(t, r, dim-1), 10, search.ErrWrongDimension},
		}
		for _, tt := range tests {
			if hits, err := idx.Search(t.Context(), tt.q, tt.k); !errors.Is(err, tt.want) || hits != nil {
				t.Errorf("Search with a %s: %d hits, err = %v, want none and one that wraps %v", tt.name, len(hits), err, tt.want)
			}
		}
	})

	t.Run("ADoneContextFailsTheSearch", func(t *testing.T) {
		idx := newIndex(t, dim)
		fill(t, idx, 8, 10)
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		r := rand.New(rand.NewPCG(8, 8))
		if hits, err := idx.Search(ctx, Vector(t, r, dim), 5); !errors.Is(err, context.Canceled) || hits != nil {
			t.Fatalf("Search with a canceled context: %d hits, err = %v, want none and context.Canceled", len(hits), err)
		}
	})

	t.Run("TheHitsBelongToTheCaller", func(t *testing.T) {
		idx := newIndex(t, dim)
		entries := fill(t, idx, 9, 20)
		q := entries[0].v
		first := find(t, idx, q, 5)
		for i := range first {
			first[i] = retrieval.Hit{}
		}
		same(t, find(t, idx, q, 5), best(entries, q, 5))
	})

	t.Run("AddAndSearchAreSafeForConcurrentCalls", func(t *testing.T) {
		idx := newIndex(t, dim)
		const adders, each = 4, 50
		var wg sync.WaitGroup
		vectors := make([][]entry, adders)
		for a := range adders {
			r := rand.New(rand.NewPCG(10, uint64(a)))
			for i := range each {
				vectors[a] = append(vectors[a], entry{Chunk(t, a*each+i), Vector(t, r, dim)})
			}
			wg.Go(func() {
				for _, e := range vectors[a] {
					if err := idx.Add(e.id, e.v); err != nil {
						t.Errorf("concurrent Add: %v", err)
					}
				}
			})
		}
		for s := range 4 {
			r := rand.New(rand.NewPCG(11, uint64(s)))
			q := Vector(t, r, dim)
			wg.Go(func() {
				for range 50 {
					if _, err := idx.Search(t.Context(), q, 5); err != nil {
						t.Errorf("concurrent Search: %v", err)
					}
					_ = idx.Len()
				}
			})
		}
		wg.Wait()
		if n := idx.Len(); n != adders*each {
			t.Fatalf("Len() = %d after %d concurrent Adds", n, adders*each)
		}
		all := slices.Concat(vectors...)
		for _, e := range all[:10] {
			same(t, find(t, idx, e.v, 3), best(all, e.v, 3))
		}
	})
}

// entry is a chunk and its vector, as the suite added them.
type entry struct {
	id shared.ChunkID
	v  shared.Vector
}

// Chunk returns the ID of a chunk of the suite, one for each n.
func Chunk(t testing.TB, n int) shared.ChunkID {
	t.Helper()
	id, err := shared.NewChunkID("index/contract.go", n, 1, 1, fmt.Sprint("chunk ", n))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// Vector returns a random vector of dim dimensions, with norm 1.
func Vector(t testing.TB, r *rand.Rand, dim int) shared.Vector {
	t.Helper()
	values := make([]float32, dim)
	for i := range values {
		values[i] = float32(r.NormFloat64())
	}
	v, err := shared.NewVector(values, dim)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// fill adds n random vectors to idx, from the seed, and returns them.
func fill(t *testing.T, idx search.VectorIndex, seed uint64, n int) []entry {
	t.Helper()
	r := rand.New(rand.NewPCG(seed, seed))
	entries := make([]entry, n)
	for i := range entries {
		entries[i] = entry{Chunk(t, i), Vector(t, r, dim)}
		add(t, idx, entries[i].id, entries[i].v)
	}
	if got := idx.Len(); got != n {
		t.Fatalf("Len() = %d after %d Adds", got, n)
	}
	return entries
}

func add(t *testing.T, idx search.VectorIndex, id shared.ChunkID, v shared.Vector) {
	t.Helper()
	if err := idx.Add(id, v); err != nil {
		t.Fatalf("Add(%s): %v", id, err)
	}
}

func find(t *testing.T, idx search.VectorIndex, q shared.Vector, k int) []retrieval.Hit {
	t.Helper()
	hits, err := idx.Search(t.Context(), q, k)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	return hits
}

// best returns the k best hits of q among the entries, scanning them all.
func best(entries []entry, q shared.Vector, k int) []retrieval.Hit {
	hits := make([]retrieval.Hit, len(entries))
	for i, e := range entries {
		hits[i] = retrieval.Hit{Chunk: e.id, Score: q.Dot(e.v)}
	}
	slices.SortFunc(hits, retrieval.Compare)
	return hits[:min(k, len(hits))]
}

// same fails the test unless got has the chunks of want, in the same order, with the same
// scores.
func same(t *testing.T, got, want []retrieval.Hit) {
	t.Helper()
	ok := len(got) == len(want)
	for i := 0; ok && i < len(got); i++ {
		ok = got[i].Chunk == want[i].Chunk && math.Abs(float64(got[i].Score-want[i].Score)) <= tolerance
	}
	if !ok {
		t.Fatalf("Search found\n%s\nwant\n%s", short(got), short(want))
	}
}

// short writes hits as the first 8 digits of each chunk and its score.
func short(hits []retrieval.Hit) string {
	var b strings.Builder
	for i, h := range hits {
		if i > 0 {
			b.WriteByte(' ')
		}
		fmt.Fprintf(&b, "%.8s:%.4f", h.Chunk, h.Score)
	}
	return "[" + b.String() + "]"
}
