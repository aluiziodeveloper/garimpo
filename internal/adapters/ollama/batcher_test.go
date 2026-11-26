package ollama

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/aluiziodeveloper/garimpo/internal/app/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/app/ingestion/embeddertest"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// newBatcher returns a Batcher in front of next, closed when the test ends.
func newBatcher(t *testing.T, next ingestion.Embedder, size int, wait time.Duration) *Batcher {
	t.Helper()
	b, err := NewBatcher(next, size, wait)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(b.Close)
	return b
}

func TestBatcherMeetsTheContract(t *testing.T) {
	embeddertest.Contract(t, func(t *testing.T) ingestion.Embedder {
		return newBatcher(t, &embeddertest.Fake{Context: 64}, 4, time.Millisecond)
	}, strings.Repeat("x", 65))
}

// embedAll calls b.Embed once per text, all at once, and returns when every call did.
func embedAll(t *testing.T, b *Batcher, texts ...string) {
	t.Helper()
	var wg sync.WaitGroup
	for _, text := range texts {
		wg.Go(func() {
			if _, err := b.Embed(t.Context(), []string{text}); err != nil {
				t.Errorf("Embed(%q): %v", text, err)
			}
		})
	}
	wg.Wait()
}

func TestAFullBatchGoesOutAtOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var f embeddertest.Fake
		b := newBatcher(t, &f, 4, time.Second)
		start := time.Now()
		embedAll(t, b, "a", "b", "c", "d")
		if took := time.Since(start); took != 0 {
			t.Errorf("a full batch took %v, want no wait", took)
		}
		if calls := f.Calls(); len(calls) != 1 || len(calls[0]) != 4 {
			t.Fatalf("calls = %q, want one batch of 4", calls)
		}
	})
}

func TestAnIncompleteBatchGoesOutAfterTheWait(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var f embeddertest.Fake
		b := newBatcher(t, &f, 4, 10*time.Millisecond)
		start := time.Now()
		embedAll(t, b, "a", "b")
		if took := time.Since(start); took != 10*time.Millisecond {
			t.Errorf("an incomplete batch took %v, want 10ms", took)
		}
		if calls := f.Calls(); len(calls) != 1 || len(calls[0]) != 2 {
			t.Fatalf("calls = %q, want one batch of 2", calls)
		}
	})
}

func TestTheWaitCountsFromTheFirstText(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var f embeddertest.Fake
		b := newBatcher(t, &f, 4, 10*time.Millisecond)
		start := time.Now()
		var wg sync.WaitGroup
		for _, text := range []string{"first", "second"} {
			wg.Go(func() {
				if _, err := b.Embed(t.Context(), []string{text}); err != nil {
					t.Errorf("Embed(%q): %v", text, err)
				}
				if took := time.Since(start); took != 10*time.Millisecond {
					t.Errorf("%s text answered after %v, want 10ms: the wait starts with the first text", text, took)
				}
			})
			time.Sleep(6 * time.Millisecond) // the second text arrives 6ms after the first
		}
		wg.Wait()
		if calls := f.Calls(); len(calls) != 1 {
			t.Fatalf("calls = %q, want one batch", calls)
		}
	})
}

func TestEachCallerGetsItsOwnVectors(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var f embeddertest.Fake
		b := newBatcher(t, &f, 8, time.Millisecond)
		texts := []string{"a", "b", "c", "d", "e"}
		got := make([]shared.Vector, len(texts))
		var wg sync.WaitGroup
		for i, text := range texts {
			wg.Go(func() {
				vs, err := b.Embed(t.Context(), []string{text})
				if err != nil || len(vs) != 1 {
					t.Errorf("Embed(%q) = %d vectors, %v", text, len(vs), err)
					return
				}
				got[i] = vs[0]
			})
		}
		wg.Wait()
		want, _ := new(embeddertest.Fake).Embed(t.Context(), texts)
		for i := range texts {
			if got[i].Dot(want[i]) < 0.9999 {
				t.Errorf("text %q got another text's vector", texts[i])
			}
		}
	})
}

func TestACallerThatGivesUpDoesNotStopTheOthers(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var f embeddertest.Fake
		b := newBatcher(t, &f, 4, 10*time.Millisecond)
		ctx, cancel := context.WithTimeout(t.Context(), 5*time.Millisecond)
		defer cancel()
		var gaveUp error
		var wg sync.WaitGroup
		wg.Go(func() { _, gaveUp = b.Embed(ctx, []string{"impatient"}) })
		wg.Go(func() {
			if _, err := b.Embed(t.Context(), []string{"patient"}); err != nil {
				t.Errorf("the patient caller failed: %v", err)
			}
		})
		wg.Wait()
		if !errors.Is(gaveUp, context.DeadlineExceeded) {
			t.Errorf("the impatient caller got %v, want DeadlineExceeded", gaveUp)
		}
	})
}

func TestOnlyTheLongTextFails(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b := newBatcher(t, &embeddertest.Fake{Context: 8}, 3, time.Millisecond)
		errs := make(map[string]error)
		var mu sync.Mutex
		var wg sync.WaitGroup
		for _, text := range []string{"short", "too long a text", "brief"} {
			wg.Go(func() {
				_, err := b.Embed(t.Context(), []string{text})
				mu.Lock()
				errs[text] = err
				mu.Unlock()
			})
		}
		wg.Wait()
		if errs["short"] != nil || errs["brief"] != nil || !errors.Is(errs["too long a text"], ingestion.ErrTextTooLong) {
			t.Fatalf("errors = %v, want only the long text to fail with ErrTextTooLong", errs)
		}
	})
}

func TestEmbedAfterCloseFails(t *testing.T) {
	b, err := NewBatcher(new(embeddertest.Fake), 4, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	b.Close()
	if _, err := b.Embed(t.Context(), []string{"a"}); err == nil {
		t.Fatal("Embed after Close worked")
	}
}

func TestNewBatcherRejectsABadSizeOrWait(t *testing.T) {
	for _, c := range []struct {
		size int
		wait time.Duration
	}{{0, time.Millisecond}, {4, 0}} {
		if _, err := NewBatcher(new(embeddertest.Fake), c.size, c.wait); err == nil {
			t.Errorf("NewBatcher(size %d, wait %v) worked", c.size, c.wait)
		}
	}
}
