package shared

import (
	"errors"
	"testing"
)

type location struct {
	path       string
	start, end int
}

// fakeFinder is a ChunkFinder backed by a map.
type fakeFinder map[ChunkID]location

func (f fakeFinder) FindChunk(id ChunkID) (path string, start, end int, ok bool) {
	l, ok := f[id]
	return l.path, l.start, l.end, ok
}

func newFinder(t *testing.T) (fakeFinder, ChunkID) {
	t.Helper()
	id, err := NewChunkID("sync/mutex.go", 0, 1, 30, "package sync")
	if err != nil {
		t.Fatal(err)
	}
	return fakeFinder{id: {"sync/mutex.go", 1, 30}}, id
}

func TestNewCitation(t *testing.T) {
	chunks, id := newFinder(t)
	tests := []struct {
		name       string
		start, end int
		want       string
	}{
		{"one line", 12, 12, "sync/mutex.go:12"},
		{"a range", 12, 20, "sync/mutex.go:12-20"},
		{"the whole chunk", 1, 30, "sync/mutex.go:1-30"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewCitation(chunks, id, tt.start, tt.end)
			if err != nil {
				t.Fatal(err)
			}
			if got := c.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
			if c.Chunk() != id {
				t.Errorf("Chunk() = %s, want %s", c.Chunk(), id)
			}
		})
	}
}

func TestNewCitationRejectsWhatIsNotInTheCorpus(t *testing.T) {
	chunks, id := newFinder(t)
	other, _ := NewChunkID("sync/once.go", 0, 1, 20, "package sync")
	tests := []struct {
		name       string
		id         ChunkID
		start, end int
	}{
		{"zero chunk ID", ChunkID{}, 1, 1},
		{"chunk not in the corpus", other, 1, 1},
		{"line past the end of the file", id, 1890, 1890},
		{"range that ends past the chunk", id, 20, 31},
		{"range that starts before the chunk", id, 0, 5},
		{"end before start", id, 20, 12},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c, err := NewCitation(chunks, tt.id, tt.start, tt.end)
			if !errors.Is(err, ErrInvalidCitation) {
				t.Errorf("err = %v, want one that wraps ErrInvalidCitation", err)
			}
			if !c.IsZero() {
				t.Errorf("c = %s, want the zero Citation", c)
			}
		})
	}
}
