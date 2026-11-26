package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// fourDims answers POST /api/embed with the vector [1 2 3 4] for every input, and
// records how many inputs each request had. With a context, an input of more bytes gets
// Ollama's answer to a text over the context.
type fourDims struct {
	context int
	mu      sync.Mutex
	batches []int
}

func (f *fourDims) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	var req struct{ Input []string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f.mu.Lock()
	f.batches = append(f.batches, len(req.Input))
	f.mu.Unlock()
	for _, text := range req.Input {
		if f.context > 0 && len(text) > f.context {
			http.Error(w, `{"error":"the input length exceeds the context length"}`, http.StatusBadRequest)
			return
		}
	}
	embeddings := make([][]float32, len(req.Input))
	for i := range embeddings {
		embeddings[i] = []float32{1, 2, 3, 4}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": embeddings})
}

// corpusOf writes the files to a new directory and returns it.
func corpusOf(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, text := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestIngestEmbedsTheCorpus(t *testing.T) {
	dir := corpusOf(t, map[string]string{"a.go": "package a\n", "b/b.go": "package b\n"})
	srv := httptest.NewServer(new(fourDims))
	defer srv.Close()

	var out bytes.Buffer
	if err := run([]string{"ingest", "-corpus", dir, "-ollama", srv.URL, "-dim", "4"}, &out); err != nil {
		t.Fatal(err)
	}
	want := "completed documents=2 chunks=2 embedded=2 failed=0 pending=0 in "
	if got := out.String(); !strings.HasPrefix(got, want) {
		t.Errorf("ingest output = %q, want it to start with %q", got, want)
	}
}

func TestIngestBatchesTheChunks(t *testing.T) {
	files := make(map[string]string)
	for i := range 12 {
		files[fmt.Sprintf("f%02d.go", i)] = fmt.Sprintf("package f%d\n", i)
	}
	dir := corpusOf(t, files)
	var f fourDims
	srv := httptest.NewServer(&f)
	defer srv.Close()

	var out bytes.Buffer
	args := []string{"ingest", "-corpus", dir, "-ollama", srv.URL, "-dim", "4", "-workers", "8", "-batch", "4"}
	if err := run(args, &out); err != nil {
		t.Fatal(err)
	}
	if want := "completed documents=12 chunks=12 embedded=12 "; !strings.HasPrefix(out.String(), want) {
		t.Fatalf("ingest output = %q, want it to start with %q", out.String(), want)
	}
	total := 0
	for _, n := range f.batches {
		if n > 4 {
			t.Errorf("a request had %d inputs, want at most 4", n)
		}
		total += n
	}
	if total != 12 || len(f.batches) == 12 {
		t.Errorf("requests had %v inputs, want 12 in fewer than 12 requests", f.batches)
	}
}

func TestIngestSplitsALongChunk(t *testing.T) {
	long := "package a\n\n" + strings.Repeat("var x = 1\n", 20) // 212 bytes, 22 lines
	dir := corpusOf(t, map[string]string{"a.go": long})
	srv := httptest.NewServer(&fourDims{context: 64})
	defer srv.Close()
	for _, c := range []struct {
		split string
		want  string
	}{
		{"-split=false", "completed documents=1 chunks=1 embedded=0 failed=1 "},
		{"-split", "completed documents=1 chunks=1 embedded=1 failed=0 "},
	} {
		var out bytes.Buffer
		if err := run([]string{"ingest", "-corpus", dir, "-ollama", srv.URL, "-dim", "4", c.split}, &out); err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(out.String(), c.want) {
			t.Errorf("%s: output = %q, want it to start with %q", c.split, out.String(), c.want)
		}
	}
}

func TestIngestNeedsAnOllama(t *testing.T) {
	t.Setenv("GARIMPO_OLLAMA_URL", "")
	err := run([]string{"ingest", "-corpus", t.TempDir()}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "GARIMPO_OLLAMA_URL") {
		t.Errorf("ingest without an Ollama: error = %v", err)
	}
}
