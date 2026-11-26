package ingestion

import (
	"fmt"
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

// OpenRun returns the run of the journal j: a new pending run with the ID id when j is
// empty, or the run its events rebuild. A run that was running when its process died is
// paused, and the pause is appended to j: RunIngestion resumes a paused run.
func OpenRun(j Journal, id string) (*domain.Run, error) {
	var events []domain.Event
	for e, err := range j.Replay() {
		if err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	if len(events) == 0 {
		return domain.NewRun(id)
	}
	run, err := domain.RestoreRun(events)
	if err != nil {
		return nil, fmt.Errorf("restore the run of the journal: %w", err)
	}
	if run.State() == domain.Running {
		if err := run.Pause(); err != nil {
			return nil, err
		}
		if err := j.Append(run.TakeEvents()); err != nil {
			return nil, err
		}
	}
	return run, nil
}
