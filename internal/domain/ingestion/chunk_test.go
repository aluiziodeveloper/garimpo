package ingestion

import (
	"errors"
	"slices"
	"strings"
	"testing"

	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// file returns the document x.go with the given text.
func file(t *testing.T, text string) Document {
	t.Helper()
	d, err := NewDocument("x.go", text)
	if err != nil {
		t.Fatal(err)
	}
	return d
}

type span struct {
	start, end int
	text       string
}

func spans(c Cutter, d Document) []span {
	var got []span
	for ch := range c.Cut(d) {
		got = append(got, span{ch.Start(), ch.End(), ch.Text()})
	}
	return got
}

func TestCutFillsTheWindowWithWholeLines(t *testing.T) {
	c, err := NewCutter(10)
	if err != nil {
		t.Fatal(err)
	}
	d := file(t, "aaaa\nbbbb\ncccc\ndd\n")
	got := spans(c, d)
	want := []span{{1, 2, "aaaa\nbbbb\n"}, {3, 4, "cccc\ndd\n"}}
	if !slices.Equal(got, want) {
		t.Errorf("Cut() = %+v, want %+v", got, want)
	}
}

func TestCutSplitsALongLine(t *testing.T) {
	c, _ := NewCutter(10)
	long := strings.Repeat("x", 25) + "\n" // 26 bytes
	got := spans(c, file(t, "a\n"+long+"b\n"))
	want := []span{
		{1, 1, "a\n"},
		{2, 2, "xxxxxxxxxx"},
		{2, 2, "xxxxxxxxxx"},
		{2, 3, "xxxxx\nb\n"},
	}
	if !slices.Equal(got, want) {
		t.Errorf("Cut() = %+v, want %+v", got, want)
	}
}

func TestCutOfAnEmptyDocumentYieldsNothing(t *testing.T) {
	var c Cutter
	if got := spans(c, file(t, "")); len(got) != 0 {
		t.Errorf("Cut() of an empty document = %+v, want nothing", got)
	}
}

func TestZeroCutterUsesTheDefaultWindow(t *testing.T) {
	var c Cutter
	line := strings.Repeat("x", 99) + "\n" // 100 bytes
	got := spans(c, file(t, strings.Repeat(line, 15)))
	if len(got) != 3 || got[0].end != 7 || got[1].end != 14 || got[2].end != 15 {
		t.Errorf("zero Cutter cut 15 lines of 100 bytes into %d chunks: %v", len(got), got)
	}
}

func TestChunkIDIsTheIDOfPathLinesAndText(t *testing.T) {
	c, _ := NewCutter(10)
	for ch := range c.Cut(file(t, "aaaa\nbbbb\ncccc\n")) {
		if ch.ID().IsZero() || ch.Path() != "x.go" {
			t.Errorf("chunk %d-%d: ID %v, path %q", ch.Start(), ch.End(), ch.ID(), ch.Path())
		}
	}
}

// TestEveryChunkHasItsOwnID cuts a line of 30 equal bytes into three equal pieces: same
// path, same line, same text. As in time/tzdata/zzipdata.go, they are three chunks.
func TestEveryChunkHasItsOwnID(t *testing.T) {
	c, _ := NewCutter(10)
	seen := make(map[shared.ChunkID]int) // ID -> position of the chunk in the cut
	n := 0
	for ch := range c.Cut(file(t, "a\n"+strings.Repeat("x", 30)+"\n")) {
		if first, dup := seen[ch.ID()]; dup {
			t.Errorf("chunk #%d (%d-%d %q) has the ID of chunk #%d", n, ch.Start(), ch.End(), ch.Text(), first)
		} else {
			seen[ch.ID()] = n
		}
		n++
	}
}

func TestNewCutterRejectsAWindowSmallerThanACharacter(t *testing.T) {
	for _, w := range []int{-1, 0, 3} {
		if _, err := NewCutter(w); !errors.Is(err, ErrInvalidWindow) {
			t.Errorf("NewCutter(%d) error = %v, want ErrInvalidWindow", w, err)
		}
	}
	if _, err := NewCutter(4); err != nil {
		t.Errorf("NewCutter(4) error = %v, want nil", err)
	}
}
