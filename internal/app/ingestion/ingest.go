package ingestion

import (
	"context"
	"errors"
	"fmt"
	"iter"

	"golang.org/x/sync/errgroup"

	"github.com/aluiziodeveloper/garimpo/internal/app/limiter"
	domain "github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// ErrInvalidConfig is wrapped when RunIngestion gets a Config it cannot run with.
var ErrInvalidConfig = errors.New("invalid ingestion config")

// DefaultMaxPending is the MaxPending of a Config that leaves it at zero.
const DefaultMaxPending = 256

// Config is what RunIngestion runs with.
type Config struct {
	Cutter   domain.Cutter
	Workers  int      // goroutines calling the embedder; at least 1
	Embedder Embedder // required: the slow stage, a call to the model
	// MaxPending is the most chunks cut and not yet embedded or failed (RN03): when it is
	// reached, the cutter waits for the embedder. Zero means DefaultMaxPending.
	MaxPending int
	// Limiter, when set, bounds the calls to the embedder at the same time and moves the
	// bound with their latency (RN04). Workers still bounds them: a limit over Workers has
	// no effect.
	Limiter  *limiter.Limiter
	Progress *Progress // optional: counted as the run goes
	// Journal, when set, gets the run's events as they happen, and an event is durable
	// before the run goes on (RN02). An Append that fails fails the run.
	Journal Journal
	// Vectors, when set, keeps the vector of every chunk embedded, before the chunk is
	// confirmed: a chunk the journal confirms has its vector. An Add that fails fails the
	// run.
	Vectors VectorStore
}

// change is something a stage asks the run's owner to apply to the run.
type change func(*domain.Run) error

// RunIngestion runs the ingestion: it reads docs, cuts each document, embeds each chunk on
// cfg.Workers goroutines and records it all in run, which must be pending or paused. A
// paused run, such as one OpenRun rebuilt from a journal, resumes: the chunks it handled
// already are skipped, never embedded again (RN02).
//
//	docs --> cut --(at most MaxPending in flight)--> embed (x Workers)
//	   \        \                                      /
//	    `--------`---------------- changes ------------'--> the run's owner (the caller)
//
// A chunk takes a slot of pending before it is counted as cut, and gives it back once it
// is counted as embedded or failed: a slow embedder stops the cutter, and the corpus is
// never held in memory (RN03, INC-07).
//
// With cfg.Limiter, a worker takes a permit before each call to the embedder: the calls
// at the same time follow the limit, not the number of workers (RN04, INC-09).
//
// The domain is sequential, so only the caller's goroutine touches run: the stages send
// it changes. A read error fails the run and is returned; a chunk the embedder fails is
// recorded and the run goes on.
//
// Canceling ctx is a request to stop, such as a SIGTERM: no new document, chunk or call
// starts, the calls to the embedder in flight finish and are recorded, the run pauses and
// RunIngestion returns ctx's error. The chunks cut and not sent are cut again when the run
// resumes. A call that never returns holds the stop: the caller decides how long to wait.
func RunIngestion(ctx context.Context, run *domain.Run, docs iter.Seq2[domain.Document, error], cfg Config) error {
	if cfg.Workers < 1 || cfg.Embedder == nil || cfg.MaxPending < 0 {
		return fmt.Errorf("%w: %d workers, max pending %d, embedder set: %t",
			ErrInvalidConfig, cfg.Workers, cfg.MaxPending, cfg.Embedder != nil)
	}
	if cfg.MaxPending == 0 {
		cfg.MaxPending = DefaultMaxPending
	}
	if cfg.Progress == nil {
		cfg.Progress = new(Progress)
	}
	if cfg.Journal == nil {
		cfg.Journal = discard{}
	}
	begin := run.Start
	if run.State() == domain.Paused {
		begin = run.Resume
	}
	if err := begin(); err != nil {
		return err
	}
	if err := cfg.Journal.Append(run.TakeEvents()); err != nil {
		return err
	}
	// The cutter skips what the run handled already. It is a copy: only the run's owner
	// may read the run once the stages start.
	handled := make(map[shared.ChunkID]struct{})
	for id := range run.HandledChunks() {
		handled[id] = struct{}{}
	}

	// Two contexts. work ends only when the run fails: the calls to the model and the
	// changes to the run use it, so a stop lets them finish. waits ends when the run fails
	// or the caller stops it (ctx): the stages wait on it for the next document, chunk or
	// permit, and a stop ends those waits.
	work, cancel := context.WithCancelCause(context.WithoutCancel(ctx))
	defer cancel(nil)
	g, work := errgroup.WithContext(work)
	waits, endWaits := context.WithCancel(work)
	defer endWaits()
	stopWaits := context.AfterFunc(ctx, endWaits)
	defer stopWaits()
	stopped := func() bool { return ctx.Err() != nil }

	changes := make(chan change)
	send := func(c change) error {
		select {
		case changes <- c:
			return nil
		case <-work.Done():
			return work.Err()
		}
	}

	// The buffer lets the cutter get up to MaxPending chunks ahead of the embedder; the
	// slots make the count exact, chunks in the workers' hands included.
	pending := make(chan struct{}, cfg.MaxPending) // one slot per chunk in flight
	cut := make(chan domain.Chunk, cfg.MaxPending)
	g.Go(func() error { // stage 1: read and cut
		defer close(cut)
		for d, err := range docs {
			if err != nil {
				return err
			}
			if stopped() {
				return nil
			}
			cfg.Progress.DocumentRead()
			if err := send(func(r *domain.Run) error { return r.DiscoverDocument(d) }); err != nil {
				return err
			}
			for c := range cfg.Cutter.Cut(d) {
				if _, done := handled[c.ID()]; done {
					cfg.Progress.ChunkSkipped()
					continue
				}
				select {
				case pending <- struct{}{}: // waits while MaxPending chunks are in flight
				case <-waits.Done():
					return failure(work)
				}
				cfg.Progress.ChunksCut(1)
				select {
				case cut <- c:
				case <-waits.Done():
					return failure(work)
				}
			}
		}
		return nil
	})

	for range cfg.Workers { // stage 2: embed
		g.Go(func() error {
			for {
				// A stop takes no new chunk: the ones still in cut were never sent to the
				// model, and a resumed run cuts them again.
				var c domain.Chunk
				select {
				case next, ok := <-cut:
					if !ok {
						return nil
					}
					c = next
				case <-waits.Done():
					return failure(work)
				}
				if stopped() {
					<-pending
					return nil
				}
				err := embed(work, waits, cfg, c, send)
				<-pending // counted as embedded or failed: the slot is free
				if err != nil {
					return err
				}
			}
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
		err := c(run)
		if err == nil {
			err = cfg.Journal.Append(run.TakeEvents()) // durable before the run goes on
		}
		if err != nil {
			applyErr = err
			cancel(err)
		}
	}
	// changes is closed: g.Wait returned and stagesErr is set.
	err := stagesErr
	if applyErr != nil {
		err = applyErr // the stages only saw the cancel it caused
	}
	switch {
	case err != nil:
		if ferr := run.Fail(err.Error()); ferr != nil {
			return errors.Join(err, ferr)
		}
		return errors.Join(err, cfg.Journal.Append(run.TakeEvents()))
	case stopped():
		// Every call in flight is recorded: pause, and the next run resumes from here.
		if perr := run.Pause(); perr != nil {
			return errors.Join(ctx.Err(), perr)
		}
		return errors.Join(ctx.Err(), cfg.Journal.Append(run.TakeEvents()))
	}
	if err := run.Complete(); err != nil {
		return err
	}
	return cfg.Journal.Append(run.TakeEvents())
}

// failure returns why the stages must end when a wait ends: the run's error if it failed,
// or nil if the caller stopped it.
func failure(work context.Context) error {
	return work.Err()
}

// discard is the Journal of a run without one: the events are dropped.
type discard struct{}

func (discard) Append([]domain.Event) error { return nil }

func (discard) Replay() iter.Seq2[domain.Event, error] {
	return func(func(domain.Event, error) bool) {}
}

// embed asks cfg.Embedder for the vector of c and sends the outcome to the run's owner.
// A canceled context and a broken contract are errors of the stage; an embedder error
// fails the chunk. The vector goes to cfg.Vectors, when set, before the chunk is confirmed;
// a vector that cannot be kept is an error of the stage, like a journal that cannot append.
//
// The call and the send use ctx, which ends only when the run fails; the wait for a
// permit uses waits, which a stop ends too: a worker that has no permit yet takes none.
func embed(ctx, waits context.Context, cfg Config, c domain.Chunk, send func(change) error) error {
	var permit limiter.Permit
	if cfg.Limiter != nil {
		var err error
		if permit, err = cfg.Limiter.Acquire(waits); err != nil {
			return failure(ctx) // a stop or a failure ended the wait
		}
	}
	vs, err := cfg.Embedder.Embed(ctx, []string{c.Text()})
	if cfg.Limiter != nil {
		if err != nil || ctx.Err() != nil {
			permit.Discard() // a failure says nothing about the model's queue
		} else {
			permit.Done()
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	id := c.ID()
	if err != nil {
		cfg.Progress.ChunkFailed()
		reason := err.Error()
		return send(func(r *domain.Run) error { return r.FailChunk(id, reason) })
	}
	if len(vs) != 1 {
		return fmt.Errorf("%w: %d vectors for 1 text", ErrBrokenEmbedder, len(vs))
	}
	if cfg.Vectors != nil {
		if err := cfg.Vectors.Add(id, vs[0]); err != nil {
			return fmt.Errorf("keeping the vector of chunk %s: %w", id, err)
		}
	}
	cfg.Progress.ChunkEmbedded()
	return send(func(r *domain.Run) error { return r.ConfirmChunk(id) })
}
