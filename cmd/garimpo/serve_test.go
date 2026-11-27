package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aluiziodeveloper/garimpo/internal/adapters/httpapi"
)

// syncBuffer is a bytes.Buffer safe for the server's goroutines and the test's.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

var files = map[string]string{
	"sort/sort.go":   "package sort\n\n// Slice sorts a slice.\nfunc Slice() {}\n",
	"strings/str.go": "package strings\n\n// Fields splits a string.\nfunc Fields() {}\n",
}

// ingested returns a corpus and the file of vectors its ingestion kept, with the model of
// fourDims.
func ingested(t *testing.T) (dir, vectors string) {
	t.Helper()
	dir = corpusOf(t, files)
	srv := httptest.NewServer(new(fourDims))
	defer srv.Close()
	vectors = filepath.Join(t.TempDir(), "corpus.vec")
	if err := run([]string{"ingest", "-corpus", dir, "-ollama", srv.URL, "-dim", "4", "-vectors", vectors}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	return dir, vectors
}

var serving = regexp.MustCompile(`serving (\d+) chunks, (\d+) with vectors, on (\S+) `)

// started runs serve with args until the test cancels the context it returns, and returns
// the address it listens on and what serve returned, once it did.
func started(t *testing.T, out *syncBuffer, args ...string) (string, context.CancelFunc, <-chan error) {
	t.Helper()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, append([]string{"-addr", "127.0.0.1:0", "-dim", "4"}, args...), out) }()
	for range 500 {
		if m := serving.FindStringSubmatch(out.String()); m != nil {
			return m[3], cancel, done
		}
		select {
		case err := <-done:
			cancel()
			t.Fatalf("serve returned before serving: %v; output: %s", err, out)
		case <-time.After(10 * time.Millisecond):
		}
	}
	cancel()
	t.Fatalf("serve is not serving after 5s; output: %s", out)
	return "", nil, nil
}

func get(t *testing.T, addr, q string) (int, httpapi.Response) {
	t.Helper()
	resp, err := http.Get(fmt.Sprintf("http://%s/search?q=%s", addr, q))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var body httpapi.Response
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
	}
	return resp.StatusCode, body
}

// serve loads what the ingestion kept and answers a search with the chunk that has its
// terms first, cited by path and lines; canceling its context stops it.
func TestServeAnswersSearchesOverTheIngestion(t *testing.T) {
	dir, vectors := ingested(t)
	model := httptest.NewServer(new(fourDims))
	defer model.Close()
	var out syncBuffer
	addr, stop, done := started(t, &out, "-corpus", dir, "-vectors", vectors, "-ollama", model.URL)
	if m := serving.FindStringSubmatch(out.String()); m[1] != "2" || m[2] != "2" {
		t.Errorf("serving %s chunks, %s with vectors; want 2 and 2", m[1], m[2])
	}
	code, body := get(t, addr, "sorts+slice")
	if code != http.StatusOK || len(body.Results) != 2 {
		t.Fatalf("status %d with %d results, want 200 with 2", code, len(body.Results))
	}
	if got := body.Results[0]; got.Citation != "sort/sort.go:1-4" || !strings.Contains(got.Text, "Slice sorts") {
		t.Errorf("first result %+v, want sort/sort.go:1-4 with its text", got)
	}
	stop()
	if err := <-done; err != nil {
		t.Fatalf("serve after a stop: %v", err)
	}
	if !strings.HasSuffix(out.String(), "stopped\n") {
		t.Errorf("output %q does not end with stopped", out.String())
	}
}

// slowModel answers the vector of a query only once release is closed; started is closed
// on the first request.
type slowModel struct {
	fourDims
	once             sync.Once
	started, release chan struct{}
}

func (m *slowModel) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.once.Do(func() { close(m.started) })
	<-m.release
	m.fourDims.ServeHTTP(w, r)
}

// A stop lets the search in flight answer: the server stops taking requests, not answering.
func TestAStopLetsTheSearchInFlightAnswer(t *testing.T) {
	dir, vectors := ingested(t)
	m := &slowModel{started: make(chan struct{}), release: make(chan struct{})}
	model := httptest.NewServer(m)
	defer model.Close()
	var out syncBuffer
	addr, stop, done := started(t, &out, "-corpus", dir, "-vectors", vectors, "-ollama", model.URL)
	answered := make(chan *http.Response, 1)
	go func() {
		resp, err := http.Get("http://" + addr + "/search?q=slice")
		if err != nil {
			t.Error(err) // Error, not Fatal: this is not the test's goroutine
		}
		answered <- resp
	}()
	<-m.started
	stop()
	select {
	case err := <-done:
		t.Fatalf("serve returned with a search in flight: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if _, err := http.Get("http://" + addr + "/search?q=slice"); err == nil {
		t.Error("a new request was taken after the stop")
	}
	close(m.release)
	if resp := <-answered; resp != nil {
		var body httpapi.Response
		err := json.NewDecoder(resp.Body).Decode(&body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK || err != nil || len(body.Results) != 2 {
			t.Errorf("the search in flight: status %d with %d results (%v), want 200 with 2", resp.StatusCode, len(body.Results), err)
		}
	}
	if err := <-done; err != nil {
		t.Fatalf("serve after a stop: %v", err)
	}
}

// Vectors of chunks the corpus does not have mean the corpus changed after the ingestion:
// serve refuses to start, instead of citing the wrong lines.
func TestServeRefusesVectorsOfAnotherCorpus(t *testing.T) {
	_, vectors := ingested(t)
	other := corpusOf(t, map[string]string{"sort/sort.go": "package sort\n\n// Changed.\n"})
	err := serve(t.Context(), []string{"-addr", "127.0.0.1:0", "-dim", "4", "-corpus", other, "-vectors", vectors, "-ollama", "http://unused"}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "2 of 2 vectors") {
		t.Errorf("serve over another corpus: err = %v, want the 2 vectors refused", err)
	}
	if err := serve(t.Context(), []string{"-ollama", "http://unused"}, &bytes.Buffer{}); err == nil {
		t.Error("serve without -vectors: err = nil")
	}
	if err := run([]string{"serve", "-vectors", vectors, "-ollama", "http://unused", "-timeout", "0s"}, &bytes.Buffer{}); err == nil {
		t.Error("serve with -timeout 0s: err = nil")
	}
}
