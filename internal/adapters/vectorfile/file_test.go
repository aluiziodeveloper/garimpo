package vectorfile_test

import (
	"errors"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/aluiziodeveloper/garimpo/internal/adapters/vectorfile"
	"github.com/aluiziodeveloper/garimpo/internal/app/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/app/ingestion/embeddertest"
	"github.com/aluiziodeveloper/garimpo/internal/app/search/indextest"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

func create(t *testing.T, dim int) (*vectorfile.Writer, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "vectors.vec")
	w, err := vectorfile.Create(path, dim)
	if err != nil {
		t.Fatal(err)
	}
	return w, path
}

func equal(a, b []shared.Vector) bool {
	return slices.EqualFunc(a, b, func(v, w shared.Vector) bool { return slices.Equal(v.Values(), w.Values()) })
}

// The file gives back the vectors in the order they were written, however many calls
// wrote them.
func TestReadGivesBackWhatWasWritten(t *testing.T) {
	w, path := create(t, 5)
	r := rand.New(rand.NewPCG(1, 1))
	var want []shared.Vector
	for _, n := range []int{3, 0, 1, 7} {
		var vs []shared.Vector
		for range n {
			vs = append(vs, indextest.Vector(t, r, 5))
		}
		if err := w.Write(vs); err != nil {
			t.Fatal(err)
		}
		want = append(want, vs...)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := vectorfile.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if !equal(got, want) {
		t.Fatalf("Read gave %d vectors, not the %d written in order", len(got), len(want))
	}
}

// Record is an Embedder: it passes the contract of the one it wraps.
func TestRecordMeetsTheEmbedderContract(t *testing.T) {
	embeddertest.Contract(t, func(t *testing.T) ingestion.Embedder {
		w, _ := create(t, embeddertest.FakeDim)
		t.Cleanup(func() { _ = w.Close() })
		return vectorfile.Record(&embeddertest.Fake{Context: 100}, w)
	}, strings.Repeat("x", 101))
}

// Record writes the vectors the model returned, and nothing for a call that failed.
func TestRecordWritesWhatTheModelReturned(t *testing.T) {
	w, path := create(t, embeddertest.FakeDim)
	fake := &embeddertest.Fake{Context: 10}
	e := vectorfile.Record(fake, w)
	want, err := e.Embed(t.Context(), []string{"a", "b"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = e.Embed(t.Context(), []string{"c", "longer than ten"}); !errors.Is(err, ingestion.ErrTextTooLong) {
		t.Fatalf("Embed of a long text: err = %v, want ErrTextTooLong", err)
	}
	if err = w.Close(); err != nil {
		t.Fatal(err)
	}
	got, err := vectorfile.Read(path)
	if err != nil {
		t.Fatal(err)
	}
	if !equal(got, want) {
		t.Fatalf("the file has %d vectors, want the %d of the call that worked", len(got), len(want))
	}
}

// Read refuses a file that is not one, or that ends in the middle of a vector: a file
// cut by a crash gives an error, never a short vector.
func TestReadRefusesABrokenFile(t *testing.T) {
	w, path := create(t, 4)
	r := rand.New(rand.NewPCG(2, 2))
	if err := w.Write([]shared.Vector{indextest.Vector(t, r, 4), indextest.Vector(t, r, 4)}); err != nil {
		t.Fatal(err)
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for name, content := range map[string][]byte{
		"not a vector file": []byte("chunk,vector\n"),
		"cut midway":        data[:len(data)-3],
	} {
		p := filepath.Join(dir, strings.ReplaceAll(name, " ", "-"))
		if err := os.WriteFile(p, content, 0o600); err != nil {
			t.Fatal(err)
		}
		if vs, err := vectorfile.Read(p); !errors.Is(err, vectorfile.ErrFormat) {
			t.Errorf("Read of a file %s: %d vectors, err = %v, want ErrFormat", name, len(vs), err)
		}
	}
}

// Write refuses a vector of another dimension than the file's.
func TestWriteRefusesAnotherDimension(t *testing.T) {
	w, _ := create(t, 4)
	defer func() { _ = w.Close() }()
	r := rand.New(rand.NewPCG(3, 3))
	if err := w.Write([]shared.Vector{indextest.Vector(t, r, 5)}); err == nil {
		t.Fatal("Write of a vector of 5 into a file of 4 did not fail")
	}
}
