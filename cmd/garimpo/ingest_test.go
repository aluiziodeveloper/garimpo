package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fourDims answers POST /api/embed with the vector [1 2 3 4] for every input.
func fourDims(w http.ResponseWriter, r *http.Request) {
	var req struct{ Input []string }
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	embeddings := make([][]float32, len(req.Input))
	for i := range embeddings {
		embeddings[i] = []float32{1, 2, 3, 4}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": embeddings})
}

func TestIngestEmbedsTheCorpus(t *testing.T) {
	dir := t.TempDir()
	for name, text := range map[string]string{"a.go": "package a\n", "b/b.go": "package b\n"} {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	srv := httptest.NewServer(http.HandlerFunc(fourDims))
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

func TestIngestNeedsAnOllama(t *testing.T) {
	t.Setenv("GARIMPO_OLLAMA_URL", "")
	err := run([]string{"ingest", "-corpus", t.TempDir()}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "GARIMPO_OLLAMA_URL") {
		t.Errorf("ingest without an Ollama: error = %v", err)
	}
}
