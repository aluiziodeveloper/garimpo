package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

const took = 50 * time.Millisecond

// questions is how many equal questions each test asks at once.
const questions = 100

// equal asks q questions times at once through s, in a synctest bubble: the first question alone,
// and the others once it waits for its search. It returns cancelFirst, which cancels the
// first question, and wait, which waits for all of them and returns the error of each, the
// first one first.
func equal(t *testing.T, s searcher) (cancelFirst context.CancelFunc, wait func() []error) {
	t.Helper()
	first, cancelFirst := context.WithCancel(t.Context())
	errs := make([]error, questions)
	var wg sync.WaitGroup
	wg.Go(func() { _, errs[0] = s.Search(first, "q") })
	synctest.Wait() // the first question is waiting for its search
	for i := 1; i < questions; i++ {
		wg.Go(func() { _, errs[i] = s.Search(context.Background(), "q") })
	}
	synctest.Wait() // every question is waiting
	return cancelFirst, func() []error {
		wg.Wait()
		return errs
	}
}

func TestPlainRunsASearchPerQuestion(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := &slow{took: took}
		_, wait := equal(t, &plain{s: s})
		errs := wait()
		if got := s.calls.Load(); got != questions {
			t.Errorf("%d searches for %d equal questions, want %d", got, questions, questions)
		}
		if err := errors.Join(errs...); err != nil {
			t.Error(err)
		}
	})
}

// Do runs one search for the questions that come while it runs, and gives each the result.
func TestDoRunsOneSearchForEqualQuestions(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := &slow{took: took}
		start := time.Now()
		_, wait := equal(t, &do{s: s})
		errs := wait()
		if got := s.calls.Load(); got != 1 {
			t.Errorf("%d searches for %d equal questions, want 1", got, questions)
		}
		if err := errors.Join(errs...); err != nil {
			t.Error(err)
		}
		if d := time.Since(start); d != took {
			t.Errorf("the questions took %v, want the one search, %v", d, took)
		}
	})
}

// The trap of Do: the search runs with the context of the first question. When that one is
// canceled, every question that waits for the search fails with its error, though nobody
// canceled them.
func TestDoGivesTheCancelOfTheFirstQuestionToAll(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := &slow{took: took}
		cancel, wait := equal(t, &do{s: s})
		time.Sleep(took / 5)
		cancel()
		failed := 0
		for _, err := range wait() {
			if errors.Is(err, context.Canceled) {
				failed++
			}
		}
		if failed != questions {
			t.Errorf("%d of the %d questions failed with context.Canceled, want all: the first one's cancel reached them", failed, questions)
		}
	})
}

// detached runs one search with a context of its own: the first question leaves alone, and
// the others get the result.
func TestDetachedLetsAQuestionLeaveAlone(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := &slow{took: took}
		cancel, wait := equal(t, &detached{s: s, timeout: 10 * took})
		time.Sleep(took / 5)
		cancel()
		got := wait()
		if !errors.Is(got[0], context.Canceled) {
			t.Errorf("the first question: err = %v, want context.Canceled", got[0])
		}
		if err := errors.Join(got[1:]...); err != nil {
			t.Errorf("the others: %v, want none", err)
		}
		if n := s.calls.Load(); n != 1 {
			t.Errorf("%d searches, want 1", n)
		}
	})
}

// The search of detached has its own deadline: when every question has left, it still runs,
// but not past timeout.
func TestDetachedEndsTheSearchAtItsDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := &slow{took: time.Hour}
		ctx, cancel := context.WithTimeout(t.Context(), took)
		defer cancel()
		x := &detached{s: s, timeout: 2 * took}
		start := time.Now()
		if _, err := x.Search(ctx, "q"); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v, want the question's deadline", err)
		}
		ch := x.g.DoChan("q", func() (any, error) { return nil, errors.New("a second search") })
		r := <-ch // waits for the search under way: it ends at its own deadline
		if !errors.Is(r.Err, context.DeadlineExceeded) || !r.Shared {
			t.Errorf("the search under way ended with %v (shared %t), want its deadline", r.Err, r.Shared)
		}
		if d := time.Since(start); d != 2*took {
			t.Errorf("the search ended after %v, want its timeout, %v", d, 2*took)
		}
	})
}

// singleflight is not a cache: a question that comes after the search ended runs a new one.
func TestAQuestionAfterTheSearchRunsANewOne(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := &slow{took: took}
		x := &do{s: s}
		for range 3 {
			if _, err := x.Search(t.Context(), "q"); err != nil {
				t.Fatal(err)
			}
		}
		if n := s.calls.Load(); n != 3 {
			t.Errorf("%d searches for 3 questions one after the other, want 3", n)
		}
	})
}

// The questions that share a search share its result: the same slice. A caller that writes
// to it writes to everyone's.
func TestEqualQuestionsShareTheSameSlice(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		x := &do{s: &slow{took: took}}
		var a, b []string
		var wg sync.WaitGroup
		wg.Go(func() { a, _ = x.Search(t.Context(), "q") })
		synctest.Wait()
		wg.Go(func() { b, _ = x.Search(t.Context(), "q") })
		wg.Wait()
		a[0] = "changed by one caller"
		if b[0] != "changed by one caller" {
			t.Errorf("b[0] = %q: the two callers got different slices", b[0])
		}
	})
}
