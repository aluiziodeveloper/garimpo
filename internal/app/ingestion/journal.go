package ingestion

import (
	"iter"

	domain "github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
)

// Journal is the write-ahead log of the ingestion: the events of a run, appended in the
// order the run recorded them, kept across processes. A run that stopped is rebuilt by
// replaying them, and a chunk confirmed in the journal is never embedded again (RN02).
//
// When Append returns nil, the events are durable: a journal opened again, by this
// process or by the next one after a crash, replays them. Appending no events does
// nothing. Replay returns every event appended so far, oldest first, the ones of earlier
// processes included, and leaves them in the journal; an error ends it.
// journaltest.Contract checks all of this.
//
// Like the run, a Journal is used by one goroutine at a time: the run's owner.
type Journal interface {
	Append(events []domain.Event) error
	Replay() iter.Seq2[domain.Event, error]
}
