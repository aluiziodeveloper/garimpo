package journaltest_test

import (
	"testing"

	"github.com/aluiziodeveloper/garimpo/internal/app/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/app/ingestion/journaltest"
)

func TestMemoryMeetsTheContract(t *testing.T) {
	journaltest.Contract(t, func(*testing.T) func(*testing.T) ingestion.Journal {
		m := new(journaltest.Memory)
		return func(*testing.T) ingestion.Journal { return m } // opened again: the same events
	})
}
