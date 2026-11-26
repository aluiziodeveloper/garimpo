// Package journaltest holds what every Journal is tested with: the contract suite and a
// journal in memory that passes it.
package journaltest

import (
	"io"
	"reflect"
	"testing"

	"github.com/aluiziodeveloper/garimpo/internal/app/ingestion"
	domain "github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// Contract runs the tests every ingestion.Journal must pass. newPlace returns a new, empty
// place for a journal (a file, a store) and open, which opens the journal of that place;
// calling open again opens the same journal again, as the next process would. Each
// subtest asks for its own place.
func Contract(t *testing.T, newPlace func(t *testing.T) (open func(t *testing.T) ingestion.Journal)) {
	t.Run("ANewJournalReplaysNothing", func(t *testing.T) {
		open := newPlace(t)
		if got := replay(t, open(t)); len(got) != 0 {
			t.Fatalf("a new journal replayed %d events: %v", len(got), got)
		}
	})

	t.Run("ReplayGivesTheEventsInTheOrderTheyWereAppended", func(t *testing.T) {
		j := newPlace(t)(t)
		all := Events(t)
		appendEvents(t, j, all[:3])
		appendEvents(t, j, all[3:])
		if got := replay(t, j); !reflect.DeepEqual(got, all) {
			t.Fatalf("replayed\n%v\nwant\n%v", got, all)
		}
	})

	t.Run("EveryEventKeepsItsTypeAndFields", func(t *testing.T) {
		j := newPlace(t)(t)
		for _, e := range Events(t) {
			appendEvents(t, j, []domain.Event{e})
		}
		got := replay(t, j)
		for i, e := range Events(t) {
			if i >= len(got) || got[i] != e {
				t.Errorf("event %d: replayed %#v, want %#v", i, at(got, i), e)
			}
		}
	})

	t.Run("AJournalOpenedAgainReplaysWhatWasAppended", func(t *testing.T) {
		open := newPlace(t)
		all := Events(t)
		first := open(t)
		appendEvents(t, first, all[:4])
		closeJournal(t, first)
		second := open(t)
		appendEvents(t, second, all[4:])
		closeJournal(t, second)
		if got := replay(t, open(t)); !reflect.DeepEqual(got, all) {
			t.Fatalf("replayed after opening again\n%v\nwant\n%v", got, all)
		}
	})

	t.Run("AppendingNoEventsAddsNothing", func(t *testing.T) {
		j := newPlace(t)(t)
		appendEvents(t, j, nil)
		if got := replay(t, j); len(got) != 0 {
			t.Fatalf("replayed %d events after appending none", len(got))
		}
	})

	t.Run("ReplayDoesNotConsumeTheEvents", func(t *testing.T) {
		j := newPlace(t)(t)
		appendEvents(t, j, Events(t))
		first, second := replay(t, j), replay(t, j)
		if !reflect.DeepEqual(first, second) || len(second) != len(Events(t)) {
			t.Fatalf("two replays gave %d and %d events, want %d each", len(first), len(second), len(Events(t)))
		}
	})
}

// Events returns one event of each type, every field set, in an order a run could have
// recorded them.
func Events(t *testing.T) []domain.Event {
	t.Helper()
	id := func(offset int) shared.ChunkID {
		c, err := shared.NewChunkID("encoding/json/decode.go", offset, 1, 9, "package json")
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	return []domain.Event{
		domain.RunStarted{Run: "r1"},
		domain.DocumentDiscovered{Run: "r1", Path: "encoding/json/decode.go"},
		domain.ChunkEmbedded{Run: "r1", Chunk: id(0)},
		domain.RunPaused{Run: "r1"},
		domain.RunResumed{Run: "r1"},
		domain.ChunkFailed{Run: "r1", Chunk: id(700), Reason: "text over the model's context: 600 tokens"},
		domain.RunCompleted{Run: "r1", Documents: 1, Embedded: 1, Failed: 1},
		domain.RunFailed{Run: "r2", Reason: "read encoding/json: permission denied"},
	}
}

func appendEvents(t *testing.T, j ingestion.Journal, events []domain.Event) {
	t.Helper()
	if err := j.Append(events); err != nil {
		t.Fatalf("Append(%d events): %v", len(events), err)
	}
}

func replay(t *testing.T, j ingestion.Journal) []domain.Event {
	t.Helper()
	var got []domain.Event
	for e, err := range j.Replay() {
		if err != nil {
			t.Fatalf("Replay after %d events: %v", len(got), err)
		}
		got = append(got, e)
	}
	return got
}

// closeJournal closes j if it holds something to close, such as a file.
func closeJournal(t *testing.T, j ingestion.Journal) {
	t.Helper()
	if c, ok := j.(io.Closer); ok {
		if err := c.Close(); err != nil {
			t.Fatalf("Close: %v", err)
		}
	}
}

func at(events []domain.Event, i int) domain.Event {
	if i < len(events) {
		return events[i]
	}
	return nil
}
