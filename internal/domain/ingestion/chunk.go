package ingestion

import (
	"errors"
	"fmt"
	"iter"
	"strings"
	"unicode/utf8"

	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// DefaultWindow is the most bytes a chunk holds (D15). With 700, a chunk of the corpus
// averages about 217 tokens and fits the 256-token context of all-minilm.
const DefaultWindow = 700

// ErrInvalidWindow is wrapped by every error NewCutter returns.
var ErrInvalidWindow = errors.New("invalid window")

// Cutter cuts documents into chunks of whole lines. The zero Cutter uses DefaultWindow.
type Cutter struct {
	window int
}

// NewCutter returns a Cutter whose chunks hold at most window bytes. The window must be
// at least utf8.UTFMax bytes, so that any character fits in a chunk.
func NewCutter(window int) (Cutter, error) {
	if window < utf8.UTFMax {
		return Cutter{}, fmt.Errorf("%w: %d bytes, want at least %d", ErrInvalidWindow, window, utf8.UTFMax)
	}
	return Cutter{window: window}, nil
}

// Chunk is a run of lines of a document: lines Start to End, 1-based and inclusive, and
// their text, newlines included. Only a line longer than the window is split, into
// chunks with Start == End. Cut is the only way to get one.
type Chunk struct {
	id         shared.ChunkID
	path       string
	start, end int
	text       string
}

// ID returns the ID of the chunk: the hash of its path, lines and text.
func (c Chunk) ID() shared.ChunkID { return c.id }

// Path returns the path of the document the chunk was cut from.
func (c Chunk) Path() string { return c.path }

// Start returns the first line of the chunk, counting from 1.
func (c Chunk) Start() int { return c.start }

// End returns the last line of the chunk.
func (c Chunk) End() int { return c.end }

// Text returns the lines of the chunk, newlines included.
func (c Chunk) Text() string { return c.text }

// Cut returns an iterator over the chunks of d, in order. Each chunk takes as many whole
// lines as fit in the window; a line longer than the window is split into pieces of the
// window's size, and the last piece starts the next chunk. The texts of the chunks,
// joined, are the text of d. A chunk's text is a substring of the document's: cutting
// copies nothing.
func (c Cutter) Cut(d Document) iter.Seq[Chunk] {
	window := c.window
	if window == 0 {
		window = DefaultWindow
	}
	return func(yield func(Chunk) bool) {
		text := d.Text()
		from, to := 0, 0 // the bytes of the chunk being filled
		start, line := 1, 0
		for l := range strings.Lines(text) {
			line++
			if to > from && to-from+len(l) > window {
				if !yield(newChunk(d.Path(), start, line-1, text[from:to])) {
					return
				}
				from, start = to, line
			}
			to += len(l)
			for to-from > window { // only a line longer than the window gets here
				cut := from + window
				for !utf8.RuneStart(text[cut]) { // never split a character
					cut--
				}
				if !yield(newChunk(d.Path(), line, line, text[from:cut])) {
					return
				}
				from, start = cut, line
			}
		}
		if to > from {
			yield(newChunk(d.Path(), start, line, text[from:to]))
		}
	}
}

// newChunk returns the chunk with lines start to end of path. Cut always passes a valid
// path (a Document's) and 1 <= start <= end, so NewChunkID cannot fail here.
func newChunk(path string, start, end int, text string) Chunk {
	id, err := shared.NewChunkID(path, start, end, text)
	if err != nil {
		panic(fmt.Sprintf("ingestion: chunk %s:%d-%d: %v", path, start, end, err))
	}
	return Chunk{id: id, path: path, start: start, end: end, text: text}
}
