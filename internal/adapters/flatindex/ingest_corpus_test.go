package flatindex_test

import (
	"fmt"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aluiziodeveloper/garimpo/internal/adapters/flatindex"
	"github.com/aluiziodeveloper/garimpo/internal/adapters/vectorfile"
	"github.com/aluiziodeveloper/garimpo/internal/app/search"
	"github.com/aluiziodeveloper/garimpo/internal/app/search/indextest"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// TestSearchDuringIngestOnTheCorpus measures the searches and the Adds of each exact index
// at once, on the real vectors of the corpus, the file that garimpo ingest -vectors writes,
// named by GARIMPO_VECTORS; without it, it skips (INC-14). The index holds all but the last
// 500 vectors, which are the queries and the vectors added. For 5 s, 4 goroutines search
// as fast as they can, while nothing is added, while one goroutine adds a vector every 35
// ms (the pace of an ingestion: 39,244 vectors in 22m48s), and while it adds them as fast
// as it can; and, last, the Adds alone. It logs the percentiles of each.
//
//	GARIMPO_VECTORS=corpus.vec go test -run TestSearchDuringIngestOnTheCorpus -v ./internal/adapters/flatindex
func TestSearchDuringIngestOnTheCorpus(t *testing.T) {
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
	base, extra := vectors[:len(vectors)-queries], vectors[len(vectors)-queries:]
	indexes := []struct {
		name string
		new  func(dim int) search.VectorIndex
	}{
		{"Index", func(dim int) search.VectorIndex { return flatindex.New(dim) }},
		{"Published", func(dim int) search.VectorIndex { return flatindex.NewPublished(dim) }},
	}
	for _, ix := range indexes {
		for _, c := range []struct {
			searchers int
			pace      time.Duration // between two Adds; negative: no Adds
		}{{4, -1}, {4, 35 * time.Millisecond}, {4, 0}, {0, 0}} {
			idx := ix.new(base[0].Dim())
			for i, v := range base {
				if err := idx.Add(indextest.Chunk(t, i), v); err != nil {
					t.Fatal(err)
				}
			}
			adds := map[time.Duration]string{-1: "no adds", 0: "adds as fast as possible"}[c.pace]
			if adds == "" {
				adds = fmt.Sprint("an add every ", c.pace)
			}
			s, a := searchWhileAdding(t, idx, extra, len(base), c.searchers, c.pace)
			t.Logf("%-9s %d searchers, %-24s searches %5d  p50 %-8v p99 %-8v max %-8v | adds %7d  p50 %-8v p99 %-8v max %v",
				ix.name, c.searchers, adds, len(s), pct(s, 50), pct(s, 99), pct(s, 100), len(a), pct(a, 50), pct(a, 99), pct(a, 100))
		}
	}
}

// searchWhileAdding searches idx for qs from searchers goroutines and adds the vectors qs,
// as chunks numbered from next, from one goroutine with a pause between two Adds, for 5 s,
// and returns how long each search and each Add took. A negative pause adds nothing, and
// the Adds stop at 200,000 (300 MB of vectors).
func searchWhileAdding(t *testing.T, idx search.VectorIndex, qs []shared.Vector, next, searchers int, pause time.Duration) (searches, adds []time.Duration) {
	var stop atomic.Bool
	var wg sync.WaitGroup
	took := make([][]time.Duration, searchers)
	for g := range searchers {
		wg.Go(func() {
			for i := g; !stop.Load(); i++ {
				start := time.Now()
				if _, err := idx.Search(t.Context(), qs[i%len(qs)], 10); err != nil {
					t.Error(err)
					return
				}
				took[g] = append(took[g], time.Since(start))
			}
		})
	}
	if pause >= 0 {
		wg.Go(func() {
			for i := 0; i < 200_000 && !stop.Load(); i++ {
				start := time.Now()
				if err := idx.Add(indextest.Chunk(t, next+i), qs[i%len(qs)]); err != nil {
					t.Error(err)
					return
				}
				adds = append(adds, time.Since(start))
				time.Sleep(pause)
			}
		})
	}
	time.Sleep(5 * time.Second)
	stop.Store(true)
	wg.Wait()
	return slices.Concat(took...), adds
}

// pct returns the p-th percentile of d, the largest for 100, rounded to the microsecond.
func pct(d []time.Duration, p int) time.Duration {
	if len(d) == 0 {
		return 0
	}
	s := slices.Clone(d)
	slices.Sort(s)
	return s[min(len(s)-1, len(s)*p/100)].Round(time.Microsecond)
}
