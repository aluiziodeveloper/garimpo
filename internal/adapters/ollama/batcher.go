package ollama

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aluiziodeveloper/garimpo/internal/app/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// Batcher is an ingestion.Embedder that gathers the texts of concurrent calls into one
// call to another Embedder, the way Ollama's /api/embed takes many inputs at once. A batch
// goes out when it holds Size texts, or Wait after its first text arrived, whichever
// comes first. Close stops it.
//
// If a batch fails because one of its texts is over the model's context, each text is
// sent again on its own, so only that text fails (INC-08).
type Batcher struct {
	next ingestion.Embedder
	size int
	wait time.Duration

	requests chan request
	stop     chan struct{}
	done     sync.WaitGroup
}

// request is one call to Embed waiting for its share of a batch.
type request struct {
	ctx   context.Context
	texts []string
	reply chan result // buffered: the batch never waits for a caller that gave up
}

type result struct {
	vs  []shared.Vector
	err error
}

// NewBatcher returns a running Batcher in front of next. size must be at least 1 and
// wait above 0.
func NewBatcher(next ingestion.Embedder, size int, wait time.Duration) (*Batcher, error) {
	if size < 1 || wait <= 0 {
		return nil, fmt.Errorf("ollama: batch of %d texts with a wait of %v", size, wait)
	}
	b := &Batcher{next: next, size: size, wait: wait, requests: make(chan request), stop: make(chan struct{})}
	b.done.Go(b.loop)
	return b, nil
}

// Embed queues texts for the next batch and waits for their vectors.
func (b *Batcher) Embed(ctx context.Context, texts []string) ([]shared.Vector, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(texts) == 0 {
		return nil, nil
	}
	r := request{ctx: ctx, texts: texts, reply: make(chan result, 1)}
	select {
	case b.requests <- r:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-b.stop:
		return nil, errClosed
	}
	select {
	case res := <-r.reply:
		return res.vs, res.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

var errClosed = errors.New("ollama: batcher closed")

// Close stops the batcher once the batches already sent have their answers. Calls to
// Embed after Close fail.
func (b *Batcher) Close() {
	close(b.stop)
	b.done.Wait()
}

// loop gathers requests into batches. It is the only goroutine that touches the batch.
func (b *Batcher) loop() {
	var (
		batch []request
		count int
		timer *time.Timer
		fire  <-chan time.Time // nil while the batch is empty: a nil channel never fires
	)
	send := func() {
		if timer != nil {
			timer.Stop()
		}
		fire = nil
		pending := batch
		batch, count = nil, 0
		b.done.Go(func() { b.embed(pending) })
	}
	for {
		select {
		case r := <-b.requests:
			if len(batch) == 0 {
				timer = time.NewTimer(b.wait)
				fire = timer.C
			}
			batch = append(batch, r)
			count += len(r.texts)
			if count >= b.size {
				send()
			}
		case <-fire:
			send()
		case <-b.stop:
			if len(batch) > 0 {
				send()
			}
			return
		}
	}
}

// embed sends one batch and hands each request its vectors. The batch's own context ends
// only if every caller in it gave up.
func (b *Batcher) embed(batch []request) {
	ctx, cancel := allDone(batch)
	defer cancel()
	var texts []string
	for _, r := range batch {
		texts = append(texts, r.texts...)
	}
	vs, err := b.next.Embed(ctx, texts)
	if errors.Is(err, ingestion.ErrTextTooLong) && len(batch) > 1 {
		for _, r := range batch { // isolate the long text: one call per request
			one, oneErr := b.next.Embed(r.ctx, r.texts)
			r.reply <- result{one, oneErr}
		}
		return
	}
	if err == nil && len(vs) != len(texts) {
		err = fmt.Errorf("%w: %d vectors for %d texts", ingestion.ErrBrokenEmbedder, len(vs), len(texts))
	}
	for _, r := range batch {
		if err != nil {
			r.reply <- result{nil, err}
			continue
		}
		r.reply <- result{vs[:len(r.texts):len(r.texts)], nil}
		vs = vs[len(r.texts):]
	}
}

// allDone returns a context that is canceled once the context of every request is, and
// a function that releases it.
func allDone(batch []request) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	var left atomic.Int64
	left.Store(int64(len(batch)))
	stops := make([]func() bool, len(batch))
	for i, r := range batch {
		stops[i] = context.AfterFunc(r.ctx, func() {
			if left.Add(-1) == 0 {
				cancel()
			}
		})
	}
	return ctx, func() {
		for _, stop := range stops {
			stop()
		}
		cancel()
	}
}
