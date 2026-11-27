package retrieval

import (
	"errors"
	"strings"
	"testing"
)

func TestNewQueryKeepsTheTextWithoutTheSpaceAround(t *testing.T) {
	q, err := NewQuery("  how do I sort a slice\n")
	if err != nil {
		t.Fatal(err)
	}
	if got := q.String(); got != "how do I sort a slice" {
		t.Errorf("String() = %q, want the text without the space around it", got)
	}
}

func TestNewQueryRefusesWhatIsNotAQuestion(t *testing.T) {
	tests := []struct {
		name string
		text string
		want error
	}{
		{"empty", "", ErrEmptyQuery},
		{"only spaces", " \t\n ", ErrEmptyQuery},
		{"not UTF-8", "sort \xff", ErrInvalidQuery},
		{"too long", strings.Repeat("a", MaxQueryBytes+1), ErrQueryTooLong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewQuery(tt.text); !errors.Is(err, tt.want) {
				t.Errorf("NewQuery: err = %v, want one that wraps %v", err, tt.want)
			}
		})
	}
	if _, err := NewQuery(strings.Repeat("a", MaxQueryBytes)); err != nil {
		t.Errorf("NewQuery of %d bytes: %v, want it accepted", MaxQueryBytes, err)
	}
}

// The zero Query is no question: the use case refuses it, as it refuses an empty one.
func TestTheZeroQueryIsZero(t *testing.T) {
	var zero Query
	if !zero.IsZero() || zero.String() != "" {
		t.Errorf("zero Query: IsZero %v, String %q", zero.IsZero(), zero.String())
	}
	if q, _ := NewQuery("sort"); q.IsZero() {
		t.Error("a Query of \"sort\" is zero")
	}
}
