package search_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/aluiziodeveloper/garimpo/internal/app/search"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// slowEmbedder takes took to answer, counts its calls and gives every text the vector [1].
type slowEmbedder struct {
	took  time.Duration
	calls atomic.Int64
}

func (s *slowEmbedder) Embed(ctx context.Context, texts []string) ([]shared.Vector, error) {
	s.calls.Add(1)
	select {
	case <-time.After(s.took):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	v, err := shared.NewVector([]float32{1}, 1)
	vs := make([]shared.Vector, len(texts))
	for i := range vs {
		vs[i] = v
	}
	return vs, err
}

// The same query with other ks is another search for Collapse, but the same text for the
// model: CollapseEmbedder asks the model once.
func TestTheSameQueryWithOtherKsEmbedsOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := &slowEmbedder{took: took}
		c := search.Collapse(&search.Hybrid{
			Embedder: search.CollapseEmbedder(e, time.Second),
			Vectors:  new(fakeVectors),
			Keywords: new(fakeKeywords),
		}, time.Second)
		var wg sync.WaitGroup
		for k := 1; k <= search.MaxK; k++ {
			wg.Go(func() {
				if _, err := c.Search(t.Context(), "sort a slice", k); err != nil {
					t.Error(err)
				}
			})
		}
		wg.Wait()
		if n := e.calls.Load(); n != 1 {
			t.Errorf("%d calls to the model for one query with %d ks, want 1", n, search.MaxK)
		}
	})
}

// Two lists of texts are the same call only when they are the same texts: "a b" is not
// "a" and "b".
func TestCollapseEmbedderTellsTheTextsApart(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		e := &slowEmbedder{took: took}
		c := search.CollapseEmbedder(e, time.Second)
		var wg sync.WaitGroup
		for _, texts := range [][]string{{"a b"}, {"a", "b"}, {"a b"}, {"ab"}} {
			wg.Go(func() {
				vs, err := c.Embed(t.Context(), texts)
				if err != nil || len(vs) != len(texts) {
					t.Errorf("Embed(%q): %d vectors, err %v", texts, len(vs), err)
				}
			})
		}
		wg.Wait()
		if n := e.calls.Load(); n != 3 {
			t.Errorf("%d calls for 3 different lists of texts, want 3", n)
		}
	})
}
