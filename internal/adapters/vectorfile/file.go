// Package vectorfile keeps the vectors of an ingestion in a file, each with the ID of its
// chunk: what garimpo serve loads into the vector index (module 16), and the real vectors
// of the corpus the indexes are measured on (modules 13 to 15).
//
// The format is a header of 12 bytes, "GVEC", the version and the dimension, both as
// little-endian uint32, and then the records one after the other. Version 2, the one this
// package writes, has a record per chunk: the 32 bytes of its ChunkID and then its vector,
// dim little-endian float32. Version 1, the one of modules 13 to 15, had the vectors only;
// Read still reads it.
package vectorfile

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"sync"

	"github.com/aluiziodeveloper/garimpo/internal/app/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// ErrFormat is wrapped when a file is not a vector file, has another dimension than the
// one asked for, or ends in the middle of a record.
var ErrFormat = errors.New("not a vector file")

var magic = []byte("GVEC")

const (
	versionVectors = 1 // vectors only
	versionChunks  = 2 // a ChunkID and its vector per record
	headerSize     = 12
)

// Entry is a chunk of the file: its ID and its vector.
type Entry struct {
	ID     shared.ChunkID
	Vector shared.Vector
}

// Writer adds records to a file of version 2. It is an ingestion.VectorStore: Add is safe
// for concurrent calls, and when it returns nil the record is on the disk.
type Writer struct {
	dim int
	mu  sync.Mutex
	f   *os.File
	rec []byte // the record being written, reused
}

var _ ingestion.VectorStore = (*Writer)(nil)

// Open opens the file at path for vectors of dim dimensions, to add to it: it creates the
// file if there is none, and else checks its header and drops a record a crash cut in the
// middle, so a resumed ingestion adds after the last whole one. A chunk embedded again on
// resume gets a second record; ReadChunks keeps the first.
func Open(path string, dim int) (*Writer, error) {
	if dim < 1 {
		return nil, fmt.Errorf("vectorfile: dimension %d", dim)
	}
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, 0o644)
	if err != nil {
		return nil, fmt.Errorf("vectorfile: %w", err)
	}
	w := &Writer{dim: dim, f: f}
	if err := w.prepare(path); err != nil {
		_ = f.Close()
		return nil, err
	}
	return w, nil
}

// prepare writes the header of a new file, or checks the one of an old file and puts the
// end of the file after its last whole record.
func (w *Writer) prepare(path string) error {
	info, err := w.f.Stat()
	if err != nil {
		return fmt.Errorf("vectorfile: %w", err)
	}
	if info.Size() == 0 {
		header := binary.LittleEndian.AppendUint32(binary.LittleEndian.AppendUint32(bytes.Clone(magic), versionChunks), uint32(w.dim))
		if _, werr := w.f.Write(header); werr != nil {
			return fmt.Errorf("vectorfile: %w", werr)
		}
		return w.sync()
	}
	header := make([]byte, headerSize)
	if _, rerr := io.ReadFull(w.f, header); rerr != nil {
		return fmt.Errorf("%w: %s: %w", ErrFormat, path, rerr)
	}
	version, dim, err := parseHeader(header)
	switch {
	case err != nil:
		return fmt.Errorf("%w: %s", err, path)
	case version != versionChunks:
		return fmt.Errorf("%w: %s is of version %d, without chunk IDs", ErrFormat, path, version)
	case dim != w.dim:
		return fmt.Errorf("%w: %s has vectors of %d, not %d", ErrFormat, path, dim, w.dim)
	}
	whole := headerSize + (info.Size()-headerSize)/int64(w.recordSize())*int64(w.recordSize())
	if whole != info.Size() {
		if err := w.f.Truncate(whole); err != nil {
			return fmt.Errorf("vectorfile: dropping the torn record of %s: %w", path, err)
		}
	}
	if _, err := w.f.Seek(whole, io.SeekStart); err != nil {
		return fmt.Errorf("vectorfile: %w", err)
	}
	return nil
}

func (w *Writer) recordSize() int { return shared.ChunkIDSize + 4*w.dim }

