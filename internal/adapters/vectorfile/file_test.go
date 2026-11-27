package vectorfile_test

import (
	"encoding/binary"
	"errors"
	"math"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/aluiziodeveloper/garimpo/internal/adapters/vectorfile"
	"github.com/aluiziodeveloper/garimpo/internal/app/search/indextest"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

func open(t *testing.T, path string, dim int) *vectorfile.Writer {
	t.Helper()
	w, err := vectorfile.Open(path, dim)
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func add(t *testing.T, w *vectorfile.Writer, entries ...vectorfile.Entry) {
	t.Helper()
	for _, e := range entries {
		if err := w.Add(e.ID, e.Vector); err != nil {
			t.Fatal(err)
		}
	}
}

func closeFile(t *testing.T, w *vectorfile.Writer) {
	t.Helper()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
}

// entries returns n chunks numbered from first, with random vectors of dim.
func entries(t *testing.T, r *rand.Rand, first, n, dim int) []vectorfile.Entry {
	out := make([]vectorfile.Entry, n)
	for i := range out {
		out[i] = vectorfile.Entry{ID: indextest.Chunk(t, first+i), Vector: indextest.Vector(t, r, dim)}
	}
	return out
}

func same(a, b []vectorfile.Entry) bool {
	return slices.EqualFunc(a, b, func(x, y vectorfile.Entry) bool {
		return x.ID == y.ID && slices.Equal(x.Vector.Values(), y.Vector.Values())
	})
}

// The file gives back each chunk with its vector, in the order they were added; Read gives
// the vectors alone.
func TestReadChunksGivesBackWhatWasAdded(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vectors.vec")
	want := entries(t, rand.New(rand.NewPCG(1, 1)), 0, 11, 5)
	w := open(t, path, 5)
	add(t, w, want...)
	closeFile(t, w)
	got, err := vectorfile.ReadChunks(path)
	if err != nil {
		t.Fatal(err)
	}
	if !same(got, want) {
		t.Fatalf("ReadChunks gave %d chunks, not the %d added in order", len(got), len(want))
	}
	vs, err := vectorfile.Read(path)
	if err != nil || len(vs) != len(want) || !slices.Equal(vs[3].Values(), want[3].Vector.Values()) {
		t.Fatalf("Read gave %d vectors, err %v; want the %d vectors", len(vs), err, len(want))
	}
}

// Many goroutines add at once, as the workers of an ingestion do: every record is whole.
func TestAddIsSafeForConcurrentCalls(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vectors.vec")
	w := open(t, path, 7)
	var wg sync.WaitGroup
	for g := range 4 {
		chunk := entries(t, rand.New(rand.NewPCG(uint64(g), 2)), g*50, 50, 7)
		wg.Go(func() {
			for _, e := range chunk {
				if err := w.Add(e.ID, e.Vector); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	wg.Wait()
	closeFile(t, w)
	got, err := vectorfile.ReadChunks(path)
	if err != nil || len(got) != 200 {
		t.Fatalf("ReadChunks: %d chunks, err %v; want 200", len(got), err)
	}
}

// A resumed ingestion opens the file again and adds after what is there; a chunk embedded
// again keeps its first record.
func TestOpenAddsToAFileThatExists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vectors.vec")
	r := rand.New(rand.NewPCG(3, 3))
	first, second := entries(t, r, 0, 4, 3), entries(t, r, 4, 3, 3)
	w := open(t, path, 3)
	add(t, w, first...)
	closeFile(t, w)
	again := vectorfile.Entry{ID: first[1].ID, Vector: second[0].Vector}
	w = open(t, path, 3)
	add(t, w, append(slices.Clone(second), again)...)
	closeFile(t, w)
	got, err := vectorfile.ReadChunks(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := slices.Concat(first, second); !same(got, want) {
		t.Errorf("after a second Open: %d chunks, want the %d of both, each once with its first vector", len(got), len(want))
	}
}

// A crash in the middle of an Add leaves part of a record at the end: Open drops it, and
// the next record starts where the last whole one ended.
func TestOpenDropsARecordCutInTheMiddle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vectors.vec")
	r := rand.New(rand.NewPCG(4, 4))
	es := entries(t, r, 0, 3, 3)
	w := open(t, path, 3)
	add(t, w, es[:2]...)
	closeFile(t, w)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if terr := os.Truncate(path, info.Size()-5); terr != nil { // the second record, torn
		t.Fatal(terr)
	}
	w = open(t, path, 3)
	add(t, w, es[2])
	closeFile(t, w)
	got, err := vectorfile.ReadChunks(path)
	if err != nil {
		t.Fatal(err)
	}
	if want := []vectorfile.Entry{es[0], es[2]}; !same(got, want) {
		t.Errorf("after a torn record: %d chunks, want the first and the one added after", len(got))
	}
}

// writeV1 writes a file of version 1, vectors without IDs, as modules 13 to 15 did.
func writeV1(t *testing.T, path string, vs []shared.Vector) {
	t.Helper()
	data := binary.LittleEndian.AppendUint32(binary.LittleEndian.AppendUint32([]byte("GVEC"), 1), uint32(vs[0].Dim()))
	for _, v := range vs {
		for _, x := range v.Values() {
			data = binary.LittleEndian.AppendUint32(data, math.Float32bits(x))
		}
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

// The vectors of the corpus measured in modules 13 to 15 are of version 1: Read still
// reads them, and ReadChunks and Open refuse them, since they have no IDs.
func TestAFileOfVersionOneHasVectorsOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.vec")
	r := rand.New(rand.NewPCG(5, 5))
	vs := []shared.Vector{indextest.Vector(t, r, 4), indextest.Vector(t, r, 4)}
	writeV1(t, path, vs)
	got, err := vectorfile.Read(path)
	if err != nil || len(got) != 2 || !slices.Equal(got[1].Values(), vs[1].Values()) {
		t.Fatalf("Read of version 1: %d vectors, err %v", len(got), err)
	}
	if _, err := vectorfile.ReadChunks(path); !errors.Is(err, vectorfile.ErrFormat) {
		t.Errorf("ReadChunks of version 1: err = %v, want %v", err, vectorfile.ErrFormat)
	}
	if _, err := vectorfile.Open(path, 4); !errors.Is(err, vectorfile.ErrFormat) {
		t.Errorf("Open of version 1: err = %v, want %v", err, vectorfile.ErrFormat)
	}
}

func TestOpenAndAddRefuseWhatTheFileCannotHold(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "vectors.vec")
	r := rand.New(rand.NewPCG(6, 6))
	w := open(t, path, 3)
	if err := w.Add(indextest.Chunk(t, 0), indextest.Vector(t, r, 4)); err == nil {
		t.Error("Add of a vector of 4 in a file of 3: err = nil")
	}
	if err := w.Add(shared.ChunkID{}, indextest.Vector(t, r, 3)); !errors.Is(err, shared.ErrInvalidChunkID) {
		t.Errorf("Add of the zero chunk: err = %v, want %v", err, shared.ErrInvalidChunkID)
	}
	closeFile(t, w)
	if _, err := vectorfile.Open(path, 5); !errors.Is(err, vectorfile.ErrFormat) {
		t.Errorf("Open of a file of 3 for vectors of 5: err = %v, want %v", err, vectorfile.ErrFormat)
	}
	other := filepath.Join(dir, "other.txt")
	if err := os.WriteFile(other, []byte("not vectors at all"), 0o644); err != nil {
		t.Fatal(err)
	}
	for name, err := range map[string]error{
		"Open":       func() error { _, err := vectorfile.Open(other, 3); return err }(),
		"Read":       func() error { _, err := vectorfile.Read(other); return err }(),
		"ReadChunks": func() error { _, err := vectorfile.ReadChunks(other); return err }(),
	} {
		if !errors.Is(err, vectorfile.ErrFormat) {
			t.Errorf("%s of a text file: err = %v, want %v", name, err, vectorfile.ErrFormat)
		}
	}
	if got, err := vectorfile.ReadChunks(path); err != nil || len(got) != 0 {
		t.Errorf("the refused Adds wrote %d records, err %v", len(got), err)
	}
}
