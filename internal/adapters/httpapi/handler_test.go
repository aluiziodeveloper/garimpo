package httpapi_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/aluiziodeveloper/garimpo/internal/adapters/httpapi"
	"github.com/aluiziodeveloper/garimpo/internal/app/search"
	"github.com/aluiziodeveloper/garimpo/internal/domain/retrieval"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

func chunk(t *testing.T, n int) shared.ChunkID {
	t.Helper()
	id, err := shared.NewChunkID("sort/sort.go", n, n+1, n+2, fmt.Sprint("chunk ", n))
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// ranker is a search.Ranker: it returns hits, or err, or, with block, waits for its context
// to end and returns its error. It records the context it got.
type ranker struct {
	hits  []retrieval.Hit
	err   error
	block bool
	ctx   context.Context
}

func (r *ranker) Search(ctx context.Context, _ string, k int) ([]retrieval.Hit, error) {
	r.ctx = ctx
	if r.block {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	return r.hits[:min(k, len(r.hits))], r.err
}

// texts holds chunks numbered n at sort/sort.go, lines n+1 to n+2.
type texts struct{ known map[shared.ChunkID]int }

func (x texts) FindChunk(id shared.ChunkID) (string, int, int, bool) {
	n, ok := x.known[id]
	return "sort/sort.go", n + 1, n + 2, ok
}

func (x texts) Text(id shared.ChunkID) (string, error) {
	return fmt.Sprint("text of ", x.known[id]), nil
}

// serve returns the handler over the real use case, with r as its ranker and chunks 1 to 3.
func serve(t *testing.T, r *ranker, timeout time.Duration) http.Handler {
	known := map[shared.ChunkID]int{}
	for n := 1; n <= 3; n++ {
		known[chunk(t, n)] = n
	}
	return httpapi.New(&search.Searcher{Ranker: r, Texts: texts{known}}, timeout, nil)
}

func get(t *testing.T, h http.Handler, target string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, target, nil))
	return w
}

func decode[T any](t *testing.T, w *httptest.ResponseRecorder) T {
	t.Helper()
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
	var body T
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("the body is not the JSON expected: %v", err)
	}
	return body
}

func TestASearchAnswersWithTheResultsAsJSON(t *testing.T) {
	r := &ranker{hits: []retrieval.Hit{{Chunk: chunk(t, 2), Score: 0.5}, {Chunk: chunk(t, 1), Score: 0.25}, {Chunk: chunk(t, 3), Score: 0.125}}}
	w := get(t, serve(t, r, time.Second), "/search?q=+sort+a+slice&k=2")
	if w.Code != http.StatusOK {
		t.Fatalf("status %d, want 200: %s", w.Code, w.Body)
	}
	got := decode[httpapi.Response](t, w)
	want := httpapi.Response{Query: "sort a slice", Results: []httpapi.Result{
		{Citation: "sort/sort.go:3-4", Score: 0.5, Text: "text of 2"},
		{Citation: "sort/sort.go:2-3", Score: 0.25, Text: "text of 1"},
	}}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("body = %+v, want %+v", got, want)
	}
	if w := get(t, serve(t, r, time.Second), "/search?q=sort"); len(decode[httpapi.Response](t, w).Results) != 3 {
		t.Errorf("without k: want the 3 hits, at most DefaultK (%d)", httpapi.DefaultK)
	}
}

// What the client asked wrong is 400, with the reason in the body.
func TestABadRequestIsAnsweredWith400(t *testing.T) {
	h := serve(t, &ranker{}, time.Second)
	for _, target := range []string{
		"/search", "/search?q=+++", "/search?q=" + strings.Repeat("a", retrieval.MaxQueryBytes+1),
		"/search?q=%FF", "/search?q=sort&k=ten", "/search?q=sort&k=0", fmt.Sprintf("/search?q=sort&k=%d", search.MaxK+1),
	} {
		w := get(t, h, target)
		if w.Code != http.StatusBadRequest {
			t.Errorf("GET %.40s: status %d, want 400", target, w.Code)
			continue
		}
		if body := decode[httpapi.Error](t, w); body.Error == "" {
			t.Errorf("GET %.40s: no reason in the body", target)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/search?q=sort", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /search: status %d, want 405", w.Code)
	}
}

// A search that runs past the deadline is canceled at the deadline, and the client gets 504.
// In a synctest bubble, the clock moves only when every goroutine waits: the test takes no
// real second, and the deadline is exact.
func TestASearchPastTheDeadlineIsAnsweredWith504(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		r := &ranker{block: true}
		start := time.Now()
		w := get(t, serve(t, r, 1500*time.Millisecond), "/search?q=sort")
		if w.Code != http.StatusGatewayTimeout {
			t.Fatalf("status %d, want 504", w.Code)
		}
		if took := time.Since(start); took != 1500*time.Millisecond {
			t.Errorf("the response came after %v, want the deadline, 1.5s", took)
		}
		if !errors.Is(r.ctx.Err(), context.DeadlineExceeded) {
			t.Errorf("the search's context ended with %v, want DeadlineExceeded", r.ctx.Err())
		}
		if body := decode[httpapi.Error](t, w); !strings.Contains(body.Error, "1.5s") {
			t.Errorf("body %q does not say the deadline", body.Error)
		}
	})
}

// A client that leaves cancels its search, and the handler writes nothing for nobody.
func TestAClientThatLeavesCancelsItsSearch(t *testing.T) {
	r := &ranker{block: true}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	w := httptest.NewRecorder()
	serve(t, r, time.Minute).ServeHTTP(w, httptest.NewRequestWithContext(ctx, http.MethodGet, "/search?q=sort", nil))
	if w.Body.Len() != 0 {
		t.Errorf("a response of %d bytes for a client that left: %s", w.Body.Len(), w.Body)
	}
	if r.ctx != nil {
		t.Error("the ranker ran for a client that had left already") // the use case checks ctx first
	}
}

// Any other failure is 500, without its details: they go to the log of the server.
func TestAFailedSearchIsAnsweredWith500(t *testing.T) {
	w := get(t, serve(t, &ranker{err: errors.New("disk on fire")}, time.Second), "/search?q=sort")
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status %d, want 500", w.Code)
	}
	if body := decode[httpapi.Error](t, w); strings.Contains(body.Error, "disk") {
		t.Errorf("body %q shows the cause to the client", body.Error)
	}
}
