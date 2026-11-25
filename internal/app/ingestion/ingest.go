package ingestion

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"golang.org/x/sync/errgroup"

	domain "github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
)

// ErrInvalidConfig is wrapped when RunIngestion gets a Config it cannot run with.
var ErrInvalidConfig = errors.New("invalid ingestion config")

// Embed turns a chunk into a vector: the slow stage of the ingestion, a call to the model.
// An error fails that chunk only. Module 08 gives it a port, Embedder, and an adapter.
type Embed func(ctx context.Context, c domain.Chunk) error

// Config is what RunIngestion runs with.
type Config struct {
	Cutter   domain.Cutter
	Workers  int                  // goroutines calling Embed; at least 1
	Embed    Embed                // required
	Progress *Progress            // optional: counted as the run goes
	Record   func([]domain.Event) // optional: gets the run's events as they happen
}

// change is something a stage asks the run's owner to apply to the run.
type change func(*domain.Run) error

// RunIngestion runs the ingestion: it reads docs, cuts each document, embeds each chunk on
// cfg.Workers goroutines and records it all in run, which must be pending.
//
//	docs --> cut --> queue --> embed (x Workers)
//	   \        \                 /
//	    `--------`-- changes ----'--> the run's owner (the caller's goroutine)
//
// The domain is sequential, so only the caller's goroutine touches run: the stages send
// it changes. A read error or a canceled ctx fails the run and is returned; a chunk the
// embedder fails is recorded and the run goes on.
func RunIngestion(ctx context.Context, run *domain.Run, docs iter.Seq2[domain.Document, error], cfg Config) error {
	if cfg.Workers < 1 || cfg.Embed == nil {
		return fmt.Errorf("%w: %d workers, embedder set: %t", ErrInvalidConfig, cfg.Workers, cfg.Embed != nil)
	}
	if cfg.Progress == nil {
		cfg.Progress = new(Progress)
	}
	if cfg.Record == nil {
		cfg.Record = func([]domain.Event) {}
	}
	if err := run.Start(); err != nil {
		return err
	}
	cfg.Record(run.TakeEvents())

	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	g, ctx := errgroup.WithContext(ctx)
	changes := make(chan change)
	send := func(c change) error {
		select {
		case changes <- c:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	cut := make(chan domain.Chunk)
	g.Go(func() error { // stage 1: read and cut
		defer close(cut)
		for d, err := range docs {
			if err != nil {
				return err
			}
			cfg.Progress.DocumentRead()
			if err := send(func(r *domain.Run) error { return r.DiscoverDocument(d) }); err != nil {
				return err
			}
			for c := range cfg.Cutter.Cut(d) {
				cfg.Progress.ChunksCut(1)
				select {
				case cut <- c:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
		}
		return nil
	})

	queued := make(chan domain.Chunk)
	g.Go(func() error { return queue(ctx, cut, queued) }) // stage 2: the cutter never waits

	for range cfg.Workers { // stage 3: embed
		g.Go(func() error {
			for c := range queued {
				if err := embed(ctx, cfg, c, send); err != nil {
					return err
				}
			}
			return nil
		})
	}

	var stagesErr error
	go func() {
		stagesErr = g.Wait()
		close(changes)
	}()
	var applyErr error
	for c := range changes {
		if applyErr != nil {
			continue // drain: the stages are stopping
		}
		if err := c(run); err != nil {
			applyErr = err
			cancel(err)
		}
		cfg.Record(run.TakeEvents())
	}
	// changes is closed: g.Wait returned and stagesErr is set.
	err := stagesErr
	if applyErr != nil {
		err = applyErr // the stages only saw the cancel it caused
	}
	if err != nil {
		if ferr := run.Fail(err.Error()); ferr != nil {
			return errors.Join(err, ferr)
		}
		cfg.Record(run.TakeEvents())
		return err
	}
	if err := run.Complete(); err != nil {
		return err
	}
	cfg.Record(run.TakeEvents())
	return nil
}

// embed calls cfg.Embed on c and sends the outcome to the run's owner. Only a canceled
// context is an error of the stage; an embedder error fails the chunk.
func embed(ctx context.Context, cfg Config, c domain.Chunk, send func(change) error) error {
	err := cfg.Embed(ctx, c)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	id := c.ID()
	if err != nil {
		cfg.Progress.ChunkFailed()
		reason := err.Error()
		return send(func(r *domain.Run) error { return r.FailChunk(id, reason) })
	}
	cfg.Progress.ChunkEmbedded()
	return send(func(r *domain.Run) error { return r.ConfirmChunk(id) })
}

// queue passes every chunk from in to out, in order, holding the ones out is not ready
// for, so that whoever sends on in never waits for whoever receives on out.
func queue(ctx context.Context, in <-chan domain.Chunk, out chan<- domain.Chunk) error {
	defer close(out)
	var held []domain.Chunk
	for in != nil || len(held) > 0 {
		var send chan<- domain.Chunk // nil while nothing is held: that case never fires
		var next domain.Chunk
		if len(held) > 0 {
			send, next = out, held[0]
		}
		select {
		case c, ok := <-in:
			if !ok {
				in = nil
				continue
			}
			held = append(held, c)
		case send <- next:
			held = held[1:]
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	return nil
}
