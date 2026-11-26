package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"syscall"
	"testing"

	"github.com/aluiziodeveloper/garimpo/internal/adapters/journal"
	"github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
)

// fourDims answers POST /api/embed with the vector [1 2 3 4] for every input, and
// records how many inputs each request had.
type fourDims struct {
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
	want := "completed documents=2 chunks=2 embedded=2 failed=0 pending=0 skipped=0 in "
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

func TestIngestLimitsTheCallsAdaptively(t *testing.T) {
	files := make(map[string]string)
	for i := range 12 {
		files[fmt.Sprintf("f%02d.go", i)] = fmt.Sprintf("package f%d\n", i)
	}
	dir := corpusOf(t, files)
	srv := httptest.NewServer(new(fourDims))
	defer srv.Close()

	var out bytes.Buffer
	args := []string{"ingest", "-corpus", dir, "-ollama", srv.URL, "-dim", "4", "-workers", "8", "-adaptive"}
	if err := run(args, &out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if want := "completed documents=12 chunks=12 embedded=12 "; !strings.HasPrefix(got, want) {
		t.Fatalf("ingest output = %q, want it to start with %q", got, want)
	}
	if !regexp.MustCompile(`in \S+ \(adaptive limit [1-8] of 8\)\n$`).MatchString(got) {
		t.Fatalf("ingest output = %q, want it to end with the adaptive limit, of 8", got)
	}
}

func TestIngestNeedsAnOllama(t *testing.T) {
	t.Setenv("GARIMPO_OLLAMA_URL", "")
	err := run([]string{"ingest", "-corpus", t.TempDir()}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "GARIMPO_OLLAMA_URL") {
		t.Errorf("ingest without an Ollama: error = %v", err)
	}
}

// A journal whose run started and never ended, as a process that died leaves it: ingest
// resumes that run, and a second ingest finds it completed and calls the model no more.
func TestIngestResumesTheRunOfTheJournal(t *testing.T) {
	dir := corpusOf(t, map[string]string{"a.go": "package a\n", "b/b.go": "package b\n"})
	var f fourDims
	srv := httptest.NewServer(&f)
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "ingest.journal")
	j, err := journal.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = j.Append([]ingestion.Event{ingestion.RunStarted{Run: "ingest"}}); err != nil {
		t.Fatal(err)
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}

	args := []string{"ingest", "-corpus", dir, "-ollama", srv.URL, "-dim", "4", "-journal", path}
	var out bytes.Buffer
	if err = run(args, &out); err != nil {
		t.Fatal(err)
	}
	if want := "completed documents=2 chunks=2 embedded=2 "; !strings.HasPrefix(out.String(), want) {
		t.Fatalf("ingest output = %q, want it to start with %q", out.String(), want)
	}
	out.Reset()
	if err = run(args, &out); err != nil {
		t.Fatal(err)
	}
	if want := "completed already: nothing to do\n"; !strings.HasSuffix(out.String(), want) || len(f.batches) != 2 {
		t.Fatalf("second ingest output = %q after %d requests, want it to end with %q after 2", out.String(), len(f.batches), want)
	}
}

// sigterm answers like fourDims and, at the first request, sends SIGTERM to this process,
// as docker stop does to the container's.
type sigterm struct {
	fourDims
	once sync.Once
}

func (s *sigterm) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.once.Do(func() {
		if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
			panic(err)
		}
	})
	s.fourDims.ServeHTTP(w, r)
}

// A SIGTERM pauses the run: ingest drains, says so and returns no error, and the journal
// lets the next ingest finish it.
func TestIngestPausesOnSIGTERM(t *testing.T) {
	files := make(map[string]string)
	for i := range 50 {
		files[fmt.Sprintf("f%02d.go", i)] = fmt.Sprintf("package f%d\n", i)
	}
	dir := corpusOf(t, files)
	srv := httptest.NewServer(new(sigterm))
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "ingest.journal")
	args := []string{"ingest", "-corpus", dir, "-ollama", srv.URL, "-dim", "4", "-journal", path}

	var out bytes.Buffer
	if err := run(args, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(out.String(), "paused ") {
		t.Fatalf("ingest output after SIGTERM = %q, want it to start with \"paused \"", out.String())
	}
	out.Reset()
	if err := run(args, &out); err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^completed documents=50 chunks=\d+ embedded=\d+ failed=0 pending=0 skipped=[1-9]`).MatchString(out.String()) {
		t.Fatalf("second ingest output = %q, want it completed, skipping what the first one did", out.String())
	}
}

// A failed run is final: ingest does not run it again, says why it failed and how to start
// over, and calls the model no more.
func TestIngestRefusesAFailedRunOfTheJournal(t *testing.T) {
	dir := corpusOf(t, map[string]string{"a.go": "package a\n"})
	var f fourDims
	srv := httptest.NewServer(&f)
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "ingest.journal")
	j, err := journal.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if err = j.Append([]ingestion.Event{
		ingestion.RunStarted{Run: "ingest"},
		ingestion.RunFailed{Run: "ingest", Reason: "read a.go: permission denied"},
	}); err != nil {
		t.Fatal(err)
	}
	if err = j.Close(); err != nil {
		t.Fatal(err)
	}

	err = run([]string{"ingest", "-corpus", dir, "-ollama", srv.URL, "-dim", "4", "-journal", path}, &bytes.Buffer{})
	want := "the run in " + path + " failed (read a.go: permission denied): remove the journal to start over"
	if err == nil || !strings.Contains(err.Error(), want) || len(f.batches) != 0 {
		t.Fatalf("ingest of a failed run: err = %v after %d requests; want %q and none", err, len(f.batches), want)
	}
}
