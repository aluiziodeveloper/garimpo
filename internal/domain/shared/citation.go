package shared

import (
	"errors"
	"fmt"
)

// ErrInvalidCitation is wrapped by every error NewCitation returns.
var ErrInvalidCitation = errors.New("invalid citation")

// ChunkFinder finds where a chunk of the corpus is. It is declared here, where it is
// consumed; the index implements it.
type ChunkFinder interface {
	// FindChunk returns the path and the line range of the chunk, and false if the
	// corpus has no chunk with that ID.
	FindChunk(id ChunkID) (path string, start, end int, ok bool)
}

// Citation points to lines of a chunk that exists in the corpus (RN01). NewCitation is
// the only way to get a non-zero Citation, so an answer that holds one cites something
// the reader can open. The zero value means "no citation".
type Citation struct {
	chunk      ChunkID
	path       string
	start, end int
}

// NewCitation returns a citation of lines start to end of the chunk id. It fails if the
// corpus has no such chunk or if the lines are not inside it.
func NewCitation(chunks ChunkFinder, id ChunkID, start, end int) (Citation, error) {
	if id.IsZero() {
		return Citation{}, fmt.Errorf("%w: zero chunk ID", ErrInvalidCitation)
	}
	path, first, last, ok := chunks.FindChunk(id)
	if !ok {
		return Citation{}, fmt.Errorf("%w: chunk %s is not in the corpus", ErrInvalidCitation, id)
	}
	if start < first || end > last || end < start {
		return Citation{}, fmt.Errorf("%w: lines %d-%d are not inside %s:%d-%d",
			ErrInvalidCitation, start, end, path, first, last)
	}
	return Citation{chunk: id, path: path, start: start, end: end}, nil
}

// Chunk returns the ID of the cited chunk.
func (c Citation) Chunk() ChunkID {
	return c.chunk
}

// IsZero reports whether c is the zero Citation.
func (c Citation) IsZero() bool {
	return c == Citation{}
}

// String returns path:line, or path:start-end for a range, or "" for the zero Citation.
func (c Citation) String() string {
	switch {
	case c.IsZero():
		return ""
	case c.start == c.end:
		return fmt.Sprintf("%s:%d", c.path, c.start)
	}
	return fmt.Sprintf("%s:%d-%d", c.path, c.start, c.end)
}
