package ollama

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"testing"
	"testing/synctest"
	"time"
)

// TestADeadlineCutsASlowOllama: a model that takes a minute is cut by the caller's 30s
// deadline, at exactly 30s on the bubble's clock, in no time on the wall. Over a real
// socket, the bubble could not move its clock (labs/synctest).
func TestADeadlineCutsASlowOllama(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := fakeOllama{reply: func(w http.ResponseWriter, r *http.Request, _ []string) {
			select {
			case <-time.After(time.Minute): // a model stuck on a long queue
				w.WriteHeader(http.StatusOK)
			case <-r.Context().Done(): // the client hung up
			}
		}}
		e := newEmbedder(t, &f)
		ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
		defer cancel()
		start := time.Now()
		_, err := e.Embed(ctx, []string{"a"})
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("err = %v, want DeadlineExceeded", err)
		}
		if took := time.Since(start); took != 30*time.Second {
			t.Fatalf("Embed gave up after %v, want 30s", took)
		}
	})
}

// TestTheBatcherSendsOneRequestPerBatch runs a Batcher in front of the Embedder over the
// in-memory network: five chunks in batches of three are two HTTP requests, the full one
// at once and the rest after the wait.
func TestTheBatcherSendsOneRequestPerBatch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var f fakeOllama
		b := newBatcher(t, newEmbedder(t, &f), 3, 10*time.Millisecond)
		start := time.Now()
		var wg sync.WaitGroup
		for _, text := range []string{"a", "b", "c", "d", "e"} {
			wg.Go(func() {
				if _, err := b.Embed(t.Context(), []string{text}); err != nil {
					t.Errorf("Embed(%q): %v", text, err)
				}
			})
		}
		wg.Wait()
		var got []string
		for _, a := range f.arrivals {
			got = append(got, a.at.Sub(start).String()+":"+string(rune('0'+a.inputs)))
		}
		if len(got) != 2 || got[0] != "0s:3" || got[1] != "10ms:2" {
			t.Fatalf("requests (time:inputs) = %v, want [0s:3 10ms:2]", got)
		}
	})
}
