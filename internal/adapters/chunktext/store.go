// Package chunktext finds the text of a chunk of the corpus by its ID, for what a search
// shows and an answer cites (module 14).
//
// It keeps where each chunk is, a few dozen bytes per chunk, and reads its text from the
// corpus only when asked. A text read stays shared while some caller holds it: the store
// points to it with a weak pointer, so it costs no memory once nobody holds it, and
// runtime.AddCleanup takes its entry out after the GC frees it.
package chunktext

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"runtime"
	"sync"
	"sync/atomic"
	"weak"

	"github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

var (
	// ErrUnknownChunk is wrapped when the store has no chunk with the ID asked for.
	ErrUnknownChunk = errors.New("unknown chunk")
	// ErrDuplicateChunk is wrapped when a chunk is added twice.
	ErrDuplicateChunk = errors.New("chunk already added")
	// ErrChangedChunk is wrapped when the text read from the corpus is no longer the
	// chunk's: the file changed after the chunk was added.
	ErrChangedChunk = errors.New("the chunk changed in the corpus")
)

// Text is the text of a chunk. Holding the *Text keeps it in the store, for every caller;
// a string taken from it with String does not, which is why the store hands out *Text.
type Text struct{ s string }

// String returns the text.
func (t *Text) String() string { return t.s }

// place is where a chunk is in the corpus.
type place struct {
	path           string
	offset, length int
	start, end     int
}

// Store finds the texts of the chunks of a corpus. It is safe for concurrent calls.
type Store struct {
	fsys  fs.FS
	reads atomic.Int64

	mu     sync.Mutex
	places map[shared.ChunkID]place
	texts  map[shared.ChunkID]weak.Pointer[Text]
}

var _ shared.ChunkFinder = (*Store)(nil)

// New returns an empty store of the corpus in fsys.
func New(fsys fs.FS) *Store {
	return &Store{
		fsys:   fsys,
		places: make(map[shared.ChunkID]place),
		texts:  make(map[shared.ChunkID]weak.Pointer[Text]),
	}
}

// Add keeps where the chunk c is, but not its text.
func (s *Store) Add(c ingestion.Chunk) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.places[c.ID()]; ok {
		return fmt.Errorf("%w: %s", ErrDuplicateChunk, c.ID())
	}
	s.places[c.ID()] = place{path: c.Path(), offset: c.Offset(), length: len(c.Text()), start: c.Start(), end: c.End()}
	return nil
}

// FindChunk returns the path and the line range of the chunk id, and false if the store
// has no such chunk.
func (s *Store) FindChunk(id shared.ChunkID) (path string, start, end int, ok bool) {
	s.mu.Lock()
	p, ok := s.places[id]
	s.mu.Unlock()
	return p.path, p.start, p.end, ok
}

// Text returns the text of the chunk id: the one another caller holds, if any, or else
// the one it reads from the corpus. It fails for a chunk not added, a file it cannot read,
// or a file that no longer holds the chunk.
func (s *Store) Text(id shared.ChunkID) (*Text, error) {
	s.mu.Lock()
	p, ok := s.places[id]
	t := s.texts[id].Value() // nil when there is no entry, or when the GC took the text
	s.mu.Unlock()
	switch {
	case !ok:
		return nil, fmt.Errorf("%w: %s", ErrUnknownChunk, id)
	case t != nil:
		return t, nil
	}
	read, err := s.read(id, p)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if t := s.texts[id].Value(); t != nil {
		return t, nil // another caller read it meanwhile: share theirs
	}
	w := weak.Make(read)
	s.texts[id] = w
	// The cleanup's argument must not reach read, or read would never be freed.
	runtime.AddCleanup(read, s.forget, entry{id, w})
	return read, nil
}

// read reads the text of chunk id from the corpus, only its bytes when the file can be
// read at an offset, and checks that it is the chunk's.
func (s *Store) read(id shared.ChunkID, p place) (*Text, error) {
	s.reads.Add(1)
	text, err := readAt(s.fsys, p)
	if err != nil {
		return nil, fmt.Errorf("chunk %s: %w", id, err)
	}
	if again, err := shared.NewChunkID(p.path, p.offset, p.start, p.end, text); err != nil || again != id {
		return nil, fmt.Errorf("%w: %s:%d-%d", ErrChangedChunk, p.path, p.start, p.end)
	}
	return &Text{s: text}, nil
}

// readAt returns the bytes of the file at p: only them, with io.ReaderAt, or else a copy
// of them from the whole file. A substring of the whole file would keep it all alive for
// as long as the text.
func readAt(fsys fs.FS, p place) (string, error) {
	f, err := fsys.Open(p.path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }() // read only: a failed Close loses nothing
	if r, ok := f.(io.ReaderAt); ok {
		b := make([]byte, p.length)
		n, rerr := r.ReadAt(b, int64(p.offset))
		if n < len(b) {
			if rerr == nil || errors.Is(rerr, io.EOF) {
				rerr = fmt.Errorf("%w: %s:%d-%d is past the end of the file", ErrChangedChunk, p.path, p.start, p.end)
			}
			return "", rerr
		}
		return string(b), nil
	}
	b, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	if p.offset+p.length > len(b) {
		return "", fmt.Errorf("%w: %s:%d-%d is past the end of the file", ErrChangedChunk, p.path, p.start, p.end)
	}
	return string(b[p.offset : p.offset+p.length]), nil // a copy of only these bytes
}

// entry is what the cleanup of a text needs to find it in the store.
type entry struct {
	id shared.ChunkID
	w  weak.Pointer[Text]
}

// forget takes out the entry of a text the GC freed, unless the chunk got a newer one.
func (s *Store) forget(e entry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.texts[e.id] == e.w {
		delete(s.texts, e.id)
	}
}

// Reads returns how many times the store read a text from the corpus.
func (s *Store) Reads() int64 { return s.reads.Load() }

// Cached returns how many texts the store has entries for, alive or freed by a GC whose
// cleanups have not run yet.
func (s *Store) Cached() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.texts)
}
