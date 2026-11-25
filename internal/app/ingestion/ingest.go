package ingestion

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"sync"

	"golang.org/x/sync/errgroup"

	domain "github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
)

// ErrInvalidConfig is wrapped when RunIngestion gets a Config it cannot run with.
var ErrInvalidConfig = errors.New("invalid ingestion config")

// DefaultMaxPending is the MaxPending of a Config that leaves it at zero.
const DefaultMaxPending = 256

// Embed turns a chunk into a vector: the slow stage of the ingestion, a call to the model.
// An error fails that chunk only. Module 08 gives it a port, Embedder, and an adapter.
type Embed func(ctx context.Context, c domain.Chunk) error

// Config is what RunIngestion runs with.
type Config struct {
	Cutter  domain.Cutter
	Cutters int   // goroutines cutting documents; zero means 1
	Workers int   // goroutines calling Embed; at least 1
	Embed   Embed // required
	// MaxPending is the most chunks cut and not yet embedded or failed (RN03): when it is
	// reached, the cutter waits for the embedder. Zero means DefaultMaxPending.
	MaxPending int
	Progress   *Progress            // optional: counted as the run goes
	Record     func([]domain.Event) // optional: gets the run's events as they happen
}

// change is something a stage asks the run's owner to apply to the run.
type change func(*domain.Run) error

// RunIngestion runs the ingestion: it reads docs, cuts each document, embeds each chunk on
// cfg.Workers goroutines and records it all in run, which must be pending.
//
//	docs --> cut --(at most MaxPending in flight)--> embed (x Workers)
//	   \        \                                      /
//	    `--------`---------------- changes ------------'--> the run's owner (the caller)
//
// A chunk takes a slot of pending before it is counted as cut, and gives it back once it
// is counted as embedded or failed: a slow embedder stops the cutter, and the corpus is
// never held in memory (RN03, INC-07).
//
// The domain is sequential, so only the caller's goroutine touches run: the stages send
// it changes. A read error or a canceled ctx fails the run and is returned; a chunk the
// embedder fails is recorded and the run goes on.
func RunIngestion(ctx context.Context, run *domain.Run, docs iter.Seq2[domain.Document, error], cfg Config) error {
	if cfg.Workers < 1 || cfg.Embed == nil || cfg.MaxPending < 0 || cfg.Cutters < 0 {
		return fmt.Errorf("%w: %d cutters, %d workers, max pending %d, embedder set: %t",
			ErrInvalidConfig, cfg.Cutters, cfg.Workers, cfg.MaxPending, cfg.Embed != nil)
	}
	if cfg.Cutters == 0 {
		cfg.Cutters = 1
	}
	if cfg.MaxPending == 0 {
		cfg.MaxPending = DefaultMaxPending
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

	// The buffer lets the cutter get up to MaxPending chunks ahead of the embedder; the
	// slots make the count exact, chunks in the workers' hands included.
	pending := make(chan struct{}, cfg.MaxPending) // one slot per chunk in flight
	cut := make(chan domain.Chunk, cfg.MaxPending)

	// stage 1: read. An iterator is not safe for several goroutines, so one reads.
	read := make(chan domain.Document)
	g.Go(func() error {
		defer close(read)
		for d, err := range docs {
			if err != nil {
				return err
			}
			select {
			case read <- d:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return nil
	})

	// stage 2: cut, on cfg.Cutters goroutines; the last one to finish closes cut.
	var cutting sync.WaitGroup
	for range cfg.Cutters {
		cutting.Add(1)
		g.Go(func() error {
			defer cutting.Done()
			for d := range read {
				if err := cutDocument(ctx, cfg, d, pending, cut, send); err != nil {
					return err
				}
			}
			return nil
		})
	}
	g.Go(func() error {
		cutting.Wait()
		close(cut)
		return nil
	})

	for range cfg.Workers { // stage 3: embed
		g.Go(func() error {
			for c := range cut {
				err := embed(ctx, cfg, c, send)
				<-pending // counted as embedded or failed: the slot is free
				if err != nil {
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

// cutDocument records d and sends its chunks to cut, each one after taking a slot of
// pending (RN03).
func cutDocument(ctx context.Context, cfg Config, d domain.Document, pending chan struct{},
	cut chan<- domain.Chunk, send func(change) error,
) error {
	cfg.Progress.DocumentRead()
	if err := send(func(r *domain.Run) error { return r.DiscoverDocument(d) }); err != nil {
		return err
	}
	for c := range cfg.Cutter.Cut(d) {
		select {
		case pending <- struct{}{}: // waits while MaxPending chunks are in flight
		case <-ctx.Done():
			return ctx.Err()
		}
		cfg.Progress.ChunksCut(1)
		select {
		case cut <- c:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
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
