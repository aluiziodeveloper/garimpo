package ingestion

import (
	"errors"
	"reflect"
	"testing"

	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// chunkID returns the ID of line of a.go, as a one-line chunk.
func chunkID(t *testing.T, line int) shared.ChunkID {
	t.Helper()
	id, err := shared.NewChunkID("a.go", 0, line, line, "x\n")
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRunRecordsWhatHappenedInOrder(t *testing.T) {
	r := runIn(t, Pending)
	a, b := chunkID(t, 1), chunkID(t, 2)
	steps := []func() error{
		r.Start,
		func() error { return r.DiscoverDocument(file(t, "x\n")) },
		func() error { return r.ConfirmChunk(a) },
		r.Pause,
		func() error { return r.FailChunk(b, "model timed out") },
		r.Resume,
		r.Complete,
	}
	for i, step := range steps {
		if err := step(); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
	}
	want := []Event{
		RunStarted{Run: "run-1"},
		DocumentDiscovered{Run: "run-1", Path: "x.go"},
		ChunkEmbedded{Run: "run-1", Chunk: a},
		RunPaused{Run: "run-1"},
		ChunkFailed{Run: "run-1", Chunk: b, Reason: "model timed out"},
		RunResumed{Run: "run-1"},
		RunCompleted{Run: "run-1", Documents: 1, Embedded: 1, Failed: 1},
	}
	if got := r.TakeEvents(); !reflect.DeepEqual(got, want) {
		t.Fatalf("events:\n got  %+v\n want %+v", got, want)
	}
}

func TestTakeEventsEmptiesTheList(t *testing.T) {
	r := runIn(t, Running)
	if got := r.TakeEvents(); len(got) != 1 {
		t.Fatalf("first TakeEvents = %+v, want RunStarted only", got)
	}
	if got := r.TakeEvents(); len(got) != 0 {
		t.Fatalf("second TakeEvents = %+v, want nothing: the events were taken", got)
	}
}

func TestRunFailedCarriesTheReason(t *testing.T) {
	r := runIn(t, Running)
	r.TakeEvents()
	if err := r.Fail("corpus unreadable"); err != nil {
		t.Fatal(err)
	}
	want := []Event{RunFailed{Run: "run-1", Reason: "corpus unreadable"}}
	if got := r.TakeEvents(); !reflect.DeepEqual(got, want) {
		t.Fatalf("events = %+v, want %+v", got, want)
	}
}

func TestARejectedCommandRecordsNothing(t *testing.T) {
	r := runIn(t, Completed)
	r.TakeEvents()
	if err := r.Pause(); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("Pause of a completed run: %v, want ErrInvalidTransition", err)
	}
	if err := r.ConfirmChunk(chunkID(t, 1)); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("ConfirmChunk on a completed run: %v, want ErrInvalidTransition", err)
	}
	if got := r.TakeEvents(); len(got) != 0 {
		t.Fatalf("events = %+v, want nothing", got)
	}
}

func TestOnlyARunningRunDiscoversDocuments(t *testing.T) {
	r := runIn(t, Paused)
	if err := r.DiscoverDocument(file(t, "x\n")); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("DiscoverDocument on a paused run: %v, want ErrInvalidTransition", err)
	}
}

func TestAChunkIsHandledOnce(t *testing.T) {
	r := runIn(t, Running)
	id := chunkID(t, 1)
	if err := r.ConfirmChunk(id); err != nil {
		t.Fatal(err)
	}
	if err := r.ConfirmChunk(id); !errors.Is(err, ErrChunkAlreadyHandled) {
		t.Errorf("second ConfirmChunk: %v, want ErrChunkAlreadyHandled", err)
	}
	if err := r.FailChunk(id, "late timeout"); !errors.Is(err, ErrChunkAlreadyHandled) {
		t.Errorf("FailChunk after ConfirmChunk: %v, want ErrChunkAlreadyHandled", err)
	}
	if err := r.ConfirmChunk(shared.ChunkID{}); !errors.Is(err, ErrInvalidRun) {
		t.Errorf("ConfirmChunk of the zero ID: %v, want ErrInvalidRun", err)
	}
}
