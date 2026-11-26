package ollama

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/aluiziodeveloper/garimpo/internal/app/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// Splitter is an Embedder that brings back the chunks the model's context leaves out
// (INC-08). When Next rejects a call with ErrTextTooLong, each text is embedded on its own,
// and a text that is still too long is cut in two at the line break nearest its middle;
// each half is embedded, and cut again if it must. The text gets the sum of its halves'
// vectors, each weighted by its bytes, scaled to norm 1.
//
// It cuts only at line breaks: a single line over the context still fails with
// ErrTextTooLong, as the port's contract asks.
type Splitter struct {
	Next ingestion.Embedder
}

// Embed returns one vector per text, in order. Only a call Next rejects for a text too long
// is done again, one text at a time.
func (s *Splitter) Embed(ctx context.Context, texts []string) ([]shared.Vector, error) {
	vs, err := s.Next.Embed(ctx, texts)
	if !errors.Is(err, ingestion.ErrTextTooLong) {
		return vs, err
	}
	out := make([]shared.Vector, len(texts))
	for i, text := range texts {
		v, oneErr := s.one(ctx, text)
		if oneErr != nil {
			return nil, oneErr
		}
		out[i] = v
	}
	return out, nil
}

// one embeds one text, cutting it in halves while it is too long.
func (s *Splitter) one(ctx context.Context, text string) (shared.Vector, error) {
	vs, err := s.Next.Embed(ctx, []string{text})
	if err == nil {
		if len(vs) != 1 {
			return shared.Vector{}, fmt.Errorf("split: %d vectors for 1 text", len(vs))
		}
		return vs[0], nil
	}
	a, b, ok := halve(text)
	if !ok || !errors.Is(err, ingestion.ErrTextTooLong) {
		return shared.Vector{}, err
	}
	va, err := s.one(ctx, a)
	if err != nil {
		return shared.Vector{}, err
	}
	vb, err := s.one(ctx, b)
	if err != nil {
		return shared.Vector{}, err
	}
	x, y := va.Values(), vb.Values()
	sum := make([]float32, len(x))
	for i := range x {
		sum[i] = float32(len(a))*x[i] + float32(len(b))*y[i]
	}
	return shared.NewVector(sum, len(sum))
}

// halve cuts text after the line break nearest its middle, so both halves keep whole
// lines. It reports false when text has a single line.
func halve(text string) (string, string, bool) {
	mid := len(text) / 2
	cut := -1
	if i := strings.LastIndexByte(text[:mid], '\n'); i >= 0 {
		cut = i + 1
	}
	if j := strings.IndexByte(text[mid:], '\n'); j >= 0 && mid+j+1 < len(text) {
		if after := mid + j + 1; cut < 0 || after-mid < mid-cut {
			cut = after
		}
	}
	if cut <= 0 || cut >= len(text) {
		return "", "", false
	}
	return text[:cut], text[cut:], true
}
