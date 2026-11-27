package search

import (
	"context"
	"fmt"

	"github.com/aluiziodeveloper/garimpo/internal/domain/retrieval"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// MaxK is the most results a search returns: a page of answers, not the corpus.
const MaxK = 50

// Ranker ranks the chunks of the corpus for a query: Hybrid does it, by vector and by
// keyword. It returns at most k hits, best first.
type Ranker interface {
	Search(ctx context.Context, query string, k int) ([]retrieval.Hit, error)
}

var _ Ranker = (*Hybrid)(nil)

// Texts finds the chunks of the corpus: where each one is, for its citation, and its text.
// Text fails if the chunk is not in the corpus, or no longer has the text it was indexed
// with.
type Texts interface {
	shared.ChunkFinder
	Text(id shared.ChunkID) (string, error)
}

// Result is one answer of a search: the citation of a chunk (RN01), its text and its score
// in the ranking.
type Result struct {
	Citation shared.Citation
	Text     string
	Score    float32
}

// Searcher is the use case Search: it ranks the chunks for a query and returns them as
// results a reader can open, each with the citation of its lines and its text. It is safe
// for concurrent calls when its ranker and its texts are.
type Searcher struct {
	Ranker Ranker
	Texts  Texts
}

// Search returns at most k results for q, best first. It fails with an error that wraps
// retrieval.ErrEmptyQuery for the zero Query, ErrInvalidK when k is not between 1 and MaxK,
// shared.ErrInvalidCitation when the ranker returns a chunk the corpus does not have, and
// ctx's error when ctx is done.
func (s *Searcher) Search(ctx context.Context, q retrieval.Query, k int) ([]Result, error) {
	switch {
	case q.IsZero():
		return nil, retrieval.ErrEmptyQuery
	case k < 1 || k > MaxK:
		return nil, fmt.Errorf("%w: %d, at most %d", ErrInvalidK, k, MaxK)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	hits, err := s.Ranker.Search(ctx, q.String(), k)
	if err != nil {
		return nil, err
	}
	results := make([]Result, 0, len(hits))
	for _, h := range hits {
		path, start, end, ok := s.Texts.FindChunk(h.Chunk)
		if !ok {
			return nil, fmt.Errorf("%w: the ranker returned chunk %s, which the corpus does not have", shared.ErrInvalidCitation, h.Chunk)
		}
		c, err := shared.NewCitation(s.Texts, h.Chunk, start, end)
		if err != nil {
			return nil, err
		}
		text, err := s.Texts.Text(h.Chunk)
		if err != nil {
			return nil, fmt.Errorf("the text of %s:%d-%d: %w", path, start, end, err)
		}
		results = append(results, Result{Citation: c, Text: text, Score: h.Score})
	}
	return results, nil
}
