package ingestion

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"runtime"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/aluiziodeveloper/garimpo/internal/app/limiter"
	domain "github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// fakeEmbedder counts the texts it was given, by text. fail, when set, decides which
// ones fail. Every vector it returns is the same: these tests are about the pipeline.
type fakeEmbedder struct {
	mu    sync.Mutex
	seen  map[string]int
	total int
	fail  func(text string) error
}

var oneVector, _ = shared.NewVector([]float32{1}, 1)

func (f *fakeEmbedder) Embed(_ context.Context, texts []string) ([]shared.Vector, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.seen == nil {
		f.seen = make(map[string]int)
	}
	vs := make([]shared.Vector, len(texts))
	for i, text := range texts {
		f.seen[text]++
		f.total++
		if f.fail != nil {
			if err := f.fail(text); err != nil {
				return nil, err
			}
		}
		vs[i] = oneVector
	}
	return vs, nil
}

// recorded is a Journal in memory for these tests: journaltest.Memory imports this
// package, and a test of this package cannot import it back.
type recorded []domain.Event

func (r *recorded) Append(events []domain.Event) error {
	*r = append(*r, events...)
	return nil
}

func (r *recorded) Replay() iter.Seq2[domain.Event, error] {
	return func(yield func(domain.Event, error) bool) {
		for _, e := range *r {
			if !yield(e, nil) {
				return
			}
		}
	}
}

// setup returns a pending run and a configuration that cuts with a 10-byte window, embeds
// with f on 3 workers and keeps every event in events.
func setup(t *testing.T, f *fakeEmbedder, events *recorded) (*domain.Run, Config) {
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
		Embedder: f,
		Progress: new(Progress),
		Journal:  events,
	}
}

