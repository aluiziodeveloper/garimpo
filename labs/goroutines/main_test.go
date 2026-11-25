package main

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

func TestCountLines(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		"a.txt": "one\ntwo\nthree\n",
		"b.txt": "",
		"c.txt": "single\n",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	paths := []string{
		filepath.Join(dir, "a.txt"),
		filepath.Join(dir, "b.txt"),
		filepath.Join(dir, "c.txt"),
	}

	got := countLines(paths)

	want := []int{3, 0, 1}
	for i, r := range got {
		if r.err != nil {
			t.Fatalf("%s: %v", paths[i], r.err)
		}
		if r.lines != want[i] {
			t.Errorf("%s: %d lines, want %d", paths[i], r.lines, want[i])
		}
	}
}

func TestCountLinesKeepsGoingAfterAnError(t *testing.T) {
	dir := t.TempDir()
	ok := filepath.Join(dir, "ok.txt")
	if err := os.WriteFile(ok, []byte("a\nb\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(dir, "missing.txt")

	got := countLines([]string{missing, ok})

	if !errors.Is(got[0].err, fs.ErrNotExist) {
		t.Errorf("missing file: err = %v, want fs.ErrNotExist", got[0].err)
	}
	if got[1].err != nil || got[1].lines != 2 {
		t.Errorf("ok file: %d lines, err %v; want 2 lines, no error", got[1].lines, got[1].err)
	}
}
