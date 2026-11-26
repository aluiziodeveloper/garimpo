package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

// fakeFile counts what a measure asks of a file: the writes, their bytes and the syncs.
type fakeFile struct {
	writes, syncs int
	data          bytes.Buffer
}

func (f *fakeFile) Write(p []byte) (int, error) {
	f.writes++
	return f.data.Write(p)
}

func (f *fakeFile) Sync() error {
	f.syncs++
	return nil
}

func TestEachRecordIsOneWrite(t *testing.T) {
	var f fakeFile
	if _, err := appendRecords(&f, 100, 64, 0); err != nil {
		t.Fatal(err)
	}
	if f.writes != 100 || f.data.Len() != 100*64 {
		t.Fatalf("%d writes, %d bytes; want 100 writes of 64 bytes", f.writes, f.data.Len())
	}
	if n := bytes.Count(f.data.Bytes(), []byte("\n")); n != 100 {
		t.Fatalf("%d lines, want 100: each record ends with a newline", n)
	}
}

// A sync after every n records, and one more for the last records, so that every record
// is synced when appendRecords returns.
func TestSyncsAfterEveryNRecords(t *testing.T) {
	for _, tc := range []struct{ every, syncs int }{{1, 100}, {8, 13}, {64, 2}, {100, 1}, {512, 1}} {
		var f fakeFile
		r, err := appendRecords(&f, 100, 64, tc.every)
		if err != nil {
			t.Fatal(err)
		}
		if f.syncs != tc.syncs || r.syncs() != tc.syncs {
			t.Errorf("every %d: %d syncs (%d timed), want %d", tc.every, f.syncs, r.syncs(), tc.syncs)
		}
	}
}

func TestZeroNeverSyncs(t *testing.T) {
	var f fakeFile
	if _, err := appendRecords(&f, 100, 64, 0); err != nil {
		t.Fatal(err)
	}
	if f.syncs != 0 {
		t.Fatalf("%d syncs, want 0", f.syncs)
	}
}

func TestTheFileHoldsEveryRecord(t *testing.T) {
	path := filepath.Join(t.TempDir(), "records")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = appendRecords(f, 50, 32, 8); err != nil {
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(data) != 50*32 || bytes.Count(data, []byte("\n")) != 50 {
		t.Fatalf("%d bytes, %d lines; want 50 lines of 32 bytes", len(data), bytes.Count(data, []byte("\n")))
	}
}
