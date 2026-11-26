package ingestion

import (
	"errors"
	"slices"
	"testing"

	domain "github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

func TestOpenRunGivesAPendingRunForAnEmptyJournal(t *testing.T) {
	var j recorded
	run, err := OpenRun(&j, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if run.ID() != "run-1" || run.State() != domain.Pending || len(j) != 0 {
		t.Fatalf("run %q is %s with %d events in the journal, want run-1, pending, none", run.ID(), run.State(), len(j))
	}
}

// A process that dies while running leaves a running run in the journal: OpenRun pauses
// it, and the pause goes to the journal, so that the next run resumes it.
func TestOpenRunPausesARunThatACrashStopped(t *testing.T) {
	j := recorded{domain.RunStarted{Run: "run-1"}, domain.DocumentDiscovered{Run: "run-1", Path: "a.go"}}
	run, err := OpenRun(&j, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	if run.State() != domain.Paused || j[len(j)-1] != (domain.RunPaused{Run: "run-1"}) {
		t.Fatalf("run is %s, last event %+v; want paused and RunPaused", run.State(), j[len(j)-1])
	}
}

// crashed returns the journal of a run over docs that died after confirming two chunks:
// the events of a whole run, cut after the second ChunkEmbedded.
func crashed(t *testing.T, texts ...string) recorded {
	t.Helper()
	var f fakeEmbedder
	var whole recorded
	run, cfg := setup(t, &f, &whole)
	if err := RunIngestion(t.Context(), run, documents(t, nil, texts...), cfg); err != nil {
		t.Fatal(err)
	}
	confirmed := 0
	for i, e := range whole {
		if _, ok := e.(domain.ChunkEmbedded); ok {
			if confirmed++; confirmed == 2 {
				return slices.Clone(whole[:i+1])
			}
		}
	}
	t.Fatalf("the run confirmed %d chunks, want at least 2", confirmed)
	return nil
}

// RN02: a chunk confirmed in the journal is never embedded again nor confirmed twice.
func TestAResumedRunEmbedsOnlyWhatTheJournalLacks(t *testing.T) {
	texts := []string{"aaaa\nbbbb\ncccc\n", "dd\n", "eeee\n"} // 4 chunks: the first file has 2
	j := crashed(t, texts...)
	run, err := OpenRun(&j, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	var f fakeEmbedder
	_, cfg := setup(t, &f, &j)
	if err := RunIngestion(t.Context(), run, documents(t, nil, texts...), cfg); err != nil {
		t.Fatal(err)
	}
	if f.total != 2 {
		t.Fatalf("the resumed run embedded %d chunks, want the 2 the journal lacks", f.total)
	}
	if got := cfg.Progress.Snapshot(); got.Skipped != 2 || got.Embedded != 2 {
		t.Fatalf("progress %v, want 2 skipped and 2 embedded", got)
	}
	confirmed := make(map[shared.ChunkID]int)
	for _, e := range j {
		if c, ok := e.(domain.ChunkEmbedded); ok {
			confirmed[c.Chunk]++
		}
	}
	for id, n := range confirmed {
		if n != 1 {
			t.Errorf("chunk %s confirmed %d times in the journal", id, n)
		}
	}
	done := domain.RunCompleted{Run: "run-1", Documents: 3, Embedded: 4}
	if len(confirmed) != 4 || j[len(j)-1] != done {
		t.Fatalf("%d chunks confirmed, last event %+v; want 4 and %+v", len(confirmed), j[len(j)-1], done)
	}
}

func TestACompletedRunDoesNotRunAgain(t *testing.T) {
	var f fakeEmbedder
	var j recorded
	run, cfg := setup(t, &f, &j)
	docs := documents(t, nil, "aaaa\n")
	if err := RunIngestion(t.Context(), run, docs, cfg); err != nil {
		t.Fatal(err)
	}
	again, err := OpenRun(&j, "run-1")
	if err != nil {
		t.Fatal(err)
	}
	f.total = 0
	if err := RunIngestion(t.Context(), again, docs, cfg); !errors.Is(err, domain.ErrInvalidTransition) || f.total != 0 {
		t.Fatalf("running a completed run again: err = %v, %d chunks embedded; want ErrInvalidTransition and none", err, f.total)
	}
}

// brokenJournal takes ok appends and then fails every one.
type brokenJournal struct {
	recorded
	ok int
}

func (b *brokenJournal) Append(events []domain.Event) error {
	if b.ok == 0 {
		return errDisk
	}
	b.ok--
	return b.recorded.Append(events)
}

// An event that does not reach the journal is not durable: the run stops there.
func TestAJournalThatFailsFailsTheRun(t *testing.T) {
	var f fakeEmbedder
	var unused recorded
	run, cfg := setup(t, &f, &unused)
	j := &brokenJournal{ok: 3}
	cfg.Journal = j
	err := RunIngestion(t.Context(), run, documents(t, nil, "aaaa\nbbbb\ncccc\n", "dd\n"), cfg)
	if !errors.Is(err, errDisk) || run.State() != domain.Failed {
		t.Fatalf("err = %v, run %s; want errDisk and failed", err, run.State())
	}
	if len(j.recorded) != 3 {
		t.Fatalf("%d events in the journal, want the 3 appended before it broke", len(j.recorded))
	}
}
