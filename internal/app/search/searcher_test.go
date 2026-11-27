package search_test

import (
	"context"
	"errors"
	"testing"

	"github.com/aluiziodeveloper/garimpo/internal/app/search"
	"github.com/aluiziodeveloper/garimpo/internal/domain/retrieval"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// fakeRanker returns hits, or err, and records the queries it got.
type fakeRanker struct {
	hits    []retrieval.Hit
	err     error
	queries []string
	ks      []int
}

func (f *fakeRanker) Search(_ context.Context, q string, k int) ([]retrieval.Hit, error) {
	f.queries, f.ks = append(f.queries, q), append(f.ks, k)
	return f.hits, f.err
}

// fakeTexts holds chunks numbered n, at doc.go lines n+1 to n+1, with text "chunk n".
type fakeTexts struct {
	known map[shared.ChunkID]int
	err   error
}

func newFakeTexts(t *testing.T, ns ...int) *fakeTexts {
	f := &fakeTexts{known: map[shared.ChunkID]int{}}
	for _, n := range ns {
		f.known[chunk(t, n)] = n
	}
	return f
}

func (f *fakeTexts) FindChunk(id shared.ChunkID) (string, int, int, bool) {
	n, ok := f.known[id]
	return "doc.go", n + 1, n + 1, ok
}

func (f *fakeTexts) Text(id shared.ChunkID) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return "chunk text of " + id.String()[:8], nil
}

func query(t *testing.T, s string) retrieval.Query {
	t.Helper()
	q, err := retrieval.NewQuery(s)
	if err != nil {
		t.Fatal(err)
	}
	return q
}

// Each hit becomes a result, in the order of the ranking: a citation of the chunk's lines,
// its text and its score.
func TestSearcherCitesEachHitWithItsText(t *testing.T) {
	a, b := chunk(t, 1), chunk(t, 2)
	ranker := &fakeRanker{hits: []retrieval.Hit{{Chunk: b, Score: 0.9}, {Chunk: a, Score: 0.4}}}
	s := search.Searcher{Ranker: ranker, Texts: newFakeTexts(t, 1, 2)}
	got, err := s.Search(t.Context(), query(t, "  sort a slice "), 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(ranker.queries) != 1 || ranker.queries[0] != "sort a slice" || ranker.ks[0] != 5 {
		t.Errorf("the ranker got %q for k %v, want \"sort a slice\" for 5", ranker.queries, ranker.ks)
	}
	if len(got) != 2 {
		t.Fatalf("%d results, want 2", len(got))
	}
	for i, want := range []struct {
		id    shared.ChunkID
		cites string
		score float32
	}{{b, "doc.go:3", 0.9}, {a, "doc.go:2", 0.4}} {
		r := got[i]
		if r.Citation.Chunk() != want.id || r.Citation.String() != want.cites || r.Score != want.score {
			t.Errorf("result %d: %s %v, want %s %v", i, r.Citation, r.Score, want.cites, want.score)
		}
		if r.Text != "chunk text of "+want.id.String()[:8] {
			t.Errorf("result %d: text %q", i, r.Text)
		}
	}
}

// A hit of a chunk the corpus does not have is a bug between the indexes and the texts:
// the search fails instead of returning a citation of nothing (RN01).
func TestSearcherRefusesAHitItCannotCite(t *testing.T) {
	s := search.Searcher{Ranker: &fakeRanker{hits: []retrieval.Hit{{Chunk: chunk(t, 7), Score: 1}}}, Texts: newFakeTexts(t, 1)}
	if _, err := s.Search(t.Context(), query(t, "q"), 5); !errors.Is(err, shared.ErrInvalidCitation) {
		t.Errorf("Search: err = %v, want one that wraps %v", err, shared.ErrInvalidCitation)
	}
}

func TestSearcherRefusesWhatItCannotDo(t *testing.T) {
	boom := errors.New("boom")
	s := search.Searcher{Ranker: &fakeRanker{}, Texts: newFakeTexts(t)}
	for _, k := range []int{0, search.MaxK + 1} {
		if _, err := s.Search(t.Context(), query(t, "q"), k); !errors.Is(err, search.ErrInvalidK) {
			t.Errorf("Search with k %d: err = %v, want %v", k, err, search.ErrInvalidK)
		}
	}
	if _, err := s.Search(t.Context(), retrieval.Query{}, 5); !errors.Is(err, retrieval.ErrEmptyQuery) {
		t.Errorf("Search of the zero Query: err = %v, want %v", err, retrieval.ErrEmptyQuery)
	}
	failing := search.Searcher{Ranker: &fakeRanker{err: boom}, Texts: newFakeTexts(t)}
	if _, err := failing.Search(t.Context(), query(t, "q"), 5); !errors.Is(err, boom) {
		t.Errorf("Search with a failing ranker: err = %v, want %v", err, boom)
	}
	noText := search.Searcher{Ranker: &fakeRanker{hits: []retrieval.Hit{{Chunk: chunk(t, 1), Score: 1}}}, Texts: &fakeTexts{known: map[shared.ChunkID]int{chunk(t, 1): 1}, err: boom}}
	if _, err := noText.Search(t.Context(), query(t, "q"), 5); !errors.Is(err, boom) {
		t.Errorf("Search with a text that cannot be read: err = %v, want %v", err, boom)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.Search(ctx, query(t, "q"), 5); !errors.Is(err, context.Canceled) {
		t.Errorf("Search with a done context: err = %v, want %v", err, context.Canceled)
	}
}
