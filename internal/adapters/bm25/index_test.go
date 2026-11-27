package bm25_test

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"sync"
	"testing"

	"github.com/aluiziodeveloper/garimpo/internal/adapters/bm25"
	"github.com/aluiziodeveloper/garimpo/internal/app/search"
	"github.com/aluiziodeveloper/garimpo/internal/domain/retrieval"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

var _ search.KeywordIndex = (*bm25.Index)(nil)

// chunk returns the ID of a chunk numbered n.
func chunk(t testing.TB, n int) shared.ChunkID {
	t.Helper()
	id, err := shared.NewChunkID("doc.go", n, n+1, n+1, fmt.Sprint("chunk ", n))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// filled returns an index with texts[i] as the text of chunk i.
func filled(t *testing.T, texts ...string) *bm25.Index {
	t.Helper()
	x := bm25.New()
	for i, text := range texts {
		if err := x.Add(chunk(t, i), text); err != nil {
			t.Fatal(err)
		}
	}
	return x
}

func find(t *testing.T, x *bm25.Index, query string, k int) []retrieval.Hit {
	t.Helper()
	hits, err := x.Search(t.Context(), query, k)
	if err != nil {
		t.Fatal(err)
	}
	return hits
}

// chunks returns the numbers of the chunks of hits, in order, out of n chunks.
func chunks(t *testing.T, hits []retrieval.Hit, n int) []int {
	t.Helper()
	var got []int
	for _, h := range hits {
		i := -1
		for j := range n {
			if chunk(t, j) == h.Chunk {
				i = j
			}
		}
		got = append(got, i)
	}
	return got
}

// The terms of a text are its runs of letters and digits, in lower case; a run of one
// character is not a term, even one of two bytes.
func TestTermsAreTheWordsInLowerCase(t *testing.T) {
	got := slices.Collect(bm25.Terms("func (b *Buffer) WriteString(s string) (n int, err error) // é x2"))
	want := []string{"func", "buffer", "writestring", "string", "int", "err", "error", "x2"}
	if !slices.Equal(got, want) {
		t.Errorf("Terms = %q, want %q", got, want)
	}
}

// Only the chunks that hold a term of the query are hits, best first.
func TestSearchFindsTheChunksWithTheTerms(t *testing.T) {
	x := filled(t,
		"func Marshal(v any) ([]byte, error)",
		"func Open(name string) (*File, error)",
		"// Marshal returns the JSON encoding of v. Marshal traverses v recursively.",
	)
	if got := chunks(t, find(t, x, "marshal", 10), 3); !slices.Equal(got, []int{2, 0}) {
		t.Errorf("chunks for marshal = %v, want [2 0]: chunk 2 has it twice", got)
	}
	if got := find(t, x, "goroutine", 10); len(got) != 0 {
		t.Errorf("hits for a term no chunk has = %v, want none", got)
	}
}

// A term few chunks have tells more than one most have: its weight, the IDF, is higher.
func TestARareTermWeighsMoreThanACommonOne(t *testing.T) {
	x := filled(t, "error rare", "error common", "error common", "error common")
	hits := find(t, x, "error rare common", 10)
	if got := chunks(t, hits[:1], 4); !slices.Equal(got, []int{0}) {
		t.Errorf("first hit is chunk %v, want 0, the one with the rare term", got)
	}
}

// A term repeated adds less and less: ten times the term is far from ten times the score.
func TestRepeatedTermsSaturate(t *testing.T) {
	x := filled(t, "lock", "lock lock lock lock lock lock lock lock lock lock", "other", "other")
	hits := find(t, x, "lock", 10)
	if len(hits) != 2 {
		t.Fatalf("%d hits, want 2", len(hits))
	}
	if ratio := hits[0].Score / hits[1].Score; ratio >= 2 {
		t.Errorf("ten times the term scores %.2f times one, want less than 2", ratio)
	}
}

// With the same count of a term, the shorter chunk ranks first: its length is normalized.
func TestTheShorterChunkRanksFirst(t *testing.T) {
	x := filled(t, "mutex unlock waiter starving spinning handoff fairness", "mutex", "other", "other")
	if got := chunks(t, find(t, x, "mutex", 10), 4); !slices.Equal(got, []int{1, 0}) {
		t.Errorf("chunks = %v, want [1 0]", got)
	}
}

// The score is BM25 with k1 = 1.2 and b = 0.75, computed here from its formula.
func TestTheScoreIsBM25(t *testing.T) {
	texts := []string{"alpha beta beta", "beta gamma", "gamma gamma gamma delta"}
	x := filled(t, texts...)
	const k1, b = 1.2, 0.75
	n, avgdl := 3.0, (3.0+2+4)/3
	idf := func(df float64) float64 { return math.Log(1 + (n-df+0.5)/(df+0.5)) }
	part := func(tf, dl float64) float64 { return tf * (k1 + 1) / (tf + k1*(1-b+b*dl/avgdl)) }
	want := map[int]float64{
		0: idf(2) * part(2, 3),                   // beta, twice in a chunk of 3 terms
		1: idf(2)*part(1, 2) + idf(2)*part(1, 2), // beta and gamma, once each in 2
		2: idf(2) * part(3, 4),                   // gamma, three times in 4
	}
	for _, h := range find(t, x, "beta gamma", 10) {
		i := chunks(t, []retrieval.Hit{h}, 3)[0]
		if math.Abs(float64(h.Score)-want[i]) > 1e-5 {
			t.Errorf("chunk %d scores %v, want %v", i, h.Score, want[i])
		}
	}
}

// Equal scores are ranked by ChunkID, as in every index (retrieval.Compare), and k bounds
// the hits.
func TestSearchKeepsTheKBestInOrder(t *testing.T) {
	x := filled(t, "same text", "same text", "same text", "other")
	hits := find(t, x, "same", 2)
	if len(hits) != 2 {
		t.Fatalf("%d hits, want 2", len(hits))
	}
	if !slices.IsSortedFunc(hits, retrieval.Compare) {
		t.Errorf("hits %v are not in the order of retrieval.Compare", hits)
	}
}

func TestAddAndSearchRefuseWhatTheyCannotDo(t *testing.T) {
	x := filled(t, "func main")
	if err := x.Add(shared.ChunkID{}, "text"); !errors.Is(err, search.ErrZeroChunk) {
		t.Errorf("Add of the zero chunk: err = %v, want %v", err, search.ErrZeroChunk)
	}
	if err := x.Add(chunk(t, 0), "again"); !errors.Is(err, search.ErrDuplicateChunk) {
		t.Errorf("Add of a chunk already held: err = %v, want %v", err, search.ErrDuplicateChunk)
	}
	if _, err := x.Search(t.Context(), "main", 0); !errors.Is(err, search.ErrInvalidK) {
		t.Errorf("Search with k 0: err = %v, want %v", err, search.ErrInvalidK)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := x.Search(ctx, "main", 1); !errors.Is(err, context.Canceled) {
		t.Errorf("Search with a done context: err = %v, want %v", err, context.Canceled)
	}
	if hits := find(t, x, "  // ", 3); hits == nil || len(hits) != 0 {
		t.Errorf("Search of a query with no term = %v, want an empty slice", hits)
	}
	if x.Len() != 1 {
		t.Errorf("Len = %d after the refused Add, want 1", x.Len())
	}
}

func TestAddAndSearchAreSafeForConcurrentCalls(t *testing.T) {
	x := bm25.New()
	var wg sync.WaitGroup
	for g := range 4 {
		wg.Go(func() {
			for i := range 100 {
				if err := x.Add(chunk(t, g*100+i), fmt.Sprint("func f", i, " returns error ", g)); err != nil {
					t.Error(err)
					return
				}
				if _, err := x.Search(t.Context(), "error returns", 5); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	wg.Wait()
	if x.Len() != 400 {
		t.Errorf("Len = %d, want 400", x.Len())
	}
}
