// Command garimpo is a local semantic search and RAG engine.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/aluiziodeveloper/garimpo/internal/adapters/corpus"
	"github.com/aluiziodeveloper/garimpo/internal/adapters/journal"
	"github.com/aluiziodeveloper/garimpo/internal/adapters/ollama"
	"github.com/aluiziodeveloper/garimpo/internal/adapters/vectorfile"
	ingest "github.com/aluiziodeveloper/garimpo/internal/app/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/app/limiter"
	"github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
)

// version is replaced at build time in the release module (-ldflags "-X main.version=...").
var version = "dev"

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "garimpo:", err)
		os.Exit(2)
	}
}

// run holds the program logic, away from os.Args and os.Exit, so a test can call it.
func run(args []string, out io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: garimpo <command>")
	}
	switch args[0] {
	case "version":
		_, err := fmt.Fprintf(out, "garimpo %s (%s)\n", version, runtime.Version())
		return err
	case "scan":
		return scan(args[1:], out)
	case "ingest":
		return ingestCorpus(args[1:], out)
	case "serve":
		// SIGINT and SIGTERM stop the server: it lets the requests in flight answer.
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		return serve(ctx, args[1:], out)
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

// scan reads the corpus one document at a time, cuts each one into chunks and prints the
// survey: how many documents, bytes and chunks it has, and the size of the largest chunk.
func scan(args []string, out io.Writer) error {
	fl := flag.NewFlagSet("scan", flag.ContinueOnError)
	dir := fl.String("corpus", "/usr/local/go/src", "the corpus directory")
	window := fl.Int("window", ingestion.DefaultWindow, "the most bytes a chunk holds")
	if err := fl.Parse(args); err != nil {
		return err
	}
	cutter, err := ingestion.NewCutter(*window)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}
	survey, err := ingest.SurveyCorpus(corpus.Walk(os.DirFS(*dir)), cutter)
	if err != nil {
		return fmt.Errorf("scan %s: %w", *dir, err)
	}
	_, err = fmt.Fprintln(out, survey)
	return err
}

// ingestCorpus runs the ingestion over the corpus with an embedding model on Ollama and
// prints how the run ended, its counts and how long it took.
func ingestCorpus(args []string, out io.Writer) error {
	fl := flag.NewFlagSet("ingest", flag.ContinueOnError)
	dir := fl.String("corpus", "/usr/local/go/src", "the corpus directory")
	workers := fl.Int("workers", 4, "goroutines calling the model")
	url := fl.String("ollama", os.Getenv("GARIMPO_OLLAMA_URL"), "the Ollama server")
	model := fl.String("model", "all-minilm:22m", "the embedding model")
	dim := fl.Int("dim", 384, "the dimension of the model's vectors")
	ctxTokens := fl.Int("context", 512, "the model's context in tokens; a longer chunk fails")
	batch := fl.Int("batch", 1, "the most chunks in one request to the model; 1 sends them one by one")
	wait := fl.Duration("batch-wait", 10*time.Millisecond, "how long a batch waits to fill up")
	adaptive := fl.Bool("adaptive", false, "move the calls to the model at the same time with their latency, up to -workers (RN04)")
	journalPath := fl.String("journal", "", "the file of the run's events; a run stopped midway resumes from it (RN02)")
	vectorsPath := fl.String("vectors", "", "the file to keep the vector of every chunk embedded in, with its chunk, for garimpo serve; a resumed run adds to it")
	if err := fl.Parse(args); err != nil {
		return err
	}
	if *url == "" {
		return errors.New("ingest: no Ollama: set GARIMPO_OLLAMA_URL or -ollama")
	}
	threads, _ := strconv.Atoi(os.Getenv("GARIMPO_OLLAMA_NUM_THREAD")) // empty: Ollama decides
	var j ingest.Journal
	run, err := ingestion.NewRun("ingest")
	if *journalPath != "" {
		f, jerr := journal.Open(*journalPath)
		if jerr != nil {
			return fmt.Errorf("ingest: %w", jerr)
		}
		defer func() { _ = f.Close() }() // Append synced every event: a failed Close loses none
		j = f
		run, err = ingest.OpenRun(f, "ingest")
	}
	if err != nil {
		return fmt.Errorf("ingest: %w", err)
	}
	if run.State() == ingestion.Completed {
		_, err = fmt.Fprintf(out, "the run in %s is completed already: nothing to do\n", *journalPath)
		return err
	}
	if run.State() == ingestion.Failed { // final: no transition leaves it
		return fmt.Errorf("ingest: the run in %s failed (%s): remove the journal to start over", *journalPath, run.Reason())
	}
	var embedder ingest.Embedder = &ollama.Embedder{
		URL: *url, Model: *model, Dim: *dim, Context: *ctxTokens, NumThread: threads,
	}
	if *batch > 1 {
		b, berr := ollama.NewBatcher(embedder, *batch, *wait)
		if berr != nil {
			return fmt.Errorf("ingest: %w", berr)
		}
		defer b.Close()
		embedder = b
	}
	progress := new(ingest.Progress)
	cfg := ingest.Config{Workers: *workers, Embedder: embedder, Progress: progress, Journal: j}
	if *adaptive {
		l, lerr := limiter.New(limiter.Config{Max: *workers})
		if lerr != nil {
			return fmt.Errorf("ingest: %w", lerr)
		}
		cfg.Limiter = l
	}
	// The last thing before the run: from here on, every path closes the file of vectors.
	var vectors *vectorfile.Writer
	if *vectorsPath != "" {
		if vectors, err = vectorfile.Open(*vectorsPath, *dim); err != nil {
			return fmt.Errorf("ingest: %w", err)
		}
		cfg.Vectors = vectors
	}
	// SIGINT (Ctrl+C) and SIGTERM (docker stop, kill) stop the run: it drains and pauses.
	// After the first signal, the default comes back, and a second one ends the process.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	context.AfterFunc(ctx, stop)
	note := ""
	start := time.Now()
	err = ingest.RunIngestion(ctx, run, corpus.Walk(os.DirFS(*dir)), cfg)
	if run.State() == ingestion.Paused && ctx.Err() != nil {
		err = nil // a stop by request: the run paused and its journal resumes it
		if j == nil {
			note += " (no -journal: the next run starts over)"
		}
	}
	if cfg.Limiter != nil {
		note = fmt.Sprintf(" (adaptive limit %d of %d)", cfg.Limiter.Limit(), *workers) + note
	}
	_, perr := fmt.Fprintf(out, "%s %s in %s%s\n", run.State(), progress.Snapshot(), time.Since(start).Round(time.Millisecond), note)
	var cerr error
	if vectors != nil {
		cerr = vectors.Close()
	}
	return errors.Join(err, perr, cerr)
}
