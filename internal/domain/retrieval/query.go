package retrieval

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxQueryBytes is the longest query, in bytes: a question, not a document. The model reads
// 512 tokens at most, and a longer query would fail at the embedding, after the keyword
// search had run for nothing.
const MaxQueryBytes = 1000

var (
	// ErrEmptyQuery is wrapped when a query has no text but spaces.
	ErrEmptyQuery = errors.New("empty query")
	// ErrInvalidQuery is wrapped when a query is not valid UTF-8.
	ErrInvalidQuery = errors.New("query is not valid UTF-8")
	// ErrQueryTooLong is wrapped when a query has more than MaxQueryBytes bytes.
	ErrQueryTooLong = errors.New("query too long")
)

// Query is the text of a question to the corpus: valid UTF-8, without the space around it,
// not empty and at most MaxQueryBytes long. NewQuery is the only way to get a non-zero
// Query; the zero value is no question.
type Query struct{ text string }

// NewQuery returns the query of text, without the space around it. It fails with an error
// that wraps ErrEmptyQuery, ErrInvalidQuery or ErrQueryTooLong.
func NewQuery(text string) (Query, error) {
	if !utf8.ValidString(text) {
		return Query{}, ErrInvalidQuery
	}
	text = strings.TrimSpace(text)
	switch {
	case text == "":
		return Query{}, ErrEmptyQuery
	case len(text) > MaxQueryBytes:
		return Query{}, fmt.Errorf("%w: %d bytes, at most %d", ErrQueryTooLong, len(text), MaxQueryBytes)
	}
	return Query{text: text}, nil
}

// String returns the text of the query.
func (q Query) String() string { return q.text }

// IsZero reports whether q is the zero Query.
func (q Query) IsZero() bool { return q.text == "" }
