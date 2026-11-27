package search_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/aluiziodeveloper/garimpo/internal/app/search"
	"github.com/aluiziodeveloper/garimpo/internal/domain/retrieval"
)

const took = 50 * time.Millisecond

// slowRanker takes took to answer, or less if its context ends first, and counts its calls.
// Its hits say the query and k it was asked for. err, when set, is its answer.
type slowRanker struct {
	took  time.Duration
	err   error
	calls atomic.Int64
}

func (s *slowRanker) Search(ctx context.Context, q string, k int) ([]retrieval.Hit, error) {
	s.calls.Add(1)
	select {
	case <-time.After(s.took):
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	if s.err != nil {
		return nil, s.err
	}
	return []retrieval.Hit{{Score: float32(k)}, {Score: float32(len(q))}}, nil
}

// at asks c the query q with k from n goroutines at once and returns, once they all
// answered, the error of each.
func at(t *testing.T, c search.Ranker, n int, q string, k int) []error {
	t.Helper()
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Go(func() {
			hits, err := c.Search(t.Context(), q, k)
			if err == nil && (len(hits) != 2 || hits[0].Score != float32(k) || hits[1].Score != float32(len(q))) {
				err = fmt.Errorf("hits %v, not the ones of %q with k %d", hits, q, k)
			}
			errs[i] = err
		})
	}
	wg.Wait()
	return errs
}

var _ search.Ranker = (*search.Collapsed)(nil)

// Equal searches at once run one search, and each gets its hits (INC-16).
func TestEqualSearchesAtOnceRunOneSearch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := &slowRanker{took: took}
		start := time.Now()
		if err := errors.Join(at(t, search.Collapse(r, time.Second), 100, "sort a slice", 10)...); err != nil {
			t.Fatal(err)
		}
		if n := r.calls.Load(); n != 1 {
			t.Errorf("%d searches for 100 equal ones at once, want 1", n)
		}
		if d := time.Since(start); d != took {
			t.Errorf("they took %v, want one search, %v", d, took)
		}
	})
}

// A search is the query and k: another query, or the same with another k, is another search.
func TestOtherQueriesOrKsAreOtherSearches(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := &slowRanker{took: took}
		c := search.Collapse(r, time.Second)
		var wg sync.WaitGroup
		for _, s := range []struct {
			q string
			k int
		}{{"sort a slice", 10}, {"sort a slice", 3}, {"parse a duration", 10}, {"sort a slic", 10}} {
			wg.Go(func() {
				if err := errors.Join(at(t, c, 10, s.q, s.k)...); err != nil {
					t.Error(err)
				}
			})
		}
		wg.Wait()
		if n := r.calls.Load(); n != 4 {
			t.Errorf("%d searches for 4 different ones, 10 times each, want 4", n)
		}
	})
}

// A caller that gives up leaves alone: the search goes on for the others, who get its hits.
func TestACallerThatGivesUpLeavesAlone(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := &slowRanker{took: took}
		c := search.Collapse(r, time.Second)
		ctx, cancel := context.WithCancel(t.Context())
		first := make(chan error, 1)
		go func() { _, err := c.Search(ctx, "q", 1); first <- err }()
		synctest.Wait() // the first caller started the search
		others := make(chan []error, 1)
		go func() { others <- at(t, c, 9, "q", 1) }()
		synctest.Wait()
		time.Sleep(took / 5)
		cancel()
		if err := <-first; !errors.Is(err, context.Canceled) {
			t.Errorf("the caller that gave up: err = %v, want context.Canceled", err)
		}
		if err := errors.Join(<-others...); err != nil {
			t.Errorf("the others: %v, want their hits", err)
		}
		if n := r.calls.Load(); n != 1 {
			t.Errorf("%d searches, want 1", n)
		}
	})
}

// When every caller gave up, the search still ends at the deadline of Collapse: it does not
// run for nobody past it.
func TestTheSearchEndsAtItsOwnDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := &slowRanker{took: time.Hour}
		c := search.Collapse(r, 2*took)
		ctx, cancel := context.WithTimeout(t.Context(), took)
		defer cancel()
		start := time.Now()
		if _, err := c.Search(ctx, "q", 1); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v, want the caller's deadline", err)
		}
		if d := time.Since(start); d != took {
			t.Errorf("the caller got its error after %v, want its deadline, %v", d, took)
		}
		synctest.Wait() // the search, alone, waits for its deadline
		// A caller now joins the search under way, and gets its end: the deadline of the
		// search, 2*took from the start, not an hour.
		_, err := c.Search(t.Context(), "q", 1)
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(start) != 2*took {
			t.Errorf("the search under way ended after %v with %v, want its deadline, %v", time.Since(start), err, 2*took)
		}
		if n := r.calls.Load(); n != 1 {
			t.Errorf("%d searches, want 1: the second caller joined the first search", n)
		}
	})
}

// The error of the search is the error of every caller that shared it.
func TestTheErrorOfTheSearchGoesToEveryCaller(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		down := errors.New("model down")
		r := &slowRanker{took: took, err: down}
		for i, err := range at(t, search.Collapse(r, time.Second), 10, "q", 1) {
			if !errors.Is(err, down) {
				t.Errorf("caller %d: err = %v, want %v", i, err, down)
			}
		}
		if n := r.calls.Load(); n != 1 {
			t.Errorf("%d searches, want 1", n)
		}
	})
}

// Collapse is not a cache: a search after the last one ended runs again.
func TestCollapseIsNotACache(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := &slowRanker{took: took}
		c := search.Collapse(r, time.Second)
		for range 3 {
			if err := errors.Join(at(t, c, 1, "q", 1)...); err != nil {
				t.Fatal(err)
			}
		}
		if n := r.calls.Load(); n != 3 {
			t.Errorf("%d searches for 3 one after the other, want 3", n)
		}
	})
}

func TestCollapseNeedsADeadline(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Collapse with a deadline of 0 did not panic")
		}
	}()
	search.Collapse(&slowRanker{}, 0)
}
