package flatindex_test

import (
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aluiziodeveloper/garimpo/internal/adapters/vectorfile"
	"github.com/aluiziodeveloper/garimpo/internal/app/search"
	"github.com/aluiziodeveloper/garimpo/internal/app/search/indextest"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// TestStalenessOnTheCorpus measures how long each exact index takes to show a vector: from
// the moment an Add of it starts to the moment another goroutine sees it in Len. It runs on
// the real vectors of the corpus, named by GARIMPO_VECTORS like
// TestSearchDuringIngestOnTheCorpus, and skips without them. While 4 goroutines search as
// fast as they can, one goroutine adds the last 100 vectors, one every 35 ms (the pace of
// an ingestion), and another one watches Len. It logs the percentiles of the staleness and
// of the Adds.
//
//	GARIMPO_VECTORS=corpus.vec go test -run TestStalenessOnTheCorpus -v ./internal/adapters/flatindex
func TestStalenessOnTheCorpus(t *testing.T) {
	path := os.Getenv("GARIMPO_VECTORS")
	if path == "" {
		t.Skip("set GARIMPO_VECTORS to a file of garimpo ingest -vectors")
	}
	vectors, err := vectorfile.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	const added, searchers, pace = 100, 4, 35 * time.Millisecond
	if len(vectors) <= added {
		t.Fatalf("%s has %d vectors, want more than %d", path, len(vectors), added)
	}
	base, extra := vectors[:len(vectors)-added], vectors[len(vectors)-added:]
	for _, ix := range exactIndexes {
		idx := ix.new(base[0].Dim())
		for i, v := range base {
			if err := idx.Add(indextest.Chunk(t, i), v); err != nil {
				t.Fatal(err)
			}
		}
		stale, adds := staleness(t, idx, extra, len(base), searchers, pace)
		t.Logf("%-9s %d searchers, an add every %v: staleness p50 %-8v p99 %-8v max %-8v | adds %d  p50 %-8v p99 %-8v max %v",
			ix.name, searchers, pace, pct(stale, 50), pct(stale, 99), pct(stale, 100), len(adds), pct(adds, 50), pct(adds, 99), pct(adds, 100))
	}
}

// staleness adds vs to idx, which holds next vectors, one every pause, while searchers
// goroutines search it for vs and one more watches idx.Len. It returns, for each vector,
// the time from the start of its Add to the moment Len showed it, and how long each Add
// took.
func staleness(t *testing.T, idx search.VectorIndex, vs []shared.Vector, next, searchers int, pause time.Duration) (stale, adds []time.Duration) {
	started := make([]time.Time, len(vs)) // written here, before each Add
	seen := make([]time.Time, len(vs))    // written by the watcher
	var stop, failed atomic.Bool          // stop the searchers; stop the watcher too
	var wg sync.WaitGroup
	for g := range searchers {
		wg.Go(func() {
			for i := g; !stop.Load(); i++ {
				if _, err := idx.Search(t.Context(), vs[i%len(vs)], 10); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	wg.Go(func() { // the watcher
		for shown := 0; shown < len(vs) && !failed.Load(); {
			n := idx.Len() - next
			now := time.Now()
			for ; shown < n; shown++ {
				seen[shown] = now
			}
		}
	})
	time.Sleep(100 * time.Millisecond) // the searches under way
	for i, v := range vs {
		started[i] = time.Now()
		if err := idx.Add(indextest.Chunk(t, next+i), v); err != nil {
			t.Error(err)
			failed.Store(true)
			break
		}
		adds = append(adds, time.Since(started[i]))
		time.Sleep(pause)
	}
	stop.Store(true)
	wg.Wait()
	if t.Failed() {
		t.FailNow()
	}
	for i := range vs {
		if seen[i].IsZero() {
			t.Fatalf("the watcher never saw vector %d", i)
		}
		stale = append(stale, seen[i].Sub(started[i]))
	}
	return stale, adds
}
