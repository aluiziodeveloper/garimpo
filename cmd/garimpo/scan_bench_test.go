package main

import (
	"io/fs"
	"iter"
	"os"
	"runtime"
	"runtime/metrics"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aluiziodeveloper/garimpo/internal/adapters/corpus"
	ingest "github.com/aluiziodeveloper/garimpo/internal/app/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
)

const library = "/usr/local/go/src"

// BenchmarkSurveyCorpus surveys the whole corpus in two ways: streaming, one document at
// a time, as scan does; and loading every document first, as the first ingestion did
// (INC-06). Besides time and allocations, it reports peak-live-B: the largest heap the
// garbage collector found alive during the run.
func BenchmarkSurveyCorpus(b *testing.B) {
	if _, err := os.Stat(library); err != nil {
		b.Skipf("the corpus is not here: %v", err)
	}
	fsys := os.DirFS(library)
	b.Run("mode=stream", func(b *testing.B) {
		survey(b, func() iter.Seq2[ingestion.Document, error] { return corpus.Walk(fsys) })
	})
	b.Run("mode=load", func(b *testing.B) {
		survey(b, func() iter.Seq2[ingestion.Document, error] { return loadAll(b, fsys) })
	})
}

// survey runs SurveyCorpus on the documents docs returns, b.N times, and reports the peak
// live heap.
func survey(b *testing.B, docs func() iter.Seq2[ingestion.Document, error]) {
	b.ReportAllocs()
	runtime.GC()
	stop := watchLiveHeap()
	var s ingest.Survey
	for b.Loop() {
		var err error
		if s, err = ingest.SurveyCorpus(docs(), ingestion.Cutter{}); err != nil {
			b.Fatal(err)
		}
	}
	b.ReportMetric(float64(stop()), "peak-live-B")
	if s.Documents == 0 {
		b.Fatal("the survey found no documents")
	}
}

// loadAll reads every document into a slice before the survey sees the first one.
func loadAll(b *testing.B, fsys fs.FS) iter.Seq2[ingestion.Document, error] {
	var docs []ingestion.Document
	for d, err := range corpus.Walk(fsys) {
		if err != nil {
			b.Fatal(err)
		}
		docs = append(docs, d)
	}
	return func(yield func(ingestion.Document, error) bool) {
		for _, d := range slices.All(docs) {
			if !yield(d, nil) {
				return
			}
		}
	}
}

// watchLiveHeap samples /gc/heap/live:bytes, the heap found alive by the last collection,
// until the returned function is called; that function returns the largest sample.
func watchLiveHeap() func() uint64 {
	var peak atomic.Uint64
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		sample := []metrics.Sample{{Name: "/gc/heap/live:bytes"}}
		tick := time.NewTicker(100 * time.Microsecond)
		defer tick.Stop()
		for {
			metrics.Read(sample)
			if v := sample[0].Value.Uint64(); v > peak.Load() {
				peak.Store(v)
			}
			select {
			case <-done:
				return
			case <-tick.C:
			}
		}
	}()
	return func() uint64 {
		close(done)
		<-finished
		return peak.Load()
	}
}
