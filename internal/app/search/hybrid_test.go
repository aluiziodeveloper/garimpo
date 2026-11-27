package search_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/aluiziodeveloper/garimpo/internal/app/ingestion/embeddertest"
	"github.com/aluiziodeveloper/garimpo/internal/app/search"
	"github.com/aluiziodeveloper/garimpo/internal/domain/retrieval"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

func chunk(t *testing.T, n int) shared.ChunkID {
	t.Helper()
	id, err := shared.NewChunkID("doc.go", n, n+1, n+1, fmt.Sprint("chunk ", n))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func ranking(ids ...shared.ChunkID) []retrieval.Hit {
	hits := make([]retrieval.Hit, len(ids))
	for i, id := range ids {
		hits[i] = retrieval.Hit{Chunk: id, Score: float32(len(ids) - i)}
	}
	return hits
}

// fakeSearch is a search of a fake index: it records what it was asked, runs during if set,
// and returns hits or err.
type fakeSearch struct {
	hits   []retrieval.Hit
	err    error
	during func(ctx context.Context) error

	mu      sync.Mutex
	queries []any
	ks      []int
}

func (f *fakeSearch) search(ctx context.Context, q any, k int) ([]retrieval.Hit, error) {
	f.mu.Lock()
	f.queries, f.ks = append(f.queries, q), append(f.ks, k)
	f.mu.Unlock()
	if f.during != nil {
		if err := f.during(ctx); err != nil {
			return nil, err
		}
	}
	if f.err != nil {
		return nil, f.err
	}
	return f.hits[:min(k, len(f.hits))], nil
}

type fakeVectors struct{ fakeSearch }

func (f *fakeVectors) Add(shared.ChunkID, shared.Vector) error { return nil }
func (f *fakeVectors) Len() int                                { return len(f.hits) }
func (f *fakeVectors) Search(ctx context.Context, q shared.Vector, k int) ([]retrieval.Hit, error) {
	return f.search(ctx, q, k)
}

type fakeKeywords struct{ fakeSearch }

func (f *fakeKeywords) Add(shared.ChunkID, string) error { return nil }
func (f *fakeKeywords) Len() int                         { return len(f.hits) }
func (f *fakeKeywords) Search(ctx context.Context, q string, k int) ([]retrieval.Hit, error) {
	return f.search(ctx, q, k)
}

// The hits are the two rankings fused by RRF: what both found comes first.
func TestHybridFusesTheTwoRankings(t *testing.T) {
	a, b, c := chunk(t, 1), chunk(t, 2), chunk(t, 3)
	vectors, keywords := &fakeVectors{fakeSearch{hits: ranking(a, b)}}, &fakeKeywords{fakeSearch{hits: ranking(c, a)}}
	h := search.Hybrid{Embedder: &embeddertest.Fake{}, Vectors: vectors, Keywords: keywords}
	got, err := h.Search(t.Context(), "how to sort a slice", 3)
	if err != nil {
		t.Fatal(err)
	}
	if want := retrieval.Fuse(3, ranking(a, b), ranking(c, a)); !slices.Equal(got, want) {
		t.Errorf("Search = %v, want %v", got, want)
	}
}

// The vector index gets the vector of the query, the keyword index the query itself, and
// both are asked for Depth hits: RRF needs more than the k it returns to find the chunks
// both rankings hold.
func TestHybridAsksEachIndexForDepthHits(t *testing.T) {
	emb := &embeddertest.Fake{}
	vectors, keywords := &fakeVectors{}, &fakeKeywords{}
	h := search.Hybrid{Embedder: emb, Vectors: vectors, Keywords: keywords, Depth: 40}
	if _, err := h.Search(t.Context(), "how to sort a slice", 5); err != nil {
		t.Fatal(err)
	}
	want, err := emb.Embed(t.Context(), []string{"how to sort a slice"})
	if err != nil {
		t.Fatal(err)
	}
	if got := emb.Calls(); len(got) != 2 || !slices.Equal(got[0], []string{"how to sort a slice"}) {
		t.Errorf("the embedder got %q, want the query once", got[:1])
	}
	if len(vectors.queries) != 1 || vectors.queries[0].(shared.Vector).Dot(want[0]) < 0.9999 || !slices.Equal(vectors.ks, []int{40}) {
		t.Errorf("the vector index got %v for k %v, want the query's vector for 40", vectors.queries, vectors.ks)
	}
	if !slices.Equal(keywords.queries, []any{"how to sort a slice"}) || !slices.Equal(keywords.ks, []int{40}) {
		t.Errorf("the keyword index got %v for k %v, want the query for 40", keywords.queries, keywords.ks)
	}

	h.Depth = 0 // the default
	if _, err := h.Search(t.Context(), "q", 100); err != nil {
		t.Fatal(err)
	}
	if got := keywords.ks[1]; got != 100 {
		t.Errorf("with k 100 over the default depth, the keyword index was asked for %d, want 100", got)
	}
}

// The two searches run at once: each fake waits for the other to start, which a search one
// after the other never does.
func TestHybridRunsBothSearchesAtOnce(t *testing.T) {
	var started sync.WaitGroup
	started.Add(2)
	meet := func(context.Context) error {
		started.Done()
		done := make(chan struct{})
		go func() { started.Wait(); close(done) }()
		select {
		case <-done:
			return nil
		case <-time.After(2 * time.Second):
			return errors.New("the other search never started: they ran one after the other")
		}
	}
	h := search.Hybrid{
		Embedder: &embeddertest.Fake{},
		Vectors:  &fakeVectors{fakeSearch{during: meet}},
		Keywords: &fakeKeywords{fakeSearch{during: meet}},
	}
	if _, err := h.Search(t.Context(), "q", 5); err != nil {
		t.Error(err)
	}
}

// When one search fails, the other is canceled, and Search returns the failure: it does not
// wait for a result it will not use.
func TestAFailedSearchCancelsTheOther(t *testing.T) {
	boom := errors.New("boom")
	var sawCancel bool
	vectorStarted := make(chan struct{})
	h := search.Hybrid{
		Embedder: &embeddertest.Fake{},
		Vectors: &fakeVectors{fakeSearch{during: func(ctx context.Context) error {
			close(vectorStarted)
			<-ctx.Done() // would block forever if nobody canceled
			sawCancel = true
			return ctx.Err()
		}}},
		// The keyword search fails once the vector search is under way, not before it starts.
		Keywords: &fakeKeywords{fakeSearch{err: boom, during: func(context.Context) error {
			<-vectorStarted
			return nil
		}}},
	}
	if _, err := h.Search(t.Context(), "q", 5); !errors.Is(err, boom) {
		t.Errorf("Search: err = %v, want one that wraps %v", err, boom)
	}
	if !sawCancel {
		t.Error("the vector search did not see its context canceled")
	}
}

func TestHybridRefusesWhatItCannotDo(t *testing.T) {
	h := search.Hybrid{Embedder: &embeddertest.Fake{}, Vectors: &fakeVectors{}, Keywords: &fakeKeywords{}}
	if _, err := h.Search(t.Context(), "q", 0); !errors.Is(err, search.ErrInvalidK) {
		t.Errorf("Search with k 0: err = %v, want %v", err, search.ErrInvalidK)
	}
	long := search.Hybrid{Embedder: &embeddertest.Fake{Context: 3}, Vectors: &fakeVectors{}, Keywords: &fakeKeywords{}}
	if _, err := long.Search(t.Context(), "a query too long", 5); err == nil {
		t.Error("Search with a query the embedder refuses: err = nil, want the embedder's error")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := h.Search(ctx, "q", 5); !errors.Is(err, context.Canceled) {
		t.Errorf("Search with a done context: err = %v, want %v", err, context.Canceled)
	}
}
