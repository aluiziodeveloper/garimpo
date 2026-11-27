package search

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/aluiziodeveloper/garimpo/internal/domain/retrieval"
)

// Collapsed is a Ranker that runs one search for the equal searches at once (INC-16): the
// callers that ask the same query with the same k while a search for it runs wait for that
// one and get its hits, with singleflight. A search after it ended runs again: it is not a
// cache.
//
// The shared search runs with a context of its own: the values of the context of the
// caller that started it, none of its cancel, and the deadline of Collapse. Each caller
// waits for the hits or for its own context, so a caller that gives up leaves alone, and
// the search goes on for the others; with no caller left, it stops at its deadline.
//
// The callers that share a search share its hits, the same slice: they must only read it.
// A Collapsed is safe for concurrent calls when its Ranker is.
type Collapsed struct {
	ranker  Ranker
	timeout time.Duration
	group   singleflight.Group
}

// Collapse returns a Collapsed over r whose shared searches last at most timeout. It panics
// if timeout is not positive: a search that nobody waits for any more must still end.
func Collapse(r Ranker, timeout time.Duration) *Collapsed {
	if timeout <= 0 {
		panic(fmt.Sprintf("search.Collapse: a deadline of %v", timeout))
	}
	return &Collapsed{ranker: r, timeout: timeout}
}

// Search returns the hits of query with k, from the search under way for them if there is
// one, or from a new one.
func (c *Collapsed) Search(ctx context.Context, query string, k int) ([]retrieval.Hit, error) {
	// k and a NUL first: the number ends at the NUL, so no two pairs make the same key.
	key := strconv.Itoa(k) + "\x00" + query
	ch := c.group.DoChan(key, func() (any, error) {
		own, cancel := context.WithTimeout(context.WithoutCancel(ctx), c.timeout)
		defer cancel()
		return c.ranker.Search(own, query, k)
	})
	select {
	case r := <-ch:
		if r.Err != nil {
			return nil, r.Err
		}
		return r.Val.([]retrieval.Hit), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
