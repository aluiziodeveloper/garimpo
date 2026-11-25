package main

import (
	"errors"
	"slices"
	"testing"
	"testing/fstest"
)

// corpus has one file of each kind the walk must keep or skip.
var corpus = fstest.MapFS{
	"bytes/bytes.go":         {Data: []byte("package bytes\n")},
	"bytes/bytes_test.go":    {Data: []byte("package bytes\n")},
	"bytes/testdata/x.go":    {Data: []byte("package x\n")},
	"cmd/go/main.go":         {Data: []byte("package main\n")},
	"vendor/golang.org/x.go": {Data: []byte("package x\n")},
	"strings/README":         {Data: []byte("not Go\n")},
	"strings/strings.go":     {Data: []byte("package strings\n")},
	"internal/cmd/tool/x.go": {Data: []byte("package tool\n")},
}

var want = []string{"bytes/bytes.go", "internal/cmd/tool/x.go", "strings/strings.go"}

func TestLoadAllKeepsOnlyLibrarySources(t *testing.T) {
	docs, err := loadAll(corpus)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, d := range docs {
		got = append(got, d.path)
	}
	if !slices.Equal(got, want) {
		t.Errorf("loadAll() paths = %v, want %v", got, want)
	}
}

func TestEachVisitsTheSameFiles(t *testing.T) {
	var got []string
	err := each(corpus, func(d doc) error {
		got = append(got, d.path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(got, want) {
		t.Errorf("each() paths = %v, want %v", got, want)
	}
}

func TestEachStopsWhenTheCallerIsDone(t *testing.T) {
	stop := errors.New("enough")
	visited := 0
	err := each(corpus, func(doc) error {
		visited++
		return stop
	})
	if !errors.Is(err, stop) || visited != 1 {
		t.Errorf("each() = %v after %d files, want the caller's error after 1", err, visited)
	}
}
