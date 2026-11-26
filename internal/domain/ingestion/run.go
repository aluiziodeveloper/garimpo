package ingestion

import (
	"errors"
	"fmt"
	"iter"
	"maps"
	"slices"
	"strings"

	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// ErrInvalidRun is wrapped by the errors of commands with an invalid argument: a blank ID,
// a failure without a reason, the zero ChunkID.
var ErrInvalidRun = errors.New("invalid ingestion run")

// ErrChunkAlreadyHandled is wrapped when a run is told twice about the same chunk.
var ErrChunkAlreadyHandled = errors.New("chunk already handled")

// ErrInvalidTransition is wrapped by every transition the state machine rejects.
var ErrInvalidTransition = errors.New("invalid ingestion run transition")

// State is where an ingestion run is in its life.
type State int

// The states of a run. Completed and Failed are final: no transition leaves them.
const (
	Pending   State = iota // created, not started yet
	Running                // reading, cutting and embedding
	Paused                 // stopped by request; it may resume
	Completed              // every chunk of the corpus was handled
	Failed                 // stopped by an error; Reason says which
)

var stateNames = [...]string{"pending", "running", "paused", "completed", "failed"}

func (s State) String() string {
	if s < 0 || int(s) >= len(stateNames) {
		return fmt.Sprintf("State(%d)", int(s))
	}
	return stateNames[s]
}

// Run is the IngestionRun aggregate: one pass of the ingestion over a corpus, from
// pending to completed or failed. Its state changes only through its methods, and each
// method checks the state machine first:
//
//	pending --Start--> running --Complete--> completed
//	                   running --Pause--> paused --Resume--> running
//	                   running, paused --Fail--> failed
//
// Each accepted command records an Event; a rejected one records nothing. Documents are
// discovered only while running; chunks still in flight may be confirmed or failed while
// paused, and each chunk is handled once.
//
// Like the whole domain, a Run is sequential: one goroutine owns it.
type Run struct {
	id     string
	state  State
	reason string

	documents, embedded, failed int
	discovered                  map[string]struct{} // by path
	handled                     map[shared.ChunkID]struct{}
	events                      []Event
}

// NewRun returns a pending run with the given ID, which must not be blank.
func NewRun(id string) (*Run, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("%w: the ID is blank", ErrInvalidRun)
	}
	return &Run{id: id, discovered: make(map[string]struct{}), handled: make(map[shared.ChunkID]struct{})}, nil
}

// ID returns the run's ID.
func (r *Run) ID() string { return r.id }

// State returns the run's current state.
func (r *Run) State() State { return r.state }

// Reason returns why the run failed, or "" if it did not.
func (r *Run) Reason() string { return r.reason }

// Start begins a pending run.
func (r *Run) Start() error {
	return r.moveAndRecord(RunStarted{Run: r.id}, Running, Pending)
}

// Pause stops a running run, which may resume later.
func (r *Run) Pause() error {
	return r.moveAndRecord(RunPaused{Run: r.id}, Paused, Running)
}

// Resume continues a paused run.
func (r *Run) Resume() error {
	return r.moveAndRecord(RunResumed{Run: r.id}, Running, Paused)
}

// Complete ends a running run that handled the whole corpus.
func (r *Run) Complete() error {
	done := RunCompleted{Run: r.id, Documents: r.documents, Embedded: r.embedded, Failed: r.failed}
	return r.moveAndRecord(done, Completed, Running)
}

// Fail ends a running or paused run because of an error; reason says which.
func (r *Run) Fail(reason string) error {
	if reason == "" {
		return fmt.Errorf("%w: a failure needs a reason", ErrInvalidRun)
	}
	if err := r.moveAndRecord(RunFailed{Run: r.id, Reason: reason}, Failed, Running, Paused); err != nil {
		return err
	}
	r.reason = reason
	return nil
}

// DiscoverDocument records a document the running run read from the corpus. A run that
// resumes reads the corpus again: a document it discovered already is not recorded twice.
func (r *Run) DiscoverDocument(d Document) error {
	return r.discover(d.Path())
}

func (r *Run) discover(path string) error {
	if err := r.require(Running); err != nil {
		return err
	}
	if _, seen := r.discovered[path]; seen {
		return nil
	}
	r.discovered[path] = struct{}{}
	r.documents++
	r.events = append(r.events, DocumentDiscovered{Run: r.id, Path: path})
	return nil
}

// ConfirmChunk records that the chunk id got its vector.
func (r *Run) ConfirmChunk(id shared.ChunkID) error {
	if err := r.handle(id); err != nil {
		return err
	}
	r.embedded++
	r.events = append(r.events, ChunkEmbedded{Run: r.id, Chunk: id})
	return nil
}

// FailChunk records that the embedder gave up on the chunk id; reason says why.
func (r *Run) FailChunk(id shared.ChunkID, reason string) error {
	if err := r.handle(id); err != nil {
		return err
	}
	r.failed++
	r.events = append(r.events, ChunkFailed{Run: r.id, Chunk: id, Reason: reason})
	return nil
}

// HandledChunks yields the chunks the run confirmed or failed, in no order: the ones a
// resumed run skips (RN02).
func (r *Run) HandledChunks() iter.Seq[shared.ChunkID] {
	return maps.Keys(r.handled)
}

// TakeEvents returns the events recorded since the last call, oldest first, and forgets
// them: whoever takes them is now responsible for them.
func (r *Run) TakeEvents() []Event {
	events := r.events
	r.events = nil
	return events
}

// handle marks the chunk id as handled, if the run accepts chunks and has not seen it.
func (r *Run) handle(id shared.ChunkID) error {
	switch err := r.require(Running, Paused); {
	case err != nil:
		return err
	case id.IsZero():
		return fmt.Errorf("%w: the zero ChunkID identifies no chunk", ErrInvalidRun)
	}
	if _, seen := r.handled[id]; seen {
		return fmt.Errorf("%w: %s", ErrChunkAlreadyHandled, id)
	}
	r.handled[id] = struct{}{}
	return nil
}

// require returns an error unless the run is in one of the states.
func (r *Run) require(states ...State) error {
	if !slices.Contains(states, r.state) {
		return fmt.Errorf("%w: a %s run does not take this command", ErrInvalidTransition, r.state)
	}
	return nil
}

// moveAndRecord moves the run to to, as move does, and records e if it moved.
func (r *Run) moveAndRecord(e Event, to State, from ...State) error {
	if err := r.move(to, from...); err != nil {
		return err
	}
	r.events = append(r.events, e)
	return nil
}

// move changes the state to to, if the current state is one of from.
func (r *Run) move(to State, from ...State) error {
	if !slices.Contains(from, r.state) {
		return fmt.Errorf("%w: %s to %s", ErrInvalidTransition, r.state, to)
	}
	r.state = to
	return nil
}
