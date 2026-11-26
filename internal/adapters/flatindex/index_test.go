package flatindex_test

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/aluiziodeveloper/garimpo/internal/adapters/flatindex"
	"github.com/aluiziodeveloper/garimpo/internal/app/search"
	"github.com/aluiziodeveloper/garimpo/internal/app/search/indextest"
)

func TestIndexMeetsTheContract(t *testing.T) {
	indextest.Contract(t, func(_ *testing.T, dim int) search.VectorIndex {
		return flatindex.New(dim)
	})
}

func TestShardedMeetsTheContract(t *testing.T) {
	for _, shards := range []int{1, 3, 8} {
		t.Run(fmt.Sprint(shards, " shards"), func(t *testing.T) {
			indextest.Contract(t, func(_ *testing.T, dim int) search.VectorIndex {
				return flatindex.NewSharded(dim, shards)
			})
		})
	}
}

// corpus is the number of chunks of the whole corpus (module 07), and dim the dimension
// of all-minilm.
const corpus, dim = 41_980, 384

// BenchmarkSearch searches an index the size of the corpus for the 10 best of a query.
func BenchmarkSearch(b *testing.B) {
	r := rand.New(rand.NewPCG(1, 2))
	idx := flatindex.New(dim)
	for i := range corpus {
		if err := idx.Add(indextest.Chunk(b, i), indextest.Vector(b, r, dim)); err != nil {
			b.Fatal(err)
		}
	}
	q := indextest.Vector(b, r, dim)
	for b.Loop() {
		if _, err := idx.Search(b.Context(), q, 10); err != nil {
			b.Fatal(err)
		}
	}
}
