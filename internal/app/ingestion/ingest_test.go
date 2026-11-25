package ingestion

import (
	"context"
	"errors"
	"runtime"
	"sync"
	"testing"
	"time"

	domain "github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// fakeEmbedder records the chunks it was given. fail, when set, decides which ones fail.
type fakeEmbedder struct {
	mu   sync.Mutex
	seen map[shared.ChunkID]int
	fail func(domain.Chunk) error
}

func (f *fakeEmbedder) embed(_ context.Context, c domain.Chunk) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.seen == nil {
		f.seen = make(map[shared.ChunkID]int)
	}
	f.seen[c.ID()]++
	if f.fail != nil {
		return f.fail(c)
	}
	return nil
}

// setup returns a pending run and a configuration that cuts with a 10-byte window, embeds
// with f on 3 workers and keeps every event in events.
func setup(t *testing.T, f *fakeEmbedder, events *[]domain.Event) (*domain.Run, Config) {
	t.Helper()
	run, err := domain.NewRun("run-1")
	if err != nil {
		t.Fatal(err)
	}
	cutter, err := domain.NewCutter(10)
	if err != nil {
		t.Fatal(err)
	}
	return run, Config{
		Cutter:   cutter,
		Workers:  3,
		Embed:    f.embed,
		Progress: new(Progress),
		Record:   func(e []domain.Event) { *events = append(*events, e...) },
	}
}

func TestRunIngestionEmbedsEveryChunkOnce(t *testing.T) {
	var f fakeEmbedder
	var events []domain.Event
	run, cfg := setup(t, &f, &events)
	docs := documents(t, nil, "aaaa\nbbbb\ncccc\n", "", "dd\n")

	if err := RunIngestion(t.Context(), run, docs, cfg); err != nil {
		t.Fatal(err)
	}
	if run.State() != domain.Completed {
		t.Fatalf("run is %s, want completed", run.State())
	}
	if len(f.seen) != 3 {
		t.Fatalf("embedded %d chunks, want 3", len(f.seen))
	}
	for id, n := range f.seen {
		if n != 1 {
			t.Errorf("chunk %s embedded %d times", id, n)
		}
	}
	want := Counts{Documents: 3, Chunks: 3, Embedded: 3}
	if got := cfg.Progress.Snapshot(); got != want {
		t.Errorf("progress = %v, want %v", got, want)
	}
	if _, ok := events[0].(domain.RunStarted); !ok {
		t.Errorf("first event = %+v, want RunStarted", events[0])
	}
	done := domain.RunCompleted{Run: "run-1", Documents: 3, Embedded: 3}
	if last := events[len(events)-1]; last != done {
		t.Errorf("last event = %+v, want %+v", last, done)
	}
	if len(events) != 1+3+3+1 {
		t.Errorf("%d events, want 8: started, 3 documents, 3 chunks, completed", len(events))
	}
}

var errModel = errors.New("model unavailable")

func TestAFailedChunkDoesNotStopTheRun(t *testing.T) {
	f := fakeEmbedder{fail: func(c domain.Chunk) error {
		if c.Path() == "b.go" {
			return errModel
		}
		return nil
	}}
	var events []domain.Event
	run, cfg := setup(t, &f, &events)

	if err := RunIngestion(t.Context(), run, documents(t, nil, "aaaa\n", "bbbb\n"), cfg); err != nil {
		t.Fatal(err)
	}
	done := domain.RunCompleted{Run: "run-1", Documents: 2, Embedded: 1, Failed: 1}
	if last := events[len(events)-1]; last != done {
		t.Fatalf("last event = %+v, want %+v", last, done)
	}
	for _, e := range events {
		if cf, ok := e.(domain.ChunkFailed); ok && cf.Reason != errModel.Error() {
			t.Errorf("ChunkFailed reason = %q, want %q", cf.Reason, errModel)
		}
	}
}

var errDisk = errors.New("disk gone")

func TestAReadErrorFailsTheRun(t *testing.T) {
	var f fakeEmbedder
	var events []domain.Event
	run, cfg := setup(t, &f, &events)

	err := RunIngestion(t.Context(), run, documents(t, errDisk, "aaaa\n"), cfg)
	if !errors.Is(err, errDisk) {
		t.Fatalf("err = %v, want errDisk", err)
	}
	if run.State() != domain.Failed || run.Reason() != err.Error() {
		t.Fatalf("run is %s (%q), want failed with %q", run.State(), run.Reason(), err)
	}
	if _, ok := events[len(events)-1].(domain.RunFailed); !ok {
		t.Fatalf("last event = %+v, want RunFailed", events[len(events)-1])
	}
}

func TestCancelingFailsTheRun(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	f := fakeEmbedder{fail: func(domain.Chunk) error { cancel(); return nil }}
	var events []domain.Event
	run, cfg := setup(t, &f, &events)
	texts := make([]string, 100)
	for i := range texts {
		texts[i] = "aaaa\n"
	}

	err := RunIngestion(ctx, run, documents(t, nil, texts...), cfg)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if run.State() != domain.Failed {
		t.Fatalf("run is %s, want failed", run.State())
	}
}

// settled returns the number of goroutines once it falls to want, or after a second. A
// goroutine that has closed its channel may still be returning, and counted, for a moment;
// one that leaked never returns.
func settled(want int) int {
	n := runtime.NumGoroutine()
	for deadline := time.Now().Add(time.Second); n > want && time.Now().Before(deadline); {
		time.Sleep(time.Millisecond)
		n = runtime.NumGoroutine()
	}
	return n
}

func TestRunIngestionLeavesNoGoroutineBehind(t *testing.T) {
	before := runtime.NumGoroutine()
	for range 10 {
		var f fakeEmbedder
		var events []domain.Event
		run, cfg := setup(t, &f, &events)
		if err := RunIngestion(t.Context(), run, documents(t, errDisk, "aaaa\n", "bbbb\n"), cfg); !errors.Is(err, errDisk) {
			t.Fatal(err)
		}
	}
	if after := settled(before); after != before {
		t.Fatalf("%d goroutines before, %d after ten failed runs", before, after)
	}
}

func TestRunIngestionNeedsAWorkerAndAnEmbedder(t *testing.T) {
	var f fakeEmbedder
	var events []domain.Event
	for _, broken := range []func(*Config){
		func(c *Config) { c.Workers = 0 },
		func(c *Config) { c.Embed = nil },
	} {
		run, cfg := setup(t, &f, &events)
		broken(&cfg)
		if err := RunIngestion(t.Context(), run, documents(t, nil, "aaaa\n"), cfg); !errors.Is(err, ErrInvalidConfig) {
			t.Errorf("err = %v, want ErrInvalidConfig", err)
		}
		if run.State() != domain.Pending {
			t.Errorf("a run with a broken config is %s, want pending", run.State())
		}
	}
}
