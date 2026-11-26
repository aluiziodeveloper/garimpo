package flatindex_test

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/aluiziodeveloper/garimpo/internal/adapters/flatindex"
	"github.com/aluiziodeveloper/garimpo/internal/app/search/indextest"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

func TestSearchBatchFindsWhatSearchFinds(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 5))
	idx := flatindex.New(16)
	for i := range 500 {
		if err := idx.Add(indextest.Chunk(t, i), indextest.Vector(t, r, 16)); err != nil {
			t.Fatal(err)
		}
	}
	qs := make([]shared.Vector, 7)
	for i := range qs {
		qs[i] = indextest.Vector(t, r, 16)
	}
	got, err := idx.SearchBatch(t.Context(), qs, 10)
	if err != nil {
		t.Fatal(err)
	}
	for i, q := range qs {
		want, err := idx.Search(t.Context(), q, 10)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got[i], want) {
			t.Errorf("query %d: SearchBatch found %v, Search %v", i, got[i], want)
		}
	}
}

// BenchmarkSearchBatch searches the corpus for a batch of queries, in one SearchBatch and
// in one Search per query; ns/op is the time of the whole batch.
func BenchmarkSearchBatch(b *testing.B) {
	idx := flatindex.New(dim)
	filled(b, idx)
	r := rand.New(rand.NewPCG(3, 4))
	for _, n := range []int{1, 4, 8, 16} {
		qs := make([]shared.Vector, n)
		for i := range qs {
			qs[i] = indextest.Vector(b, r, dim)
		}
		b.Run(fmt.Sprint("queries=", n, "/batch"), func(b *testing.B) {
			for b.Loop() {
				if _, err := idx.SearchBatch(b.Context(), qs, 10); err != nil {
					b.Fatal(err)
				}
			}
		})
		b.Run(fmt.Sprint("queries=", n, "/one-by-one"), func(b *testing.B) {
			for b.Loop() {
				for _, q := range qs {
					if _, err := idx.Search(b.Context(), q, 10); err != nil {
						b.Fatal(err)
					}
				}
			}
		})
	}
}
