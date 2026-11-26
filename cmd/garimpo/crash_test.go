package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"syscall"
	"testing"
	"time"

	"github.com/aluiziodeveloper/garimpo/internal/adapters/journal"
	"github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// slowDims is fourDims with a delay per request, so that a run lasts long enough to be
// killed in the middle.
type slowDims struct {
	fourDims
	delay time.Duration
}

func (s *slowDims) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	time.Sleep(s.delay)
	s.fourDims.ServeHTTP(w, r)
}

// TestCrashChild is the process the crash test kills: it runs ingest with the arguments
// of GARIMPO_CRASH_ARGS against a slow fake Ollama. Run alone, it skips.
func TestCrashChild(t *testing.T) {
	dir, path := os.Getenv("GARIMPO_CRASH_CORPUS"), os.Getenv("GARIMPO_CRASH_JOURNAL")
	if dir == "" || path == "" {
		t.Skip("the child of TestIngestResumesAfterKill9; it runs only from there")
	}
	srv := httptest.NewServer(&slowDims{delay: 5 * time.Millisecond})
	defer srv.Close()
	if err := run([]string{"ingest", "-corpus", dir, "-ollama", srv.URL, "-dim", "4", "-journal", path}, os.Stdout); err != nil {
		t.Fatal(err)
	}
}

// confirmed returns how many times the journal at path confirms each chunk.
func confirmed(t *testing.T, path string) map[shared.ChunkID]int {
	t.Helper()
	j, err := journal.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = j.Close() }()
	n := make(map[shared.ChunkID]int)
	for e, err := range j.Replay() {
		if err != nil {
			t.Fatal(err)
		}
		if c, ok := e.(ingestion.ChunkEmbedded); ok {
			n[c.Chunk]++
		}
	}
	return n
}

// RN02 against the worst stop: kill -9 gives the process no chance to drain or pause.
// What reached the journal is never embedded again, and nothing is confirmed twice.
func TestIngestResumesAfterKill9(t *testing.T) {
	if testing.Short() {
		t.Skip("starts a process and waits for it")
	}
	files := make(map[string]string)
	for i := range 400 {
		files[fmt.Sprintf("f%03d.go", i)] = fmt.Sprintf("package f%d\n", i)
	}
	dir := corpusOf(t, files)
	path := filepath.Join(t.TempDir(), "ingest.journal")

	child := exec.Command(os.Args[0], "-test.run=^TestCrashChild$")
	child.Env = append(os.Environ(), "GARIMPO_CRASH_CORPUS="+dir, "GARIMPO_CRASH_JOURNAL="+path)
	var childOut bytes.Buffer
	child.Stdout, child.Stderr = &childOut, &childOut
	if err := child.Start(); err != nil {
		t.Fatal(err)
	}
	// Kill it once the journal confirms 100 chunks: by then it is running, with calls in
	// flight and chunks waiting in the pipeline.
	for deadline := time.Now().Add(10 * time.Second); ; time.Sleep(time.Millisecond) {
		data, _ := os.ReadFile(path) // the child creates it; until then, nothing to count
		if bytes.Count(data, []byte(`"ChunkEmbedded"`)) >= 100 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("the child confirmed fewer than 100 chunks in 10 s:\n%s", childOut.String())
		}
	}
	if err := child.Process.Signal(syscall.SIGKILL); err != nil {
		t.Fatal(err)
	}
	if err := child.Wait(); err == nil || child.ProcessState.String() != "signal: killed" {
		t.Fatalf("the child ended with %v, want signal: killed:\n%s", err, childOut.String())
	}
	before := confirmed(t, path)
	t.Logf("killed with %d chunks confirmed in the journal", len(before))

	var f fourDims
	srv := httptest.NewServer(&f)
	defer srv.Close()
	var out bytes.Buffer
	if err := run([]string{"ingest", "-corpus", dir, "-ollama", srv.URL, "-dim", "4", "-journal", path}, &out); err != nil {
		t.Fatal(err)
	}
	t.Logf("resumed: %s", out.String())
	if want := fmt.Sprintf(`^completed documents=400 chunks=%d embedded=%d failed=0 pending=0 skipped=%d `,
		400-len(before), 400-len(before), len(before)); !regexp.MustCompile(want).MatchString(out.String()) {
		t.Fatalf("resumed ingest output = %q, want it to match %q", out.String(), want)
	}
	if len(f.batches) != 400-len(before) {
		t.Fatalf("the resumed run called the model %d times, want %d: one per chunk the journal lacked", len(f.batches), 400-len(before))
	}
	after := confirmed(t, path)
	for id, n := range after {
		if n != 1 {
			t.Errorf("chunk %s confirmed %d times", id, n)
		}
	}
	if len(after) != 400 {
		t.Fatalf("%d chunks confirmed in the journal, want 400", len(after))
	}
}
