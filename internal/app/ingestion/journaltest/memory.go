package journaltest

import (
	"iter"
	"slices"
	"sync"

	domain "github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
)

// Memory is a Journal in memory, for the tests: its events last as long as the Memory,
// and using the same Memory again is opening the same journal again. The zero value is an
// empty journal.
type Memory struct {
	mu      sync.Mutex
	events  []domain.Event
	appends int
}

// Append adds the events at the end.
func (m *Memory) Append(events []domain.Event) error {
	if len(events) == 0 {
		return nil
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.events = append(m.events, events...)
	m.appends++
	return nil
}

// Replay yields the events appended so far, oldest first. It yields a copy: an Append
// during the replay is not part of it.
func (m *Memory) Replay() iter.Seq2[domain.Event, error] {
	m.mu.Lock()
	events := slices.Clone(m.events)
	m.mu.Unlock()
	return func(yield func(domain.Event, error) bool) {
		for _, e := range events {
			if !yield(e, nil) {
				return
			}
		}
	}
}

// Appends returns how many calls to Append added events: a journal on disk pays a sync
// for each one.
func (m *Memory) Appends() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.appends
}
