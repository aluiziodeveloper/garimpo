package ingestion

import (
	"errors"
	"fmt"
	"io/fs"
	"unicode/utf8"
)

// ErrInvalidDocument is wrapped by every error NewDocument returns.
var ErrInvalidDocument = errors.New("invalid document")

// Document is one file of the corpus: its path, relative to the corpus root, and its
// text. NewDocument is the only way to get a non-zero Document.
type Document struct {
	path string
	text string
}

// NewDocument returns the document at path with the given text. The path must be a
// valid, slash-separated relative path (io/fs.ValidPath), as a citation will print it,
// and the text must be valid UTF-8.
func NewDocument(path, text string) (Document, error) {
	switch {
	case path == "" || path == "." || !fs.ValidPath(path):
		return Document{}, fmt.Errorf("%w: path %q is not a relative slash-separated path", ErrInvalidDocument, path)
	case !utf8.ValidString(text):
		return Document{}, fmt.Errorf("%w: %s is not valid UTF-8", ErrInvalidDocument, path)
	}
	return Document{path: path, text: text}, nil
}

// Path returns the path of the document, relative to the corpus root.
func (d Document) Path() string { return d.path }

// Text returns the text of the document.
func (d Document) Text() string { return d.text }
