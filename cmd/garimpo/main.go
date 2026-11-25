// Command garimpo is a local semantic search and RAG engine.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"

	"github.com/aluiziodeveloper/garimpo/internal/adapters/corpus"
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
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}

// scan reads the corpus one document at a time, cuts each one into chunks and prints how
// many documents, bytes and chunks it has, and the size of the largest chunk. It holds one document at a time, never the
// whole corpus.
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
	docs, size, chunks, largest := 0, 0, 0, 0
	for d, err := range corpus.Walk(os.DirFS(*dir)) {
		if err != nil {
			return fmt.Errorf("scan %s: %w", *dir, err)
		}
		docs++
		size += len(d.Text())
		for c := range cutter.Cut(d) {
			chunks++
			largest = max(largest, len(c.Text()))
		}
	}
	average := 0
	if chunks > 0 {
		average = size / chunks
	}
	_, err = fmt.Fprintf(out, "documents=%d bytes=%d chunks=%d average=%d largest=%d\n",
		docs, size, chunks, average, largest)
	return err
}
