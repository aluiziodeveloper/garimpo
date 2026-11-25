// Command load-all looks for one file of the corpus, the Go standard library, in two ways.
// The first way reads every file into a slice and then searches the slice, the way the
// first ingestion did (INC-06). The second reads one file at a time and stops at the one
// it wants. Both print how much heap they used.
//
//	docker compose run --rm lab go run ./labs/load-all
//	docker compose run --rm lab go run ./labs/load-all -mode each
package main

import (
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path"
	"runtime"
	"strings"
)

func main() {
	root := flag.String("root", "/usr/local/go/src", "the corpus directory")
	mode := flag.String("mode", "all", "how to read the corpus: all or each")
	want := flag.String("file", "strings/builder.go", "the file to look for")
	flag.Parse()

	if err := find(os.DirFS(*root), *mode, *want); err != nil {
		fmt.Fprintln(os.Stderr, "load-all:", err)
		os.Exit(1)
	}
}

// doc is one file of the corpus: its path, relative to the corpus root, and its text.
type doc struct {
	path string
	text string
}

var (
	errMode     = errors.New("unknown mode")
	errNotFound = errors.New("file not in the corpus")
	errFound    = errors.New("found") // stops each early; never returned by find
)

// find looks for the file want in the corpus and prints its size, the heap still in use
// after a collection, and the total the walk allocated.
func find(fsys fs.FS, mode, want string) error {
	var (
		found doc
		read  int
	)
	switch mode {
	case "all":
		docs, err := loadAll(fsys)
		if err != nil {
			return err
		}
		read = len(docs)
		for _, d := range docs {
			if d.path == want {
				found = d
				break
			}
		}
		defer runtime.KeepAlive(docs)
	case "each":
		err := each(fsys, func(d doc) error {
			read++
			if d.path == want {
				found = d
				return errFound
			}
			return nil
		})
		if err != nil && !errors.Is(err, errFound) {
			return err
		}
	default:
		return fmt.Errorf("%w %q: want all or each", errMode, mode)
	}
	if found.path == "" {
		return fmt.Errorf("%w: %s", errNotFound, want)
	}
	var m runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&m)
	fmt.Printf("mode=%s read=%d files found=%s (%d bytes) heap=%.1f MB allocated=%.1f MB\n",
		mode, read, found.path, len(found.text), mb(m.HeapAlloc), mb(m.TotalAlloc))
	return nil
}

// mb converts bytes to mebibytes.
func mb(b uint64) float64 { return float64(b) / (1 << 20) }

// loadAll reads every library source of the corpus into memory and returns them in
// lexical order.
func loadAll(fsys fs.FS) ([]doc, error) {
	var docs []doc
	err := each(fsys, func(d doc) error {
		docs = append(docs, d)
		return nil
	})
	return docs, err
}

// each calls fn with every library source of the corpus, in lexical order, one file at a
// time. It stops at the first error, its own or one fn returns, and returns it.
func each(fsys fs.FS, fn func(doc) error) error {
	return fs.WalkDir(fsys, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if skipDir(p, d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if !librarySource(p) {
			return nil
		}
		b, err := fs.ReadFile(fsys, p)
		if err != nil {
			return err
		}
		return fn(doc{path: p, text: string(b)})
	})
}

// skipDir reports whether the walk skips the directory: test data, vendored code and the
// commands of the toolchain are not part of the library.
func skipDir(p, name string) bool {
	return name == "testdata" || name == "vendor" || p == "cmd"
}

// librarySource reports whether the file is a Go source of the library, not a test.
func librarySource(p string) bool {
	return path.Ext(p) == ".go" && !strings.HasSuffix(p, "_test.go")
}
