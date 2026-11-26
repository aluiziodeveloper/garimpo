package ingestion

import (
	"errors"
	"slices"
	"testing"

	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// history returns the events of a run that started, read x.go, confirmed one chunk,
// failed another and paused, with the IDs of the two chunks.
func history(t *testing.T) (events []Event, confirmed, failed shared.ChunkID) {
	t.Helper()
	r := runIn(t, Running)
	a, b := chunkID(t, 1), chunkID(t, 2)
	for _, step := range []func() error{
		func() error { return r.DiscoverDocument(file(t, "x\n")) },
		func() error { return r.ConfirmChunk(a) },
		func() error { return r.FailChunk(b, "model timed out") },
		r.Pause,
	} {
		if err := step(); err != nil {
			t.Fatal(err)
		}
	}
	return r.TakeEvents(), a, b // RunStarted first: runIn started the run
}

func TestRestoreRunRebuildsTheRunFromItsEvents(t *testing.T) {
	events, confirmed, failed := history(t)
	r, err := RestoreRun(events)
	if err != nil {
		t.Fatal(err)
	}
	if r.ID() != "run-1" || r.State() != Paused {
		t.Fatalf("restored run %q is %s, want run-1, paused", r.ID(), r.State())
	}
	if got := slices.Collect(r.HandledChunks()); len(got) != 2 ||
		!slices.Contains(got, confirmed) || !slices.Contains(got, failed) {
		t.Fatalf("handled chunks %v, want the confirmed and the failed one", got)
	}
	if evs := r.TakeEvents(); len(evs) != 0 {
		t.Fatalf("restoring recorded %d events, want none: they are in the journal already", len(evs))
	}
	if err := r.Resume(); err != nil {
		t.Fatal(err)
	}
	if err := r.ConfirmChunk(confirmed); !errors.Is(err, ErrChunkAlreadyHandled) {
		t.Fatalf("confirming a chunk of the journal again: err = %v, want ErrChunkAlreadyHandled", err)
	}
	if err := r.Complete(); err != nil {
		t.Fatal(err)
	}
	done := RunCompleted{Run: "run-1", Documents: 1, Embedded: 1, Failed: 1}
	if evs := r.TakeEvents(); evs[len(evs)-1] != done {
		t.Fatalf("last event %+v, want %+v: the counts of the journal go on", evs[len(evs)-1], done)
	}
}

// A journal the state machine rejects was not written by a run: restoring it fails, and
// nothing is rebuilt from it.
func TestRestoreRunRejectsWhatNoRunRecords(t *testing.T) {
	events, confirmed, _ := history(t)
	for _, tc := range []struct {
		name   string
		events []Event
		want   error
	}{
		{"no events", nil, ErrInvalidRun},
		{"not started first", events[1:], ErrInvalidRun},
		{"another run's event", append(slices.Clone(events), RunResumed{Run: "run-2"}), ErrInvalidRun},
		{"a chunk after the end", []Event{RunStarted{Run: "run-1"}, RunCompleted{Run: "run-1"}, ChunkEmbedded{Run: "run-1", Chunk: confirmed}}, ErrInvalidTransition},
		{"a chunk twice", append(slices.Clone(events[:3]), ChunkEmbedded{Run: "run-1", Chunk: confirmed}), ErrChunkAlreadyHandled},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := RestoreRun(tc.events)
			if !errors.Is(err, tc.want) || r != nil {
				t.Fatalf("RestoreRun = %v, %v; want no run and %v", r, err, tc.want)
			}
		})
	}
}

// A resumed run reads the corpus again: a document it had discovered is not counted twice.
func TestADocumentDiscoveredAgainIsNotCountedAgain(t *testing.T) {
	r := runIn(t, Running)
	for range 2 {
		if err := r.DiscoverDocument(file(t, "x\n")); err != nil {
			t.Fatal(err)
		}
	}
	if err := r.Complete(); err != nil {
		t.Fatal(err)
	}
	evs := r.TakeEvents()
	if len(evs) != 3 || evs[2] != (RunCompleted{Run: "run-1", Documents: 1}) {
		t.Fatalf("events %+v, want started, one DocumentDiscovered and 1 document at the end", evs)
	}
}
