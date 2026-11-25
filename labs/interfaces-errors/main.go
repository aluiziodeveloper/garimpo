// Command interfaces-errors loads documents through a Source interface and tells the
// errors apart by what they are, not by their text.
//
//	docker compose exec dev go run ./labs/interfaces-errors goroutines.txt missing.txt broken.txt
package main

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// A Source gives the text of a document by name. Read returns an error that wraps
// ErrNotFound when the document does not exist.
type Source interface {
	Read(name string) (string, error)
}

// ErrNotFound is the sentinel error for a document that does not exist.
var ErrNotFound = errors.New("document not found")

// ParseError says which line of a document is wrong.
type ParseError struct {
	Line int
	Msg  string
}

func (e *ParseError) Error() string {
	return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
}

// Doc is a loaded document.
type Doc struct {
	Title string
	Body  string
}

// load reads name from src and parses it. Every error comes back wrapped with %w and the
// document name, so the caller gets context and can still inspect the cause.
func load(src Source, name string) (Doc, error) {
	text, err := src.Read(name)
	if err != nil {
		return Doc{}, fmt.Errorf("load %s: %w", name, err)
	}
	doc, err := parse(text)
	if err != nil {
		return Doc{}, fmt.Errorf("load %s: %w", name, err)
	}
	return doc, nil
}

// parse expects a first line "title: <title>".
func parse(text string) (Doc, error) {
	first, body, _ := strings.Cut(text, "\n")
	title, ok := strings.CutPrefix(first, "title: ")
	if !ok {
		return Doc{}, &ParseError{Line: 1, Msg: `want "title: <title>"`}
	}
	return Doc{Title: title, Body: body}, nil
}

// mapSource keeps documents in memory. The tests use it.
type mapSource map[string]string

func (m mapSource) Read(name string) (string, error) {
	text, ok := m[name]
	if !ok {
		return "", ErrNotFound
	}
	return text, nil
}

// dirSource reads documents from files in a directory.
type dirSource string

func (d dirSource) Read(name string) (string, error) {
	data, err := os.ReadFile(filepath.Join(string(d), name))
	if errors.Is(err, fs.ErrNotExist) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func main() {
	if err := run(os.Stdout, dirSource("labs/interfaces-errors/testdata"), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "interfaces-errors:", err)
		os.Exit(1)
	}
}

func run(w io.Writer, src Source, names []string) error {
	for _, name := range names {
		doc, err := load(src, name)
		var line string
		switch {
		case err == nil:
			line = fmt.Sprintf("%s: title %q", name, doc.Title)
		case errors.Is(err, ErrNotFound):
			line = fmt.Sprintf("%s: skipped, %v", name, err)
		default:
			if perr, ok := errors.AsType[*ParseError](err); ok {
				line = fmt.Sprintf("%s: fix line %d, %v", name, perr.Line, err)
			} else {
				return err
			}
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
	return nil
}
