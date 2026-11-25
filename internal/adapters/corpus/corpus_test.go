package corpus

import (
	"errors"
	"io/fs"
	"slices"
	"testing"
	"testing/fstest"

	"github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
)

// library has one file of each kind the walk must keep or skip.
var library = fstest.MapFS{
	"bytes/bytes.go":         {Data: []byte("package bytes\n")},
	"bytes/bytes_test.go":    {Data: []byte("package bytes\n")},
	"bytes/testdata/x.go":    {Data: []byte("package x\n")},
	"cmd/go/main.go":         {Data: []byte("package main\n")},
	"vendor/golang.org/x.go": {Data: []byte("package x\n")},
	"strings/README":         {Data: []byte("not Go\n")},
	"strings/strings.go":     {Data: []byte("package strings\n")},
	"internal/cmd/tool/x.go": {Data: []byte("package tool\n")},
}

func paths(t *testing.T, fsys fs.FS) []string {
	t.Helper()
	var got []string
	for d, err := range Walk(fsys) {
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, d.Path())
	}
	return got
}

func TestWalkYieldsTheLibrarySources(t *testing.T) {
	got := paths(t, library)
	want := []string{"bytes/bytes.go", "internal/cmd/tool/x.go", "strings/strings.go"}
	if !slices.Equal(got, want) {
		t.Errorf("Walk() paths = %v, want %v", got, want)
	}
}

func TestWalkStopsWhenTheLoopBreaks(t *testing.T) {
	opened := &countingFS{FS: library}
	for _, err := range Walk(opened) {
		if err != nil {
			t.Fatal(err)
		}
		break
	}
	if opened.files != 1 {
		t.Errorf("the loop broke after one document and the walk opened %d files", opened.files)
	}
}

func TestWalkYieldsTheErrorAndStops(t *testing.T) {
	broken := fstest.MapFS{
		"a/a.go": {Data: []byte("package a\n")},
		"b/b.go": {Data: []byte("package b\n\xff\n")},
		"c/c.go": {Data: []byte("package c\n")},
	}
	var got []string
	var errs []error
	for d, err := range Walk(broken) {
		if err != nil {
			errs = append(errs, err)
			continue
		}
		got = append(got, d.Path())
	}
	if !slices.Equal(got, []string{"a/a.go"}) || len(errs) != 1 {
		t.Fatalf("Walk() = %v and %d errors, want [a/a.go] and 1 error", got, len(errs))
	}
	if !errors.Is(errs[0], ingestion.ErrInvalidDocument) {
		t.Errorf("error = %v, want it to wrap ErrInvalidDocument", errs[0])
	}
}

// countingFS counts the files the walk opens.
type countingFS struct {
	fs.FS
	files int
}

func (c *countingFS) Open(name string) (fs.File, error) {
	f, err := c.FS.Open(name)
	if err == nil {
		if st, statErr := f.Stat(); statErr == nil && !st.IsDir() {
			c.files++
		}
	}
	return f, err
}
