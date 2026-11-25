package ingestion

import (
	"errors"
	"testing"
)

// action is one of the transitions a Run accepts, by name, for the tables below.
type action struct {
	name string
	do   func(*Run) error
}

var actions = []action{
	{"start", (*Run).Start},
	{"pause", (*Run).Pause},
	{"resume", (*Run).Resume},
	{"complete", (*Run).Complete},
	{"fail", func(r *Run) error { return r.Fail("disk gone") }},
}

// allowed is the state machine: from each state, the actions that are legal and the state
// each one leads to. Every pair missing here must be rejected.
var allowed = map[State]map[string]State{
	Pending:   {"start": Running},
	Running:   {"pause": Paused, "complete": Completed, "fail": Failed},
	Paused:    {"resume": Running, "fail": Failed},
	Completed: {},
	Failed:    {},
}

// runIn returns a run driven to state through legal transitions.
func runIn(t *testing.T, state State) *Run {
	t.Helper()
	r, err := NewRun("run-1")
	if err != nil {
		t.Fatal(err)
	}
	path := map[State][]func(*Run) error{
		Pending:   nil,
		Running:   {(*Run).Start},
		Paused:    {(*Run).Start, (*Run).Pause},
		Completed: {(*Run).Start, (*Run).Complete},
		Failed:    {(*Run).Start, func(r *Run) error { return r.Fail("disk gone") }},
	}[state]
	for _, step := range path {
		if err := step(r); err != nil {
			t.Fatalf("driving the run to %s: %v", state, err)
		}
	}
	return r
}

func TestNewRunIsPending(t *testing.T) {
	r, err := NewRun("run-1")
	if err != nil {
		t.Fatal(err)
	}
	if r.ID() != "run-1" || r.State() != Pending {
		t.Fatalf("NewRun = %s %s, want run-1 pending", r.ID(), r.State())
	}
}

func TestNewRunRejectsAnEmptyID(t *testing.T) {
	for _, id := range []string{"", "  "} {
		if _, err := NewRun(id); !errors.Is(err, ErrInvalidRun) {
			t.Errorf("NewRun(%q) error = %v, want ErrInvalidRun", id, err)
		}
	}
}

func TestRunAllowsOnlyTheTransitionsOfItsStateMachine(t *testing.T) {
	for from := Pending; from <= Failed; from++ {
		legal := allowed[from]
		for _, a := range actions {
			t.Run(from.String()+"/"+a.name, func(t *testing.T) {
				r := runIn(t, from)
				err := a.do(r)
				to, ok := legal[a.name]
				switch {
				case ok && err != nil:
					t.Fatalf("%s from %s: %v, want %s", a.name, from, err, to)
				case ok && r.State() != to:
					t.Fatalf("%s from %s left the run %s, want %s", a.name, from, r.State(), to)
				case !ok && !errors.Is(err, ErrInvalidTransition):
					t.Fatalf("%s from %s: error = %v, want ErrInvalidTransition", a.name, from, err)
				case !ok && r.State() != from:
					t.Fatalf("a rejected %s moved the run from %s to %s", a.name, from, r.State())
				}
			})
		}
	}
}

func TestFailKeepsTheReason(t *testing.T) {
	r := runIn(t, Running)
	if err := r.Fail("disk gone"); err != nil {
		t.Fatal(err)
	}
	if r.Reason() != "disk gone" {
		t.Fatalf("Reason() = %q, want %q", r.Reason(), "disk gone")
	}
}

func TestFailNeedsAReason(t *testing.T) {
	r := runIn(t, Running)
	if err := r.Fail(""); !errors.Is(err, ErrInvalidRun) {
		t.Fatalf("Fail(\"\") error = %v, want ErrInvalidRun", err)
	}
	if r.State() != Running {
		t.Fatalf("a rejected Fail left the run %s, want running", r.State())
	}
}
