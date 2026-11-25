package ingestion

import (
	"errors"
	"runtime"
	"testing"
	"time"

	domain "github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
)

func TestParallelCuttersEmbedEveryChunkOnce(t *testing.T) {
	var f fakeEmbedder
	var events []domain.Event
	run, cfg := setup(t, &f, &events)
	cfg.Cutters = 4

	if err := RunIngestion(t.Context(), run, oneLiners(t, 200), cfg); err != nil {
		t.Fatal(err)
	}
	if run.State() != domain.Completed || len(f.seen) != 200 {
		t.Fatalf("run %s with %d chunks embedded, want completed with 200", run.State(), len(f.seen))
	}
	for id, n := range f.seen {
		if n != 1 {
			t.Errorf("chunk %s embedded %d times", id, n)
		}
	}
}

func TestParallelCuttersKeepMaxPending(t *testing.T) {
	var cfg Config
	var most int64
	f := fakeEmbedder{fail: func(domain.Chunk) error {
		most = max(most, cfg.Progress.Snapshot().Pending())
		time.Sleep(time.Millisecond)
		return nil
	}}
	var events []domain.Event
	run, cfg := setup(t, &f, &events)
	cfg.Cutters, cfg.Workers, cfg.MaxPending = 4, 2, 8

	if err := RunIngestion(t.Context(), run, oneLiners(t, 100), cfg); err != nil {
		t.Fatal(err)
	}
	if most > 8 {
		t.Fatalf("%d chunks pending at once with 4 cutters, want at most 8 (RN03)", most)
	}
}

func TestParallelCuttersStopOnAReadError(t *testing.T) {
	var f fakeEmbedder
	var events []domain.Event
	run, cfg := setup(t, &f, &events)
	cfg.Cutters = 4
	before := runtime.NumGoroutine()

	err := RunIngestion(t.Context(), run, documents(t, errDisk, "aaaa\n", "bbbb\n"), cfg)
	if !errors.Is(err, errDisk) || run.State() != domain.Failed {
		t.Fatalf("err = %v, run %s; want errDisk and a failed run", err, run.State())
	}
	if after := settled(before); after != before {
		t.Fatalf("%d goroutines before, %d after", before, after)
	}
}

func TestANegativeNumberOfCuttersIsRejected(t *testing.T) {
	var f fakeEmbedder
	var events []domain.Event
	run, cfg := setup(t, &f, &events)
	cfg.Cutters = -1
	if err := RunIngestion(t.Context(), run, oneLiners(t, 1), cfg); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("err = %v, want ErrInvalidConfig", err)
	}
}
