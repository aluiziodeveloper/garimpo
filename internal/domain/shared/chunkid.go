package shared

import (
	"bytes"
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

// Compare returns -1, 0 or +1 as id comes before, is equal to or comes after other. The
// order is the order of String, and the zero ChunkID comes first. It means nothing about
// the chunks: it is a fixed order, so that a ranking of hits with equal scores comes out
// the same in every run and from every index.
func (id ChunkID) Compare(other ChunkID) int {
	return bytes.Compare(id.sum[:], other.sum[:])
}

// String returns the ID as 64 hexadecimal digits, or "" for the zero ChunkID.
func (id ChunkID) String() string {
	if id.IsZero() {
		return ""
	}
	return hex.EncodeToString(id.sum[:])
}

// ChunkIDSize is the length of a ChunkID in bytes, as AppendBinary writes it.
const ChunkIDSize = sha256.Size

// AppendBinary appends the 32 bytes of id to b, the way to store an ID in a binary file
// (encoding.BinaryAppender). It fails for the zero ChunkID, which identifies no chunk.
func (id ChunkID) AppendBinary(b []byte) ([]byte, error) {
	if id.IsZero() {
		return b, fmt.Errorf("%w: the zero ChunkID", ErrInvalidChunkID)
	}
	return append(b, id.sum[:]...), nil
}

// UnmarshalBinary sets id to the ChunkID whose AppendBinary wrote data: ChunkIDSize bytes,
// not all zero (encoding.BinaryUnmarshaler).
func (id *ChunkID) UnmarshalBinary(data []byte) error {
	var back ChunkID
	if len(data) != len(back.sum) {
		return fmt.Errorf("%w: %d bytes, want %d", ErrInvalidChunkID, len(data), len(back.sum))
	}
	copy(back.sum[:], data)
	if back.IsZero() {
		return fmt.Errorf("%w: %d zero bytes", ErrInvalidChunkID, len(data))
	}
	*id = back
	return nil
}

// ParseChunkID returns the ChunkID whose String is s: 64 lower-case hexadecimal digits,
// not all zero. It is the way back for an ID stored as text, such as in the journal.
func ParseChunkID(s string) (ChunkID, error) {
	var id ChunkID
	if len(s) != 2*len(id.sum) {
		return ChunkID{}, fmt.Errorf("%w: %d characters, want %d", ErrInvalidChunkID, len(s), 2*len(id.sum))
	}
	if _, err := hex.Decode(id.sum[:], []byte(s)); err != nil {
		return ChunkID{}, fmt.Errorf("%w: %w", ErrInvalidChunkID, err)
	}
	// hex.Decode takes upper case, and 64 zeros decode to the zero ChunkID. String writes
	// neither (lower case only, "" for the zero ID), so comparing with it rejects both.
	if id.String() != s {
		return ChunkID{}, fmt.Errorf("%w: %q is not in canonical form", ErrInvalidChunkID, s)
	}
	return id, nil
}
