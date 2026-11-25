package ingestion

import (
	"strings"
	"testing"
	"unicode/utf8"
)

// FuzzCut checks the properties every cut must have, for any valid document and window:
// the chunks rebuild the text, none is larger than the window or empty, each one is
// valid UTF-8, and their lines run from 1 to the last line without a gap.
func FuzzCut(f *testing.F) {
	f.Add("package x\n\nfunc F() {}\n", 10)
	f.Add("a\n"+strings.Repeat("x", 25)+"\nb\n", 10)
	f.Add("sem quebra no fim", 4)
	f.Add(strings.Repeat("ː", 5), 5) // a 5-byte cut falls inside the third character
	f.Fuzz(func(t *testing.T, text string, window int) {
		d, err := NewDocument("x.go", text)
		if err != nil || window > 4096 {
			t.Skip("not a valid document, or a window larger than the test needs")
		}
		c, err := NewCutter(window)
		if err != nil {
			t.Skip("NewCutter rejects the window; its own test covers it")
		}
		var joined strings.Builder
		prevEnd := 0
		for ch := range c.Cut(d) {
			switch {
			case ch.Text() == "":
				t.Fatalf("empty chunk at lines %d-%d", ch.Start(), ch.End())
			case len(ch.Text()) > window:
				t.Fatalf("chunk %d-%d has %d bytes, window %d", ch.Start(), ch.End(), len(ch.Text()), window)
			case !utf8.ValidString(ch.Text()):
				t.Fatalf("chunk %d-%d is not valid UTF-8: %q", ch.Start(), ch.End(), ch.Text())
			case ch.Start() != prevEnd && ch.Start() != prevEnd+1, ch.End() < ch.Start():
				t.Fatalf("chunk %d-%d after a chunk that ended at line %d", ch.Start(), ch.End(), prevEnd)
			}
			joined.WriteString(ch.Text())
			prevEnd = ch.End()
		}
		if joined.String() != text {
			t.Fatalf("the chunks rebuild %q, want %q", joined.String(), text)
		}
		if lines := strings.Count(text, "\n") + 1 - boolToInt(strings.HasSuffix(text, "\n") || text == ""); prevEnd != lines {
			t.Fatalf("the last chunk ends at line %d, the document has %d lines", prevEnd, lines)
		}
	})
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
