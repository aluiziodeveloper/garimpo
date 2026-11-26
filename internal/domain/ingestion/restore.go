package ingestion

import "fmt"

// RestoreRun rebuilds a run from the events it recorded, oldest first, as a journal keeps
// them: the events are the truth, and the run's state is what they add up to. Restoring
// is running again the command each event records, so the state machine checks the
// journal as it checked the run; the restored run records no events, since the journal
// has them already.
//
// The first event must be the RunStarted of the run, and every event must be of the same
// run. A journal that breaks any of this, or that the state machine rejects, was not
// written by a run: RestoreRun fails and returns no run.
func RestoreRun(events []Event) (*Run, error) {
	if len(events) == 0 {
		return nil, fmt.Errorf("%w: no events to restore", ErrInvalidRun)
	}
	first, ok := events[0].(RunStarted)
	if !ok {
		return nil, fmt.Errorf("%w: the first event is %T, want RunStarted", ErrInvalidRun, events[0])
	}
	r, err := NewRun(first.Run)
	if err != nil {
		return nil, err
	}
	for i, e := range events {
		if err := r.redo(e); err != nil {
			return nil, fmt.Errorf("event %d (%T): %w", i+1, e, err)
		}
	}
	r.events = nil
	return r, nil
}

// redo runs again the command that recorded e.
func (r *Run) redo(e Event) error {
	if id := runOf(e); id != r.id {
		return fmt.Errorf("%w: an event of run %q in the journal of run %q", ErrInvalidRun, id, r.id)
	}
	switch e := e.(type) {
	case RunStarted:
		return r.Start()
	case RunPaused:
		return r.Pause()
	case RunResumed:
		return r.Resume()
	case RunCompleted:
		return r.Complete()
	case RunFailed:
		return r.Fail(e.Reason)
	case DocumentDiscovered:
		return r.discover(e.Path)
	case ChunkEmbedded:
		return r.ConfirmChunk(e.Chunk)
	case ChunkFailed:
		return r.FailChunk(e.Chunk, e.Reason)
	}
	return fmt.Errorf("%w: unknown event %T", ErrInvalidRun, e)
}

// runOf returns the ID of the run that recorded e.
func runOf(e Event) string {
	switch e := e.(type) {
	case RunStarted:
		return e.Run
	case RunPaused:
		return e.Run
	case RunResumed:
		return e.Run
	case RunCompleted:
		return e.Run
	case RunFailed:
		return e.Run
	case DocumentDiscovered:
		return e.Run
	case ChunkEmbedded:
		return e.Run
	case ChunkFailed:
		return e.Run
	}
	return ""
}
