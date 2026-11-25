// Command goroutines counts the lines of every .go file in a directory, one goroutine
// per file, and waits for all of them with a sync.WaitGroup.
//
//	docker compose exec dev go run ./labs/goroutines /usr/local/go/src/sync
package main

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: goroutines <dir>")
		os.Exit(2)
	}
	if err := run(os.Stdout, os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, "goroutines:", err)
		os.Exit(1)
	}
}

func run(w io.Writer, dir string) error {
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		return err
	}

	total := 0
	for i, r := range countLines(paths) {
		if r.err != nil {
			return r.err
		}
		total += r.lines
		if _, err = fmt.Fprintf(w, "%6d %s\n", r.lines, filepath.Base(paths[i])); err != nil {
			return err
		}
	}
	_, err = fmt.Fprintf(w, "%6d lines in %d files\n", total, len(paths))
	return err
}

type result struct {
	lines int
	err   error
}

// countLines counts the lines of each file in its own goroutine. Goroutine i writes
// only results[i], so no two goroutines touch the same memory, and Wait makes every
// write visible before countLines returns.
func countLines(paths []string) []result {
	results := make([]result, len(paths))
	var wg sync.WaitGroup
	for i, path := range paths {
		wg.Go(func() {
			results[i].lines, results[i].err = lines(path)
		})
	}
	wg.Wait()
	return results
}

func lines(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return bytes.Count(data, []byte{'\n'}), nil
}
