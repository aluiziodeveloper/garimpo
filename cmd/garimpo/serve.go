package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/aluiziodeveloper/garimpo/internal/adapters/bm25"
	"github.com/aluiziodeveloper/garimpo/internal/adapters/chunktext"
	"github.com/aluiziodeveloper/garimpo/internal/adapters/corpus"
	"github.com/aluiziodeveloper/garimpo/internal/adapters/flatindex"
	"github.com/aluiziodeveloper/garimpo/internal/adapters/httpapi"
	"github.com/aluiziodeveloper/garimpo/internal/adapters/ollama"
	"github.com/aluiziodeveloper/garimpo/internal/adapters/vectorfile"
	"github.com/aluiziodeveloper/garimpo/internal/app/search"
	"github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// shutdownGrace is how long a stop waits for the requests in flight. It is more than any
// search lasts (-timeout), so a stop drops none that would have answered.
const shutdownGrace = 10 * time.Second

// serve loads the corpus and the vectors of an ingestion and serves its searches over HTTP
// until ctx ends; then it stops taking requests, lets the ones in flight answer and returns.
func serve(ctx context.Context, args []string, out io.Writer) error {
	fl := flag.NewFlagSet("serve", flag.ContinueOnError)
	dir := fl.String("corpus", "/usr/local/go/src", "the corpus directory, the one the ingestion read")
	vectorsPath := fl.String("vectors", "", "the file of vectors garimpo ingest -vectors kept (required)")
	addr := fl.String("addr", "localhost:8080", "the address to listen on")
	timeout := fl.Duration("timeout", 2*time.Second, "how long a search has to answer, from when its request arrives")
	url := fl.String("ollama", os.Getenv("GARIMPO_OLLAMA_URL"), "the Ollama server, for the vector of each query")
	model := fl.String("model", "all-minilm:22m", "the embedding model, the one of the ingestion")
	dim := fl.Int("dim", 384, "the dimension of the model's vectors")
	ctxTokens := fl.Int("context", 512, "the model's context in tokens")
	if err := fl.Parse(args); err != nil {
		return err
	}
	switch {
	case *vectorsPath == "":
		return errors.New("serve: no -vectors: run garimpo ingest -vectors first")
	case *url == "":
		return errors.New("serve: no Ollama: set GARIMPO_OLLAMA_URL or -ollama")
	case *timeout <= 0:
		return fmt.Errorf("serve: -timeout %v", *timeout)
	}
	start := time.Now()
	texts, keywords, err := loadCorpus(os.DirFS(*dir))
	if err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	vectors, err := loadVectors(*vectorsPath, *dim, texts)
	if err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	searcher := &search.Searcher{
		Ranker: &search.Hybrid{
			Embedder: &ollama.Embedder{URL: *url, Model: *model, Dim: *dim, Context: *ctxTokens},
			Vectors:  vectors,
			Keywords: keywords,
		},
		Texts: storeTexts{texts},
	}
	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	log := slog.New(slog.NewTextHandler(out, nil))
	srv := &http.Server{
		Handler: httpapi.New(searcher, *timeout, log),
		// A client that sends its request slowly holds a connection: these bound how long.
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		// The search ends at -timeout; the rest is for writing its response.
		WriteTimeout: *timeout + 5*time.Second,
		IdleTimeout:  time.Minute,
		ErrorLog:     slog.NewLogLogger(log.Handler(), slog.LevelWarn),
	}
	if _, perr := fmt.Fprintf(out, "serving %d chunks, %d with vectors, on %s (loaded in %s)\n",
		keywords.Len(), vectors.Len(), ln.Addr(), time.Since(start).Round(time.Millisecond)); perr != nil {
		_ = ln.Close()
		return perr
	}
	failed := make(chan error, 1)
	go func() { failed <- srv.Serve(ln) }()
	select {
	case serr := <-failed:
		return fmt.Errorf("serve: %w", serr)
	case <-ctx.Done():
	}
	// Shutdown closes the listener, then waits for the requests in flight, up to the grace.
	stop, cancel := context.WithTimeout(context.Background(), shutdownGrace)
	defer cancel()
	err = srv.Shutdown(stop)
	if serr := <-failed; !errors.Is(serr, http.ErrServerClosed) {
		err = errors.Join(err, serr)
	}
	_, perr := fmt.Fprintln(out, "stopped")
	return errors.Join(err, perr)
}

// loadCorpus cuts the corpus the way the ingestion did, and keeps where each chunk is and
// its terms. It takes about as long as garimpo scan.
func loadCorpus(fsys fs.FS) (*chunktext.Store, *bm25.Index, error) {
	texts, keywords := chunktext.New(fsys), bm25.New()
	for d, err := range corpus.Walk(fsys) {
		if err != nil {
			return nil, nil, err
		}
		for c := range (ingestion.Cutter{}).Cut(d) {
			if err := texts.Add(c); err != nil {
				return nil, nil, err
			}
			if err := keywords.Add(c.ID(), c.Text()); err != nil {
				return nil, nil, err
			}
		}
	}
	return texts, keywords, nil
}

// loadVectors puts the vectors of the file at path in an exact index. Every vector must be
// of a chunk of the corpus: one that is not means the corpus changed since the ingestion,
// and the citations of the search would be wrong.
func loadVectors(path string, dim int, texts *chunktext.Store) (*flatindex.Published, error) {
	entries, err := vectorfile.ReadChunks(path)
	if err != nil {
		return nil, err
	}
	index := flatindex.NewPublished(dim)
	unknown := 0
	for _, e := range entries {
		if _, _, _, ok := texts.FindChunk(e.ID); !ok {
			unknown++
			continue
		}
		if err := index.Add(e.ID, e.Vector); err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
	}
	if unknown > 0 {
		return nil, fmt.Errorf("%s: %d of %d vectors are of chunks the corpus no longer has: run the ingestion again",
			path, unknown, len(entries))
	}
	return index, nil
}

// storeTexts is the port search.Texts over a chunktext.Store, which hands out *Text.
type storeTexts struct{ *chunktext.Store }

func (s storeTexts) Text(id shared.ChunkID) (string, error) {
	t, err := s.Store.Text(id)
	if err != nil {
		return "", err
	}
	return t.String(), nil
}