// Add appends the record of chunk id and its vector v, and syncs it to the disk: a chunk the
// journal confirms after Add returned has its vector in the file, even after a crash. It
// fails, and writes nothing, for a vector of another dimension or the zero ChunkID.
func (w *Writer) Add(id shared.ChunkID, v shared.Vector) error {
	if v.Dim() != w.dim {
		return fmt.Errorf("vectorfile: a vector of %d in a file of %d", v.Dim(), w.dim)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	rec, err := id.AppendBinary(w.rec[:0])
	if err != nil {
		return fmt.Errorf("vectorfile: %w", err)
	}
	for _, x := range v.Values() {
		rec = binary.LittleEndian.AppendUint32(rec, math.Float32bits(x))
	}
	w.rec = rec
	if _, err := w.f.Write(rec); err != nil {
		return fmt.Errorf("vectorfile: %w", err)
	}
	return w.sync()
}

func (w *Writer) sync() error {
	if err := w.f.Sync(); err != nil {
		return fmt.Errorf("vectorfile: %w", err)
	}
	return nil
}

// Close closes the file. Every record Add returned for is on the disk already.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.f.Close()
}

// parseHeader returns the version and the dimension of a header.
func parseHeader(h []byte) (version, dim int, err error) {
	if len(h) < headerSize || !bytes.Equal(h[:4], magic) {
		return 0, 0, ErrFormat
	}
	version, dim = int(binary.LittleEndian.Uint32(h[4:])), int(binary.LittleEndian.Uint32(h[8:]))
	if (version != versionVectors && version != versionChunks) || dim < 1 {
		return 0, 0, fmt.Errorf("%w: version %d, dimension %d", ErrFormat, version, dim)
	}
	return version, dim, nil
}

// read returns the records of the file at path: with the IDs for version 2, with zero IDs
// for version 1.
func read(path string) ([]Entry, int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, fmt.Errorf("vectorfile: %w", err)
	}
	version, dim, err := parseHeader(data)
	if err != nil {
		return nil, 0, fmt.Errorf("%w: %s", err, path)
	}
	idSize := 0
	if version == versionChunks {
		idSize = shared.ChunkIDSize
	}
	size := idSize + 4*dim
	body := data[headerSize:]
	if len(body)%size != 0 {
		return nil, 0, fmt.Errorf("%w: %s ends in the middle of a record of %d bytes", ErrFormat, path, size)
	}
	entries := make([]Entry, 0, len(body)/size)
	values := make([]float32, dim)
	for ; len(body) > 0; body = body[size:] {
		var e Entry
		if idSize > 0 {
			if uerr := e.ID.UnmarshalBinary(body[:idSize]); uerr != nil {
				return nil, 0, fmt.Errorf("vectorfile: record %d of %s: %w", len(entries), path, uerr)
			}
		}
		for i := range values {
			values[i] = math.Float32frombits(binary.LittleEndian.Uint32(body[idSize+4*i:]))
		}
		if e.Vector, err = shared.NewVector(values, dim); err != nil {
			return nil, 0, fmt.Errorf("vectorfile: record %d of %s: %w", len(entries), path, err)
		}
		entries = append(entries, e)
	}
	return entries, version, nil
}

// Read returns the vectors of the file at path, of either version, in the order they were
// written.
func Read(path string) ([]shared.Vector, error) {
	entries, _, err := read(path)
	if err != nil {
		return nil, err
	}
	vs := make([]shared.Vector, len(entries))
	for i, e := range entries {
		vs[i] = e.Vector
	}
	return vs, nil
}

// ReadChunks returns the chunks of the file at path, which must be of version 2, in the
// order they were written, each once: a chunk with two records, embedded again by a resumed
// ingestion, keeps the first.
func ReadChunks(path string) ([]Entry, error) {
	entries, version, err := read(path)
	if err != nil {
		return nil, err
	}
	if version != versionChunks {
		return nil, fmt.Errorf("%w: %s is of version %d, without chunk IDs", ErrFormat, path, version)
	}
	seen := make(map[shared.ChunkID]bool, len(entries))
	kept := entries[:0]
	for _, e := range entries {
		if !seen[e.ID] {
			seen[e.ID] = true
			kept = append(kept, e)
		}
	}
	return kept, nil
}
