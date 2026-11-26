package journal_test

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/aluiziodeveloper/garimpo/internal/adapters/journal"
	"github.com/aluiziodeveloper/garimpo/internal/app/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/app/ingestion/journaltest"
	domain "github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
)

func TestFileMeetsTheContract(t *testing.T) {
	journaltest.Contract(t, func(t *testing.T) func(*testing.T) ingestion.Journal {
		path := filepath.Join(t.TempDir(), "run.journal")
		return func(t *testing.T) ingestion.Journal { return open(t, path) }
	})
}

// open opens the journal at path and closes it when the test ends.
func open(t *testing.T, path string) *journal.File {
	t.Helper()
	f, err := journal.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = f.Close() }) // a second Close returns an error, and the test closed it already
	return f
}

// written returns a journal file holding the first 3 events, closed, and its lines.
func written(t *testing.T) (path string, lines [][]byte) {
	t.Helper()
	path = filepath.Join(t.TempDir(), "run.journal")
	f := open(t, path)
	if err := f.Append(journaltest.Events(t)[:3]); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, bytes.SplitAfter(data, []byte("\n"))[:3]
}

func replay(t *testing.T, j ingestion.Journal) []domain.Event {
	t.Helper()
	var got []domain.Event
	for e, err := range j.Replay() {
		if err != nil {
			t.Fatalf("Replay: %v", err)
		}
		got = append(got, e)
	}
	return got
}

func TestEachEventIsOneLineWithItsChecksum(t *testing.T) {
	_, lines := written(t)
	want := `{"type":"DocumentDiscovered","run":"r1","path":"encoding/json/decode.go"}`
	if got := string(lines[1]); len(got) != 8+1+len(want)+1 || got[9:len(got)-1] != want {
		t.Fatalf("line 2 is %q, want 8 hexadecimal digits, a space, %s and a newline", got, want)
	}
}

// A crash in the middle of a write leaves part of the last line. Open drops it: those
// events were never durable, since Append had not returned.
func TestOpenDropsALastLineCutByACrash(t *testing.T) {
	path, lines := written(t)
	cut := append(bytes.Join(lines, nil), lines[2][:20]...) // a fourth line, cut
	if err := os.WriteFile(path, cut, 0o600); err != nil {
		t.Fatal(err)
	}
	f := open(t, path)
	if got := replay(t, f); !reflect.DeepEqual(got, journaltest.Events(t)[:3]) {
		t.Fatalf("replayed %v, want the 3 whole events", got)
	}
	if err := f.Append(journaltest.Events(t)[3:4]); err != nil {
		t.Fatal(err)
	}
	if got := replay(t, f); !reflect.DeepEqual(got, journaltest.Events(t)[:4]) {
		t.Fatalf("after a new append, replayed %v, want 4 events", got)
	}
}

func TestOpenDropsALastLineWithABadChecksum(t *testing.T) {
	path, lines := written(t)
	lines[2][12] ^= 1 // a disk that kept the length and lost the bytes
	if err := os.WriteFile(path, bytes.Join(lines, nil), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := replay(t, open(t, path)); !reflect.DeepEqual(got, journaltest.Events(t)[:2]) {
		t.Fatalf("replayed %v, want the 2 events before the bad line", got)
	}
}

// A bad line followed by good ones is not a crash in the middle of a write: the file was
// damaged after it was written, and dropping the line would lose events that were durable.
func TestOpenRefusesADamagedLineBeforeTheLast(t *testing.T) {
	path, lines := written(t)
	lines[1][12] ^= 1
	if err := os.WriteFile(path, bytes.Join(lines, nil), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := journal.Open(path); !errors.Is(err, journal.ErrCorrupt) {
		t.Fatalf("Open of a journal damaged at line 2: err = %v, want ErrCorrupt", err)
	}
}
