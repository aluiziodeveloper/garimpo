package hnsw

import (
	"math/rand/v2"
	"os"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/aluiziodeveloper/garimpo/internal/adapters/flatindex"
	"github.com/aluiziodeveloper/garimpo/internal/adapters/vectorfile"
	"github.com/aluiziodeveloper/garimpo/internal/app/search"
	"github.com/aluiziodeveloper/garimpo/internal/app/search/indextest"
	"github.com/aluiziodeveloper/garimpo/internal/domain/retrieval"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// recallK is the k of recall@k, the 10 best of a query, and wantRecall the recall the
// graph must reach with the default EfSearch: the 0.95 of RNF04.
const (
	recallK    = 10
	wantRecall = 0.95
)

// build adds the vectors to a new graph and to the exact index, from workers goroutines
// at once, and returns both and how long the graph took.
func build(t *testing.T, cfg Config, vectors []shared.Vector, workers int) (*Index, *flatindex.Index, time.Duration) {
	t.Helper()
	dim := vectors[0].Dim()
	graph, exact := New(dim, cfg), flatindex.New(dim)
	for i, v := range vectors {
		if err := exact.Add(indextest.Chunk(t, i), v); err != nil {
			t.Fatal(err)
		}
	}
	next := make(chan int)
	var wg sync.WaitGroup
	start := time.Now()
	for range workers {
		wg.Go(func() {
			for i := range next {
				if err := graph.Add(indextest.Chunk(t, i), vectors[i]); err != nil {
					t.Error(err)
				}
			}
		})
	}
	for i := range vectors {
		next <- i
	}
	close(next)
	wg.Wait()
	return graph, exact, time.Since(start)
}

// recallOf returns the share of the k best of each query, by the exact index, that the
// graph found, and the mean time of a search in the graph.
func recallOf(t *testing.T, graph *Index, exact *flatindex.Index, queries []shared.Vector) (float64, time.Duration) {
	t.Helper()
	found, elapsed := 0, time.Duration(0)
	for _, q := range queries {
		want := find(t, exact, q)
		start := time.Now()
		got := find(t, graph, q)
		elapsed += time.Since(start)
		in := make(map[shared.ChunkID]bool, len(got))
		for _, h := range got {
			in[h.Chunk] = true
		}
		for _, h := range want {
			if in[h.Chunk] {
				found++
			}
		}
	}
	return float64(found) / float64(recallK*len(queries)), elapsed / time.Duration(len(queries))
}

// find returns the recallK best of q in idx.
func find(t *testing.T, idx search.VectorIndex, q shared.Vector) []retrieval.Hit {
	t.Helper()
	hits, err := idx.Search(t.Context(), q, recallK)
	if err != nil {
		t.Fatal(err)
	}
	return hits
}

// The graph finds at least 95% of the 10 best that the exact index finds, on random
// vectors small enough for the CI: 2000 of 32 dimensions, 100 queries outside the index.
func TestRecallAgainstTheExactIndex(t *testing.T) {
	r := rand.New(rand.NewPCG(6, 6))
	vectors := make([]shared.Vector, 2100)
	for i := range vectors {
		vectors[i] = indextest.Vector(t, r, 32)
	}
	graph, exact, _ := build(t, Config{Seed: 6}, vectors[:2000], 4)
	got, _ := recallOf(t, graph, exact, vectors[2000:])
	t.Logf("recall@%d = %.3f", recallK, got)
	if got < wantRecall {
		t.Fatalf("recall@%d = %.3f, want at least %.2f", recallK, got, wantRecall)
	}
}

// TestRecallOnTheCorpus measures the graph on the real vectors of the corpus, the file
// that garimpo ingest -vectors writes, named by GARIMPO_VECTORS; without it, it skips. The
// last 500 vectors are the queries, outside the index. The graph is built by GOMAXPROCS
// goroutines at once, and the test logs the recall and the time of a search for each
// EfSearch, and fails if the default one does not reach 0.95.
//
//	GARIMPO_VECTORS=corpus.vec go test -run TestRecallOnTheCorpus -v ./internal/adapters/hnsw
func TestRecallOnTheCorpus(t *testing.T) {
	path := os.Getenv("GARIMPO_VECTORS")
	if path == "" {
		t.Skip("set GARIMPO_VECTORS to a file of garimpo ingest -vectors")
	}
	vectors, err := vectorfile.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	const queries = 500
	if len(vectors) <= queries {
		t.Fatalf("%s has %d vectors, want more than %d", path, len(vectors), queries)
	}
	workers := runtime.GOMAXPROCS(0)
	graph, exact, took := build(t, Config{Seed: 7}, vectors[:len(vectors)-queries], workers)
	t.Logf("%d vectors of %d dimensions, %d queries; graph built by %d goroutines in %v",
		len(vectors)-queries, vectors[0].Dim(), queries, workers, took.Round(time.Millisecond))

	start := time.Now()
	for _, q := range vectors[len(vectors)-queries:] {
		find(t, exact, q)
	}
	t.Logf("exact index: %v per search", (time.Since(start) / queries).Round(time.Microsecond))

	defaultEf := graph.cfg.EfSearch
	var atDefault float64
	for _, ef := range []int{10, 32, 64, 128, 256} {
		graph.cfg.EfSearch = ef
		got, per := recallOf(t, graph, exact, vectors[len(vectors)-queries:])
		t.Logf("EfSearch=%-3d recall@%d=%.3f  %v per search", ef, recallK, got, per.Round(time.Microsecond))
		if ef == defaultEf {
			atDefault = got
		}
	}
	if atDefault < wantRecall {
		t.Errorf("recall@%d = %.3f with the default EfSearch %d, want at least %.2f", recallK, atDefault, defaultEf, wantRecall)
	}
}
