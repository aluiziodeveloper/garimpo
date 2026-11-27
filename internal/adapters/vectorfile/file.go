// Package vectorfile keeps the vectors of an ingestion in a file: the real vectors of the
// corpus, which the recall of the HNSW index is measured on (module 13). Garimpo has no
// store of vectors yet; this file holds vectors only, without their chunks.
//
// The format is a header of 12 bytes, "GVEC", the version 1 and the dimension, both as
// little-endian uint32, and then the vectors one after the other, each as dim
// little-endian float32.
package vectorfile

import (
	"bufio"
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"sync"

	"github.com/aluiziodeveloper/garimpo/internal/app/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// ErrFormat is wrapped when a file is not a vector file or ends in the middle of a vector.
var ErrFormat = errors.New("not a vector file")

var magic = []byte("GVEC")

const version = 1

// Writer writes vectors to a file. Write is safe for concurrent calls.
type Writer struct {
	dim int

	mu  sync.Mutex
	f   *os.File
	buf *bufio.Writer
}

// Create creates the file at path, or truncates it, for vectors of dim dimensions.
func Create(path string, dim int) (*Writer, error) {
	if dim < 1 {
		return nil, fmt.Errorf("vectorfile: dimension %d", dim)
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, fmt.Errorf("vectorfile: %w", err)
	}
	w := &Writer{dim: dim, f: f, buf: bufio.NewWriter(f)}
	header := binary.LittleEndian.AppendUint32(binary.LittleEndian.AppendUint32(bytes.Clone(magic), version), uint32(dim))
	if _, err := w.buf.Write(header); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("vectorfile: %w", err)
	}
	return w, nil
}

// Write appends vs to the file, in order. It fails, and writes none of them, if one has
// another dimension than the file's.
func (w *Writer) Write(vs []shared.Vector) error {
	for i, v := range vs {
		if v.Dim() != w.dim {
			return fmt.Errorf("vectorfile: vector %d has %d dimensions, the file %d", i, v.Dim(), w.dim)
		}
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	var b [4]byte
	for _, v := range vs {
		for _, x := range v.Values() {
			binary.LittleEndian.PutUint32(b[:], math.Float32bits(x))
			if _, err := w.buf.Write(b[:]); err != nil {
				return fmt.Errorf("vectorfile: %w", err)
			}
		}
	}
	return nil
}

// Close writes what is buffered and closes the file.
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return errors.Join(w.buf.Flush(), w.f.Close())
}

// Read returns the vectors of the file at path, in the order they were written.
func Read(path string) ([]shared.Vector, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("vectorfile: %w", err)
	}
	if len(data) < 12 || !bytes.Equal(data[:4], magic) || binary.LittleEndian.Uint32(data[4:]) != version {
		return nil, fmt.Errorf("%w: %s", ErrFormat, path)
	}
	dim := int(binary.LittleEndian.Uint32(data[8:]))
	body := data[12:]
	if dim < 1 || len(body)%(4*dim) != 0 {
		return nil, fmt.Errorf("%w: %s ends in the middle of a vector of %d", ErrFormat, path, dim)
	}
	vs := make([]shared.Vector, 0, len(body)/(4*dim))
	values := make([]float32, dim)
	for len(body) > 0 {
		for i := range values {
			values[i] = math.Float32frombits(binary.LittleEndian.Uint32(body[4*i:]))
		}
		v, err := shared.NewVector(values, dim)
		if err != nil {
			return nil, fmt.Errorf("vectorfile: vector %d of %s: %w", len(vs), path, err)
		}
		vs = append(vs, v)
		body = body[4*dim:]
	}
	return vs, nil
}

// Record returns an Embedder that calls e and writes to w every vector e returns. A call
// that fails writes nothing; a vector it cannot write fails the call.
func Record(e ingestion.Embedder, w *Writer) ingestion.Embedder {
	return recorder{e, w}
}

type recorder struct {
	e ingestion.Embedder
	w *Writer
}

func (r recorder) Embed(ctx context.Context, texts []string) ([]shared.Vector, error) {
	vs, err := r.e.Embed(ctx, texts)
	if err != nil {
		return nil, err
	}
	if err := r.w.Write(vs); err != nil {
		return nil, err
	}
	return vs, nil
}
