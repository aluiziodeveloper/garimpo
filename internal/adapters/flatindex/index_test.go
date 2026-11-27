package flatindex_test

import (
	"fmt"
	"math/rand/v2"
	"sync"
	"sync/atomic"
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

func TestPublishedMeetsTheContract(t *testing.T) {
	indextest.Contract(t, func(_ *testing.T, dim int) search.VectorIndex {
		return flatindex.NewPublished(dim)
	})
}

// A snapshot taken before an Add stays as it was (RN05): the Add publishes the next
// version, and a search that started on the old one keeps reading the old one.
func TestASnapshotStaysAsItWasAfterAnAdd(t *testing.T) {
	x := flatindex.NewPublished(4)
	r := rand.New(rand.NewPCG(9, 9))
	for i := range 3 {
		if err := x.Add(indextest.Chunk(t, i), indextest.Vector(t, r, 4)); err != nil {
			t.Fatal(err)
		}
	}
	before := x.Snapshot()
	if err := x.Add(indextest.Chunk(t, 3), indextest.Vector(t, r, 4)); err != nil {
		t.Fatal(err)
	}
	after := x.Snapshot()
	if before.Len() != 3 || before.Version() != 3 {
		t.Errorf("the snapshot taken before the Add has %d chunks, version %d; want 3, version 3", before.Len(), before.Version())
	}
	if after.Len() != 4 || after.Version() != 4 {
		t.Errorf("the snapshot taken after the Add has %d chunks, version %d; want 4, version 4", after.Len(), after.Version())
	}
}

// A search sees every Add that returned before it started: the consistency is eventual
// only for the Adds still under way. One goroutine adds 500 vectors and counts the Adds
// that returned; 4 others, after reading the count, search for the last vector counted and
// must find it first.
func TestASearchSeesEveryAddThatReturned(t *testing.T) {
	for _, ix := range []struct {
		name string
		idx  search.VectorIndex
	}{{"Index", flatindex.New(16)}, {"Published", flatindex.NewPublished(16)}} {
		t.Run(ix.name, func(t *testing.T) {
			r := rand.New(rand.NewPCG(5, 7))
			vs := make([]shared.Vector, 500)
			for i := range vs {
				vs[i] = indextest.Vector(t, r, 16)
			}
			var returned atomic.Int64
			var wg sync.WaitGroup
			for range 4 {
				wg.Go(func() {
					for c := returned.Load(); c < int64(len(vs)); c = returned.Load() {
						if c == 0 {
							continue
						}
						hits, err := ix.idx.Search(t.Context(), vs[c-1], 1)
						if err != nil {
							t.Error(err)
							return
						}
						if len(hits) != 1 || hits[0].Chunk != indextest.Chunk(t, int(c-1)) {
							t.Errorf("after %d Adds returned, a search for vector %d found %v", c, c-1, hits)
							return
						}
					}
				})
			}
			for i, v := range vs {
				if err := ix.idx.Add(indextest.Chunk(t, i), v); err != nil {
					t.Error(err)
					returned.Store(int64(len(vs)))
					break
				}
				returned.Store(int64(i + 1))
			}
			wg.Wait()
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
// BenchmarkSearchPublished is BenchmarkSearch on the index that publishes snapshots.
func BenchmarkSearchPublished(b *testing.B) {
	idx, q := filled(b, flatindex.NewPublished(dim))
	for b.Loop() {
		if _, err := idx.Search(b.Context(), q, 10); err != nil {
			b.Fatal(err)
		}
	}
}

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
