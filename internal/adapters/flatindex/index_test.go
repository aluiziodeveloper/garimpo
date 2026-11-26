package flatindex_test

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/aluiziodeveloper/garimpo/internal/adapters/flatindex"
	"github.com/aluiziodeveloper/garimpo/internal/app/search"
	"github.com/aluiziodeveloper/garimpo/internal/app/search/indextest"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
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
	idx, q := filled(b, flatindex.New(dim))
	for b.Loop() {
		if _, err := idx.Search(b.Context(), q, 10); err != nil {
			b.Fatal(err)
		}
	}
}

// shardCounts are the numbers of shards the benchmarks compare.
var shardCounts = []int{1, 2, 4, 8, 16}

// BenchmarkShardedSearch measures the latency of one search at a time, the case of a
// single user: with more shards, more CPUs share the scan of each query.
func BenchmarkShardedSearch(b *testing.B) {
	for _, n := range shardCounts {
		b.Run(fmt.Sprint("shards=", n), func(b *testing.B) {
			idx, q := filled(b, flatindex.NewSharded(dim, n))
			for b.Loop() {
				if _, err := idx.Search(b.Context(), q, 10); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkShardedSearchParallel runs as many searches at once as GOMAXPROCS, the case of
// a busy server: every CPU already has a query to scan, and ns/op is the time per search
// of the whole, the inverse of the throughput.
func BenchmarkShardedSearchParallel(b *testing.B) {
	for _, n := range shardCounts {
		b.Run(fmt.Sprint("shards=", n), func(b *testing.B) {
			idx, q := filled(b, flatindex.NewSharded(dim, n))
			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					if _, err := idx.Search(b.Context(), q, 10); err != nil {
						b.Error(err)
						return
					}
				}
			})
		})
	}
}

// filled adds corpus random vectors to idx and returns it with a random query.
func filled(b *testing.B, idx search.VectorIndex) (search.VectorIndex, shared.Vector) {
	b.Helper()
	r := rand.New(rand.NewPCG(1, 2))
	for i := range corpus {
		if err := idx.Add(indextest.Chunk(b, i), indextest.Vector(b, r, dim)); err != nil {
			b.Fatal(err)
		}
	}
	return idx, indextest.Vector(b, r, dim)
}
