package ingestion

import (
	"testing"
	"time"

	domain "github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
)

// slowJournal is a journal in memory whose appends take a millisecond, as a sync on a disk
// does (labs/fsync), and are counted.
type slowJournal struct {
	recorded
	appends int
}

func (s *slowJournal) Append(events []domain.Event) error {
	if len(events) == 0 {
		return nil
	}
	time.Sleep(time.Millisecond)
	s.appends++
	return s.recorded.Append(events)
}

// Group commit: while one append waits for the disk, the changes the stages send queue up,
// and the next append takes the events of all of them. One sync per group, not per event.
func TestChangesWaitingTogetherShareOneAppend(t *testing.T) {
	var f fakeEmbedder
	var unused recorded
	run, cfg := setup(t, &f, &unused)
	j := new(slowJournal)
	cfg.Journal = j
	texts := make([]string, 200)
	for i := range texts {
		texts[i] = "aaaa\n"
	}
	if err := RunIngestion(t.Context(), run, documents(t, nil, texts...), cfg); err != nil {
		t.Fatal(err)
	}
	events := j.recorded
	if len(events) != 1+200+200+1 || events[len(events)-1] != (domain.RunCompleted{Run: "run-1", Documents: 200, Embedded: 200}) {
		t.Fatalf("%d events, last %+v; want 402, ending with the run completed", len(events), events[len(events)-1])
	}
	if j.appends*2 > len(events) {
		t.Fatalf("%d appends for %d events, want at most half: one per group of changes waiting together", j.appends, len(events))
	}
	t.Logf("%d appends for %d events", j.appends, len(events))
}
