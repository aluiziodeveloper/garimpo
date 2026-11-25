package main

import (
	"errors"
	"testing"
)

func TestLoadReadsTheTitle(t *testing.T) {
	src := mapSource{"a.txt": "title: Goroutines\nCheap threads.\n"}

	doc, err := load(src, "a.txt")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Title != "Goroutines" {
		t.Errorf("title = %q, want %q", doc.Title, "Goroutines")
	}
}

func TestLoadMissingDocumentIsErrNotFound(t *testing.T) {
	sources := map[string]Source{
		"map": mapSource{},
		"dir": dirSource(t.TempDir()),
	}
	for name, src := range sources {
		_, err := load(src, "missing.txt")
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("%s source: err = %v, want one that wraps ErrNotFound", name, err)
		}
	}
}

func TestLoadReportsTheBadLine(t *testing.T) {
	src := mapSource{"bad.txt": "Goroutines\nNo title line.\n"}

	_, err := load(src, "bad.txt")

	perr, ok := errors.AsType[*ParseError](err)
	if !ok {
		t.Fatalf("err = %v, want a *ParseError in the chain", err)
	}
	if perr.Line != 1 {
		t.Errorf("line = %d, want 1", perr.Line)
	}
}
