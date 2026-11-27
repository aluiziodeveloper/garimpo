package chunktext_test

import (
	"errors"
	"runtime"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	"github.com/aluiziodeveloper/garimpo/internal/adapters/chunktext"
	"github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// small is a corpus of two files, and window a window that cuts each in several
// chunks.
var small = fstest.MapFS{
	"a/a.go": {Data: []byte("package a\n\nfunc A() int {\n\treturn 1\n}\n\nfunc B() int {\n\treturn 2\n}\n")},
	"b/b.go": {Data: []byte("package b\n\n// C is the third.\nconst C = 3\n")},
}

const window = 20

// filled returns a store of fsys with every chunk of its files, and the chunks.
func filled(t *testing.T, fsys fstest.MapFS) (*chunktext.Store, []ingestion.Chunk) {
	t.Helper()
	cutter, err := ingestion.NewCutter(window)
	if err != nil {
		t.Fatal(err)
	}
	s := chunktext.New(fsys)
	var chunks []ingestion.Chunk
	for _, p := range []string{"a/a.go", "b/b.go"} {
		d, err := ingestion.NewDocument(p, string(fsys[p].Data))
		if err != nil {
			t.Fatal(err)
		}
		for c := range cutter.Cut(d) {
			if err := s.Add(c); err != nil {
				t.Fatal(err)
			}
			chunks = append(chunks, c)
		}
	}
	return s, chunks
}

func TestTextIsTheTextOfTheChunk(t *testing.T) {
	s, chunks := filled(t, small)
	if len(chunks) < 4 {
		t.Fatalf("%d chunks, want at least 4 to test anything", len(chunks))
	}
	for _, c := range chunks {
		got, err := s.Text(c.ID())
		if err != nil {
			t.Fatal(err)
		}
		if got.String() != c.Text() {
			t.Errorf("Text of %s:%d-%d = %q, want %q", c.Path(), c.Start(), c.End(), got.String(), c.Text())
		}
	}
}

// The store finds where a chunk is, for a citation (RN01).
func TestFindChunkSaysWhereTheChunkIs(t *testing.T) {
	s, chunks := filled(t, small)
	var _ shared.ChunkFinder = s
	for _, c := range chunks {
		path, start, end, ok := s.FindChunk(c.ID())
		if !ok || path != c.Path() || start != c.Start() || end != c.End() {
			t.Errorf("FindChunk = %q %d-%d %v, want %q %d-%d true", path, start, end, ok, c.Path(), c.Start(), c.End())
		}
	}
	if _, _, _, ok := s.FindChunk(shared.ChunkID{}); ok {
		t.Error("FindChunk of the zero ID found a chunk")
	}
}

func TestTextOfAChunkNotAddedFails(t *testing.T) {
	s, _ := filled(t, small)
	id, err := shared.NewChunkID("c.go", 0, 1, 1, "package c\n")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.Text(id); !errors.Is(err, chunktext.ErrUnknownChunk) {
		t.Errorf("Text of a chunk not added: err = %v, want one that wraps %v", err, chunktext.ErrUnknownChunk)
	}
}

func TestAddRefusesAChunkAlreadyAdded(t *testing.T) {
	s, chunks := filled(t, small)
	if err := s.Add(chunks[0]); !errors.Is(err, chunktext.ErrDuplicateChunk) {
		t.Errorf("Add of a chunk already added: err = %v, want one that wraps %v", err, chunktext.ErrDuplicateChunk)
	}
}

// A file that changed after the chunks were added does not give a text that is not the
// chunk's: the text read must hash to the chunk's ID.
func TestTextOfAChangedFileFails(t *testing.T) {
	fsys := fstest.MapFS{}
	for p, f := range small {
		fsys[p] = &fstest.MapFile{Data: append([]byte(nil), f.Data...)}
	}
	s, chunks := filled(t, fsys)
	fsys["a/a.go"].Data[0] = 'P'
	if _, err := s.Text(chunks[0].ID()); !errors.Is(err, chunktext.ErrChangedChunk) {
		t.Errorf("Text after the file changed: err = %v, want one that wraps %v", err, chunktext.ErrChangedChunk)
	}
}

// While a caller holds a text, every other caller gets the same one, and the store reads
// the corpus once.
func TestAHeldTextIsReadOnce(t *testing.T) {
	s, chunks := filled(t, small)
	first, err := s.Text(chunks[1].ID())
	if err != nil {
		t.Fatal(err)
	}
	runtime.GC()
	second, err := s.Text(chunks[1].ID())
	if err != nil {
		t.Fatal(err)
	}
	if first != second || s.Reads() != 1 {
		t.Errorf("two Texts of a chunk held between them: same text %v, %d reads; want true, 1", first == second, s.Reads())
	}
	runtime.KeepAlive(first)
}

// A text nobody holds goes with the GC, and its entry with it: the store keeps the places
// of the chunks, not their texts.
func TestTheStoreForgetsTextsNobodyHolds(t *testing.T) {
	s, chunks := filled(t, small)
	for _, c := range chunks {
		if _, err := s.Text(c.ID()); err != nil {
			t.Fatal(err)
		}
	}
	for start := time.Now(); s.Cached() > 0; runtime.GC() {
		if time.Since(start) > 5*time.Second {
			t.Fatalf("%d texts still cached after 5 s, with nobody holding them", s.Cached())
		}
		time.Sleep(time.Millisecond)
	}
	if _, err := s.Text(chunks[0].ID()); err != nil {
		t.Fatal(err)
	}
	if want := int64(len(chunks) + 1); s.Reads() != want {
		t.Errorf("%d reads, want %d: one per chunk, and one more after the GC took them", s.Reads(), want)
	}
}

func TestTextIsSafeForConcurrentCalls(t *testing.T) {
	s, chunks := filled(t, small)
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Go(func() {
			for i := range 200 {
				c := chunks[(g+i)%len(chunks)]
				got, err := s.Text(c.ID())
				if err != nil {
					t.Error(err)
					return
				}
				if got.String() != c.Text() {
					t.Errorf("Text = %q, want %q", got.String(), c.Text())
					return
				}
				if i%50 == 0 {
					runtime.GC()
				}
			}
		})
	}
	wg.Wait()
}
