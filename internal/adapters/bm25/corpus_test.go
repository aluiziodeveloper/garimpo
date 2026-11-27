package bm25_test

import (
	"math/rand/v2"
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/aluiziodeveloper/garimpo/internal/adapters/bm25"
	"github.com/aluiziodeveloper/garimpo/internal/adapters/corpus"
	"github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
)

// TestIndexOnTheCorpus measures the index on the corpus in the directory GARIMPO_CORPUS
// (the Go library, /usr/local/go/src in the container); without it, it skips. It logs how
// long the index takes to build, the heap it keeps after a GC, and the time and the bytes
// allocated per search of 1,000 queries of 3 terms, each drawn from a chunk of the corpus.
//
//	GARIMPO_CORPUS=/usr/local/go/src go test -run TestIndexOnTheCorpus -v ./internal/adapters/bm25
func TestIndexOnTheCorpus(t *testing.T) {
	dir := os.Getenv("GARIMPO_CORPUS")
	if dir == "" {
		t.Skip("set GARIMPO_CORPUS to the corpus directory")
	}
	fsys := os.DirFS(dir)
	r := rand.New(rand.NewPCG(15, 3))
	var queries []string
	base := heap()
	x := bm25.New()
	n, took := 0, time.Duration(0)
	for d, err := range corpus.Walk(fsys) {
		if err != nil {
			t.Fatal(err)
		}
		for c := range (ingestion.Cutter{}).Cut(d) {
			start := time.Now()
			if err := x.Add(c.ID(), c.Text()); err != nil {
				t.Fatal(err)
			}
			took += time.Since(start)
			n++
			if terms := slices.Collect(bm25.Terms(c.Text())); len(terms) >= 3 && r.IntN(40) == 0 {
				q := make([]string, 3)
				for i := range q {
					q[i] = strings.Clone(terms[r.IntN(len(terms))])
				}
				queries = append(queries, strings.Join(q, " "))
			}
		}
	}
	queries = queries[:min(len(queries), 1000)]
	t.Logf("%d chunks indexed in %v of Add; heap after a GC: %.1f MiB", n, took.Round(time.Millisecond), mib(heap()-base))

	var times []time.Duration
	hits := 0
	allocs := totalAlloc()
	for _, q := range queries {
		start := time.Now()
		got, err := x.Search(t.Context(), q, 10)
		if err != nil {
			t.Fatal(err)
		}
		times = append(times, time.Since(start))
		hits += len(got)
	}
	slices.Sort(times)
	t.Logf("%d searches of 3 terms: p50 %v, p99 %v, %.1f KiB allocated per search, %.1f hits each",
		len(queries), times[len(times)/2].Round(time.Microsecond), times[len(times)*99/100].Round(time.Microsecond),
		float64(totalAlloc()-allocs)/float64(len(queries))/1024, float64(hits)/float64(len(queries)))
	runtime.KeepAlive(x)
}

// heap returns the bytes of the heap in use after a full GC.
func heap() uint64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

// totalAlloc returns the bytes allocated so far, freed or not.
func totalAlloc() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.TotalAlloc
}

func mib(b uint64) float64 { return float64(b) / (1 << 20) }