func TestRunIngestionEmbedsEveryChunkOnce(t *testing.T) {
	var f fakeEmbedder
	var events recorded
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
	for text, n := range f.seen {
		if n != 1 {
			t.Errorf("chunk %q embedded %d times", text, n)
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
	f := fakeEmbedder{fail: func(text string) error {
		if text == "bbbb\n" {
			return errModel
		}
		return nil
	}}
	var events recorded
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
	var events recorded
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

// Canceling ctx is a request to stop, as a SIGTERM is: the run pauses, and every call to
// the model that was in flight finishes and is recorded, so that no answer is thrown away.
func TestStoppingPausesTheRun(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	f := fakeEmbedder{fail: func(string) error { cancel(); return nil }}
	var events recorded
	run, cfg := setup(t, &f, &events)
	texts := make([]string, 100)
	for i := range texts {
		texts[i] = "aaaa\n"
	}

	err := RunIngestion(ctx, run, documents(t, nil, texts...), cfg)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	if run.State() != domain.Paused || events[len(events)-1] != (domain.RunPaused{Run: "run-1"}) {
		t.Fatalf("run is %s, last event %+v; want paused and RunPaused", run.State(), events[len(events)-1])
	}
	confirmed := 0
	for _, e := range events {
		if _, ok := e.(domain.ChunkEmbedded); ok {
			confirmed++
		}
	}
	if confirmed != f.total || confirmed == 0 || confirmed == len(texts) {
		t.Fatalf("%d calls answered, %d chunks confirmed; want them equal, above 0 and below %d", f.total, confirmed, len(texts))
	}
}

// blockingEmbedder holds every call until release is closed, and says if a call saw its
// context done.
type blockingEmbedder struct {
	started  chan struct{} // gets a value when a call begins
	release  chan struct{}
	mu       sync.Mutex
	canceled int
}

func (b *blockingEmbedder) Embed(ctx context.Context, texts []string) ([]shared.Vector, error) {
	b.started <- struct{}{}
	<-b.release
	if ctx.Err() != nil {
		b.mu.Lock()
		b.canceled++
		b.mu.Unlock()
		return nil, ctx.Err()
	}
	return slices.Repeat([]shared.Vector{oneVector}, len(texts)), nil
}

// The stop does not cancel the calls in flight: they finish with their context alive.
func TestAStopLetsTheCallsInFlightFinish(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	b := &blockingEmbedder{started: make(chan struct{}, 3), release: make(chan struct{})}
	var events recorded
	run, cfg := setup(t, nil, &events)
	cfg.Embedder = b
	done := make(chan error)
	go func() { done <- RunIngestion(ctx, run, documents(t, nil, "aaaa\n", "bbbb\n", "cccc\n", "dddd\n"), cfg) }()
	for range 3 { // the three workers are in a call each
		<-b.started
	}
	cancel()
	close(b.release)
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	confirmed := 0
	for _, e := range events {
		if _, ok := e.(domain.ChunkEmbedded); ok {
			confirmed++
		}
	}
	if b.canceled != 0 || confirmed != 3 || events[len(events)-1] != (domain.RunPaused{Run: "run-1"}) {
		t.Fatalf("%d calls saw a canceled context, %d chunks confirmed, last event %+v; want none, 3 and RunPaused",
			b.canceled, confirmed, events[len(events)-1])
	}
}

// A stopped run resumes where it paused: every chunk is embedded once, across the runs.
func TestAStoppedRunResumes(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	calls := 0
	f := fakeEmbedder{fail: func(string) error {
		if calls++; calls == 5 {
			cancel()
		}
		return nil
	}}
	var j recorded
	run, cfg := setup(t, &f, &j)
	texts := make([]string, 20)
	for i := range texts {
		texts[i] = fmt.Sprintf("%04d\n", i)
	}
	if err := RunIngestion(ctx, run, documents(t, nil, texts...), cfg); !errors.Is(err, context.Canceled) {
		t.Fatalf("first run: err = %v, want context.Canceled", err)
	}
	again, err := OpenRun(&j, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if err := RunIngestion(t.Context(), again, documents(t, nil, texts...), cfg); err != nil {
		t.Fatal(err)
	}
	for text, n := range f.seen {
		if n != 1 {
			t.Errorf("chunk %q embedded %d times across the two runs", text, n)
		}
	}
	if len(f.seen) != len(texts) || again.State() != domain.Completed {
		t.Fatalf("%d chunks embedded, run %s; want %d and completed", len(f.seen), again.State(), len(texts))
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

// TestRunIngestionLeavesNoGoroutineBehind fails only on more goroutines after than before.
// The count before may still hold goroutines of an earlier test that are returning, so it
// can be a few too high; a leak in each of the ten runs adds at least ten.
func TestRunIngestionLeavesNoGoroutineBehind(t *testing.T) {
	before := runtime.NumGoroutine()
	for range 10 {
		var f fakeEmbedder
		var events recorded
		run, cfg := setup(t, &f, &events)
		if err := RunIngestion(t.Context(), run, documents(t, errDisk, "aaaa\n", "bbbb\n"), cfg); !errors.Is(err, errDisk) {
			t.Fatal(err)
		}
	}
	if after := settled(before); after > before {
		t.Fatalf("%d goroutines before, %d after ten failed runs", before, after)
	}
}

func TestRunIngestionNeedsAWorkerAndAnEmbedder(t *testing.T) {
	var f fakeEmbedder
	var events recorded
	for _, broken := range []func(*Config){
		func(c *Config) { c.Workers = 0 },
		func(c *Config) { c.Embedder = nil },
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

// oneLiners returns n documents of one line each: n chunks with any window.
func oneLiners(t *testing.T, n int) iter.Seq2[domain.Document, error] {
	t.Helper()
	texts := make([]string, n)
	for i := range texts {
		texts[i] = "aaaa\n"
	}
	return documents(t, nil, texts...)
}

// TestRunIngestionNeverHoldsMoreThanMaxPending is RN03: the chunks cut and not yet
// embedded or failed never pass the limit, however slow the embedder is.
func TestRunIngestionNeverHoldsMoreThanMaxPending(t *testing.T) {
	var cfg Config
	var most int64
	f := fakeEmbedder{fail: func(string) error {
		most = max(most, cfg.Progress.Snapshot().Pending())
		time.Sleep(time.Millisecond) // a slow model: the cutter gets ahead
		return nil
	}}
	var events recorded
	run, cfg := setup(t, &f, &events)
	cfg.Workers, cfg.MaxPending = 2, 8

	if err := RunIngestion(t.Context(), run, oneLiners(t, 100), cfg); err != nil {
		t.Fatal(err)
	}
	if most > 8 {
		t.Fatalf("%d chunks pending at once, want at most 8 (RN03)", most)
	}
	if got := cfg.Progress.Snapshot(); got.Embedded != 100 {
		t.Fatalf("progress = %v, want all 100 embedded", got)
	}
}

func TestAMaxPendingOfOneStillCompletes(t *testing.T) {
	var f fakeEmbedder
	var events recorded
	run, cfg := setup(t, &f, &events)
	cfg.MaxPending = 1

	if err := RunIngestion(t.Context(), run, oneLiners(t, 50), cfg); err != nil {
		t.Fatal(err)
	}
	if run.State() != domain.Completed || f.total != 50 {
		t.Fatalf("run %s with %d chunks embedded, want completed with 50", run.State(), f.total)
	}
}

func TestANegativeMaxPendingIsRejected(t *testing.T) {
	var f fakeEmbedder
	var events recorded
	run, cfg := setup(t, &f, &events)
	cfg.MaxPending = -1
	if err := RunIngestion(t.Context(), run, oneLiners(t, 1), cfg); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("err = %v, want ErrInvalidConfig", err)
	}
}

// lossyEmbedder breaks the contract: it returns no vector and no error.
type lossyEmbedder struct{}

func (lossyEmbedder) Embed(context.Context, []string) ([]shared.Vector, error) { return nil, nil }

func TestAnEmbedderThatBreaksItsContractFailsTheRun(t *testing.T) {
	var events recorded
	run, cfg := setup(t, new(fakeEmbedder), &events)
	cfg.Embedder = lossyEmbedder{}

	err := RunIngestion(t.Context(), run, documents(t, nil, "aaaa\n"), cfg)
	if !errors.Is(err, ErrBrokenEmbedder) {
		t.Fatalf("err = %v, want ErrBrokenEmbedder", err)
	}
	if run.State() != domain.Failed {
		t.Fatalf("run is %s, want failed: a lost vector is a bug, not a failed chunk", run.State())
	}
}

// TestTheLimiterBoundsTheCallsToTheEmbedder is RN04: with a limiter, the calls to the
// embedder at the same time never pass its limit, however many workers there are.
func TestTheLimiterBoundsTheCallsToTheEmbedder(t *testing.T) {
	var mu sync.Mutex
	var now, most int
	f := fakeEmbedder{}
	var events recorded
	run, cfg := setup(t, &f, &events)
	cfg.Workers = 8
	cfg.Embedder = embedderFunc(func(ctx context.Context, texts []string) ([]shared.Vector, error) {
		mu.Lock()
		now++
		most = max(most, now)
		mu.Unlock()
		time.Sleep(time.Millisecond) // a slow model: the workers pile up
		mu.Lock()
		now--
		mu.Unlock()
		return f.Embed(ctx, texts)
	})
	l, err := limiter.New(limiter.Config{Max: 2, Initial: 2})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Limiter = l

	if err := RunIngestion(t.Context(), run, oneLiners(t, 100), cfg); err != nil {
		t.Fatal(err)
	}
	if most > 2 {
		t.Fatalf("%d calls to the embedder at once, want at most the limit, 2 (RN04)", most)
	}
	if f.total != 100 {
		t.Fatalf("%d chunks embedded, want 100", f.total)
	}
}

// A failed chunk gives its permit back without a sample: a text over the context fails
// fast, and its latency would make the model look faster than it is.
func TestAFailedChunkGivesItsPermitBack(t *testing.T) {
	f := fakeEmbedder{fail: func(string) error { return errModel }}
	var events recorded
	run, cfg := setup(t, &f, &events)
	l, err := limiter.New(limiter.Config{Max: 4, Initial: 3})
	if err != nil {
		t.Fatal(err)
	}
	cfg.Limiter = l

	if err := RunIngestion(t.Context(), run, oneLiners(t, 50), cfg); err != nil {
		t.Fatal(err)
	}
	if got := cfg.Progress.Snapshot(); got.Failed != 50 {
		t.Fatalf("progress = %v, want all 50 failed", got)
	}
	if in, lim := l.InFlight(), l.Limit(); in != 0 || lim != 3 {
		t.Fatalf("%d permits out and limit %d after the run, want 0 and 3", in, lim)
	}
}

// embedderFunc is an Embedder made of a function.
type embedderFunc func(ctx context.Context, texts []string) ([]shared.Vector, error)

func (f embedderFunc) Embed(ctx context.Context, texts []string) ([]shared.Vector, error) {
	return f(ctx, texts)
}
