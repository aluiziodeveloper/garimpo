package search

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// CollapsedEmbedder is an Embedder that makes one call to the model for the equal calls at
// once, the way Collapsed runs one search for the equal searches: the same query with other
// ks is another search, but the same text for the model. Its shared call has a context of
// its own and a deadline, like Collapsed's, and its callers share the vectors: read only.
type CollapsedEmbedder struct {
	embedder Embedder
	timeout  time.Duration
	group    singleflight.Group
}

// CollapseEmbedder returns a CollapsedEmbedder over e whose shared calls last at most
// timeout. It panics if timeout is not positive.
func CollapseEmbedder(e Embedder, timeout time.Duration) *CollapsedEmbedder {
	if timeout <= 0 {
		panic(fmt.Sprintf("search.CollapseEmbedder: a deadline of %v", timeout))
	}
	return &CollapsedEmbedder{embedder: e, timeout: timeout}
}

// Embed returns the vectors of texts, from the call under way for the same texts if there
// is one, or from a new one.
func (c *CollapsedEmbedder) Embed(ctx context.Context, texts []string) ([]shared.Vector, error) {
	// Each text with its length first: "a b" and the two texts "a" and "b" are other keys.
	var key strings.Builder
	for _, t := range texts {
		key.WriteString(strconv.Itoa(len(t)))
		key.WriteByte(':')
		key.WriteString(t)
	}
	ch := c.group.DoChan(key.String(), func() (any, error) {
		own, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.timeout)
		defer cancel()
		return c.embedder.Embed(own, texts)
	})
	select {
	case r := <-ch:
		if r.Err != nil {
			return nil, r.Err
		}
		return r.Val.([]shared.Vector), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
