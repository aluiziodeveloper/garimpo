// Package bm25 is the keyword index of Garimpo: an inverted index of the terms of the
// chunks' texts, ranked by BM25 (module 15).
//
// An inverted index maps each term to the chunks that hold it, its postings, with how many
// times each holds it. A search reads only the postings of the query's terms, not every
// chunk. BM25 scores a chunk by each term of the query it holds: more for a term few chunks
// have (the IDF), more for a term repeated but less and less (k1), and less in a longer
// chunk (b).
package bm25

import (
	"context"
	"fmt"
	"iter"
	"math"
	"slices"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"

	"github.com/aluiziodeveloper/garimpo/internal/app/search"
	"github.com/aluiziodeveloper/garimpo/internal/domain/retrieval"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
	"github.com/aluiziodeveloper/garimpo/internal/topk"
)

// K1 and B are the parameters of BM25, at the values most systems use: K1 bounds what a
// repeated term adds, and B is how much the length of a chunk counts, from 0 to 1.
const (
	K1 = 1.2
	B  = 0.75
)

// Terms yields the terms of text, in order: its runs of letters and digits, in lower case.
// A run of one character is not a term: it says almost nothing about a chunk of Go.
func Terms(text string) iter.Seq[string] {
	return func(yield func(string) bool) {
		start := -1
		for i, r := range text {
			word := unicode.IsLetter(r) || unicode.IsDigit(r)
			switch {
			case word && start < 0:
				start = i
			case !word && start >= 0:
				if !emit(text[start:i], yield) {
					return
				}
				start = -1
			}
		}
		if start >= 0 {
			emit(text[start:], yield)
		}
	}
}

// emit yields run in lower case, unless it is one character long; it returns what yield
// did.
func emit(run string, yield func(string) bool) bool {
	if utf8.RuneCountInString(run) < 2 {
		return true
	}
	return yield(strings.ToLower(run))
}

// posting is a chunk that holds a term, and how many times.
type posting struct {
	chunk int32 // the position of the chunk in Index.ids
	count int32
}

// Index is an inverted index of the chunks' texts, safe for concurrent calls.
type Index struct {
	mu      sync.RWMutex
	ids     []shared.ChunkID // the chunks, in the order they were added
	lengths []int32          // the terms of each chunk
	total   int64            // the terms of all the chunks
	has     map[shared.ChunkID]bool
	// postings maps each term to the chunks that hold it. Its key is a copy of the term,
	// stored once, when the term is new: a substring of a chunk's text would keep the whole
	// text alive. The value is a pointer so that adding a posting never stores the key again,
	// which, for a string key, would replace the copy with the substring.
	postings map[string]*[]posting

	scratch sync.Pool // of *scratch: what a search computes the scores in, reused
	counts  sync.Pool // of map[string]int32: what an Add counts the terms of a text in
}

// scratch is what a search computes the scores in. scores has a place for every chunk, at
// 0 but for the chunks in touched, which the search gave a score; the search puts every
// one back at 0 before it returns the scratch to the pool.
type scratch struct {
	scores  []float64
	touched []int32
}

// getScratch returns a scratch with room for n chunks, every score at 0.
func (x *Index) getScratch(n int) *scratch {
	s, _ := x.scratch.Get().(*scratch)
	if s == nil {
		s = new(scratch)
	}
	if len(s.scores) < n {
		s.scores = make([]float64, n+n/4) // the index grows: room for a quarter more
	}
	return s
}

// putScratch puts the scores s touched back at 0, and s in the pool.
func (x *Index) putScratch(s *scratch) {
	for _, c := range s.touched {
		s.scores[c] = 0
	}
	s.touched = s.touched[:0]
	x.scratch.Put(s)
}

var _ search.KeywordIndex = (*Index)(nil)

// New returns an empty index.
func New() *Index {
	x := &Index{has: make(map[shared.ChunkID]bool), postings: make(map[string]*[]posting)}
	x.counts.New = func() any { return make(map[string]int32) } // a map is a pointer: no box
	return x
}

// Add keeps the terms of text as the terms of chunk id.
func (x *Index) Add(id shared.ChunkID, text string) error {
	if id.IsZero() {
		return search.ErrZeroChunk
	}
	counts := x.counts.Get().(map[string]int32)
	defer func() {
		clear(counts) // its keys are substrings of text: none may stay in the pool
		x.counts.Put(counts)
	}()
	n := int32(0)
	for term := range Terms(text) {
		counts[term]++
		n++
	}
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.has[id] {
		return fmt.Errorf("%w: %s", search.ErrDuplicateChunk, id)
	}
	c := int32(len(x.ids))
	x.ids = append(x.ids, id)
	x.lengths = append(x.lengths, n)
	x.total += int64(n)
	x.has[id] = true
	for term, count := range counts {
		ps := x.postings[term]
		if ps == nil {
			ps = new([]posting)
			x.postings[strings.Clone(term)] = ps // the one store of the key
		}
		*ps = append(*ps, posting{chunk: c, count: count})
	}
	return nil
}

// Search returns the k chunks that best match the terms of query, by BM25.
func (x *Index) Search(ctx context.Context, query string, k int) ([]retrieval.Hit, error) {
	if k < 1 {
		return nil, fmt.Errorf("%w: %d", search.ErrInvalidK, k)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	var terms []string
	for term := range Terms(query) {
		if !slices.Contains(terms, term) {
			terms = append(terms, term) // a term repeated in the query counts once
		}
	}
	x.mu.RLock()
	defer x.mu.RUnlock()
	if len(terms) == 0 || len(x.ids) == 0 {
		return []retrieval.Hit{}, nil
	}
	n := float64(len(x.ids))
	avg := float64(x.total) / n
	sc := x.getScratch(len(x.ids))
	defer x.putScratch(sc)
	scores := sc.scores
	for _, term := range terms {
		p := x.postings[term]
		if p == nil {
			continue
		}
		ps := *p
		df := float64(len(ps))
		idf := math.Log(1 + (n-df+0.5)/(df+0.5))
		for _, p := range ps {
			tf := float64(p.count)
			norm := K1 * (1 - B + B*float64(x.lengths[p.chunk])/avg)
			if scores[p.chunk] == 0 { // its first term: every term adds more than 0
				sc.touched = append(sc.touched, p.chunk)
			}
			scores[p.chunk] += idf * tf * (K1 + 1) / (tf + norm)
		}
	}
	best := topk.New(min(k, max(1, len(sc.touched))), retrieval.Compare)
	for _, c := range sc.touched {
		best.Offer(retrieval.Hit{Chunk: x.ids[c], Score: float32(scores[c])})
	}
	return best.Sorted(), nil
}

// Len returns how many chunks the index keeps.
func (x *Index) Len() int {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return len(x.ids)
}
