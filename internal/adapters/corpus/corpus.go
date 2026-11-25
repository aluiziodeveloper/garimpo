// Package corpus reads the corpus, the Go standard library, from a file system, one
// document at a time.
package corpus

import (
	"io/fs"
	"iter"
	"path"
	"strings"

	"github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
)

// Walk returns an iterator over the library sources of fsys, in lexical order: the .go
// files that are not tests, outside testdata, vendor and the top-level cmd. It reads one
// file per step, and only when the loop asks for it.
//
// When a file cannot be read or is not a valid document, Walk yields the zero Document
// and the error, and stops.
func Walk(fsys fs.FS) iter.Seq2[ingestion.Document, error] {
	return func(yield func(ingestion.Document, error) bool) {
		err := fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if skipDir(p, d.Name()) {
					return fs.SkipDir
				}
				return nil
			}
			if !librarySource(p) {
				return nil
			}
			b, err := fs.ReadFile(fsys, p)
			if err != nil {
				return err
			}
			doc, err := ingestion.NewDocument(p, string(b))
			if err != nil {
				return err
			}
			if !yield(doc, nil) {
				return fs.SkipAll // the loop stopped: so does the walk
			}
			return nil
		})
		if err != nil {
			yield(ingestion.Document{}, err)
		}
	}
}

// skipDir reports whether the walk skips the directory: test data, vendored code and the
// commands of the toolchain are not part of the library.
func skipDir(p, name string) bool {
	return name == "testdata" || name == "vendor" || p == "cmd"
}

// librarySource reports whether the file is a Go source of the library, not a test.
func librarySource(p string) bool {
	return path.Ext(p) == ".go" && !strings.HasSuffix(p, "_test.go")
}
