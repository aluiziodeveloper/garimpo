package shared

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
)

// ErrInvalidChunkID is wrapped by every error NewChunkID returns.
var ErrInvalidChunkID = errors.New("invalid chunk ID")

// ChunkID identifies a chunk of the corpus: the SHA-256 of its path, the byte where it
// starts in the file, its line range and its text. The same chunk always gets the same ID,
// in any run, which is what lets a resumed ingestion skip what it already confirmed (RN02).
// The byte offset tells apart the equal pieces of a long line, which share path, line and
// text (time/tzdata/zzipdata.go has two such pairs).
//
// The field is unexported: outside this package, the only way to get a non-zero ChunkID
// is NewChunkID. The zero value means "no chunk" and is safe to use.
type ChunkID struct {
	sum [sha256.Size]byte
}

// NewChunkID returns the ID of the chunk of path that starts at byte offset and spans the
// lines start to end (1-based, inclusive). The path must not be empty, offset must not be
// negative, start must be at least 1 and end at least start.
func NewChunkID(path string, offset, start, end int, text string) (ChunkID, error) {
	switch {
	case path == "":
		return ChunkID{}, fmt.Errorf("%w: empty path", ErrInvalidChunkID)
	case offset < 0:
		return ChunkID{}, fmt.Errorf("%w: byte offset %d", ErrInvalidChunkID, offset)
	case start < 1:
		return ChunkID{}, fmt.Errorf("%w: start line %d, lines start at 1", ErrInvalidChunkID, start)
	case end < start:
		return ChunkID{}, fmt.Errorf("%w: end line %d before start line %d", ErrInvalidChunkID, end, start)
	}
	// The NUL bytes keep the fields apart: without them, path "x1" at line 2 and path "x"
	// at line 12 would both hash "x12...".
	data := fmt.Appendf(nil, "%s\x00%d\x00%d\x00%d\x00%s", path, offset, start, end, text)
	return ChunkID{sum: sha256.Sum256(data)}, nil
}

// IsZero reports whether id is the zero ChunkID, which identifies no chunk.
func (id ChunkID) IsZero() bool {
	return id == ChunkID{}
}

// String returns the ID as 64 hexadecimal digits, or "" for the zero ChunkID.
func (id ChunkID) String() string {
	if id.IsZero() {
		return ""
	}
	return hex.EncodeToString(id.sum[:])
}
