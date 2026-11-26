// Command garimpo is a local semantic search and RAG engine.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"
	"strconv"
	"time"

	"github.com/aluiziodeveloper/garimpo/internal/adapters/corpus"
	"github.com/aluiziodeveloper/garimpo/internal/adapters/ollama"
	ingest "github.com/aluiziodeveloper/garimpo/internal/app/ingestion"
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
	if err := fl.Parse(args); err != nil {
		return err
	}
	if *url == "" {
		return errors.New("ingest: no Ollama: set GARIMPO_OLLAMA_URL or -ollama")
	}
	threads, _ := strconv.Atoi(os.Getenv("GARIMPO_OLLAMA_NUM_THREAD")) // empty: Ollama decides
	run, err := ingestion.NewRun("ingest")
	if err != nil {
		return err
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
	cfg := ingest.Config{Workers: *workers, Embedder: embedder, Progress: progress}
	start := time.Now()
	err = ingest.RunIngestion(context.Background(), run, corpus.Walk(os.DirFS(*dir)), cfg)
	_, perr := fmt.Fprintf(out, "%s %s in %s\n", run.State(), progress.Snapshot(), time.Since(start).Round(time.Millisecond))
	return errors.Join(err, perr)
}
