package ingestion

import (
	"errors"
	"fmt"
	"slices"
	"strings"
)

// ErrInvalidRun is wrapped by the errors of NewRun and of a Fail without a reason.
var ErrInvalidRun = errors.New("invalid ingestion run")

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
// Like the whole domain, a Run is sequential: one goroutine owns it.
type Run struct {
	id     string
	state  State
	reason string
}

// NewRun returns a pending run with the given ID, which must not be blank.
func NewRun(id string) (*Run, error) {
	if strings.TrimSpace(id) == "" {
		return nil, fmt.Errorf("%w: the ID is blank", ErrInvalidRun)
	}
	return &Run{id: id}, nil
}

// ID returns the run's ID.
func (r *Run) ID() string { return r.id }

// State returns the run's current state.
func (r *Run) State() State { return r.state }

// Reason returns why the run failed, or "" if it did not.
func (r *Run) Reason() string { return r.reason }

// Start begins a pending run.
func (r *Run) Start() error { return r.move(Running, Pending) }

// Pause stops a running run, which may resume later.
func (r *Run) Pause() error { return r.move(Paused, Running) }

// Resume continues a paused run.
func (r *Run) Resume() error { return r.move(Running, Paused) }

// Complete ends a running run that handled the whole corpus.
func (r *Run) Complete() error { return r.move(Completed, Running) }

// Fail ends a running or paused run because of an error; reason says which.
func (r *Run) Fail(reason string) error {
	if reason == "" {
		return fmt.Errorf("%w: a failure needs a reason", ErrInvalidRun)
	}
	if err := r.move(Failed, Running, Paused); err != nil {
		return err
	}
	r.reason = reason
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
