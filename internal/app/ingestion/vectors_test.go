package ingestion

import (
	"errors"
	"sync"
	"testing"

	domain "github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// memoryVectors is a VectorStore in memory; fail, when set, fails every Add.
type memoryVectors struct {
	mu   sync.Mutex
	kept map[shared.ChunkID]shared.Vector
	fail error
}

func (m *memoryVectors) Add(id shared.ChunkID, v shared.Vector) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.fail != nil {
		return m.fail
	}
	if m.kept == nil {
		m.kept = make(map[shared.ChunkID]shared.Vector)
	}
	m.kept[id] = v
	return nil
}

// Every chunk the journal confirms has its vector in the store, under its ID: what a search
// needs to tell which chunk a vector is.
func TestEveryConfirmedChunkHasItsVector(t *testing.T) {
	var f fakeEmbedder
	var events recorded
	var vectors memoryVectors
	run, cfg := setup(t, &f, &events)
	cfg.Vectors = &vectors
	if err := RunIngestion(t.Context(), run, documents(t, nil, "aaaa\nbbbb\ncccc\n", "dd\n"), cfg); err != nil {
		t.Fatal(err)
	}
	confirmed := 0
	for _, e := range events {
		if c, ok := e.(domain.ChunkEmbedded); ok {
			confirmed++
			if v, ok := vectors.kept[c.Chunk]; !ok || v.Dot(oneVector) != 1 {
				t.Errorf("chunk %s confirmed, its vector in the store: %v", c.Chunk, ok)
			}
		}
	}
	if confirmed != 3 || len(vectors.kept) != 3 {
		t.Errorf("%d chunks confirmed and %d vectors kept, want 3 and 3", confirmed, len(vectors.kept))
	}
}

// A vector that cannot be kept fails the run, like a journal that cannot append: the chunk
// is not confirmed, so a resumed run embeds it again.
func TestAVectorThatCannotBeKeptFailsTheRun(t *testing.T) {
	disk := errors.New("disk full")
	var f fakeEmbedder
	var events recorded
	run, cfg := setup(t, &f, &events)
	cfg.Vectors = &memoryVectors{fail: disk}
	if err := RunIngestion(t.Context(), run, documents(t, nil, "aaaa\n"), cfg); !errors.Is(err, disk) {
		t.Fatalf("RunIngestion: err = %v, want one that wraps %v", err, disk)
	}
	for _, e := range events {
		if _, ok := e.(domain.ChunkEmbedded); ok {
			t.Errorf("a chunk was confirmed without its vector: %+v", e)
		}
	}
}
