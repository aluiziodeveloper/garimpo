// Command garimpo is a local semantic search and RAG engine.
package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
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
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}
}
