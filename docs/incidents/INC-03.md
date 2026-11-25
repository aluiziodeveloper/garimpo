# INC-03: the code that does not say what it does

- **Date:** 2026-11-25
- **Module:** 03, Garimpo's architecture
- **Reproduction:** the program in [Reproduction](#reproduction), the first prototype's search

## Summary

A search written with `[]float32`, `string` and `int` ranked a chunk with a score of 3.000,
compared a vector of another model by its first 4 of 8 values, and cited line 1890 of a file
with 66 lines. Nothing failed: every bug came out as a plausible number.

## Impact

Run on 2026-11-25 with `golang:1.27.4`
(`docker run --rm -v "$PWD":/src -w /src golang:1.27.4 go run .`):

```
rank  score  chunk
1     3.000  sync/rwmutex.go:1-40
2     0.973  sync/mutex.go:1-30
3     0.200  sync/once.go:1-20
answer cites sync/mutex.go:1890
sync/mutex.go has 66 lines
```

- `sync/rwmutex.go` ranks first with 3.000: its vector was never normalized, and a cosine
  similarity is never above 1.
- `sync/once.go` has a vector of 8 dimensions; `score` reads the first 4 and answers 0.200.
- The answer cites `sync/mutex.go:1890`; the file has 66 lines.

## Cause

The types say nothing. `score` takes any two slices, `cite` takes any path and any line, and
every rule lives in a comment or in someone's head. The function that has to check is the
one that forgets.

## Fix

Value objects in the shared kernel ([ADR 0002](../adr/0002-bounded-contexts-and-layers.md)), each with a constructor that validates:

| Bug | Now |
|---|---|
| Vector never normalized | `NewVector` always returns norm 1 (RN07) |
| Vector of another model | `NewVector(values, 4)` with 8 values fails: `invalid vector: 8 dimensions, the model has 4`; `Dot` panics on a mismatch |
| Line 1890 of a 66-line file | `NewCitation` fails unless the lines are inside a chunk of the corpus (RN01) |

## Prevention

- Tests for each value object, including property tests for `Vector`
  (`internal/domain/shared`).
- The lint keeps the domain pure (`depguard`, `forbidigo`), so these types stay the only
  way in.

## Reproduction

Save it as `main.go` in an empty folder, with a `go.mod` that says `module inc03` and
`go 1.27.4`, and run the command in [Impact](#impact).

```go
// The "before" code of INC-03: a search written with plain slices and strings.
// Nothing in the types says that a vector has the model's dimension and norm 1, or that a
// citation points to lines that exist. Every function accepts anything, and the bugs
// come out as plausible numbers.
//
//	docker run --rm -v "$PWD":/src -w /src golang:1.27.4 go run .
package main

import (
	"bufio"
	"fmt"
	"os"
	"slices"
)

// chunk as the first prototype kept it: a path, a line range and a vector.
type chunk struct {
	path       string
	start, end int
	vec        []float32
}

// score is "cosine similarity", as long as both vectors have norm 1 and the same size.
func score(a, b []float32) float32 {
	var s float32
	for i := range min(len(a), len(b)) {
		s += a[i] * b[i]
	}
	return s
}

// cite formats a citation. Any path and any line are accepted.
func cite(path string, line int) string {
	return fmt.Sprintf("%s:%d", path, line)
}

func main() {
	query := []float32{0.5, 0.5, 0.5, 0.5} // norm 1, 4 dimensions (the "model")

	index := []chunk{
		{"sync/mutex.go", 1, 30, []float32{0.6, 0.6, 0.4, 0.346}},                // normalized
		{"sync/rwmutex.go", 1, 40, []float32{2, 1, 2, 1}},                         // never normalized
		{"sync/once.go", 1, 20, []float32{0.1, 0.1, 0.1, 0.1, 0.5, 0.5, 0.5, 0.5}}, // another model, 8 dimensions
	}

	slices.SortFunc(index, func(a, b chunk) int {
		sa, sb := score(query, a.vec), score(query, b.vec)
		switch {
		case sa > sb:
			return -1
		case sa < sb:
			return 1
		}
		return 0
	})

	fmt.Println("rank  score  chunk")
	for i, c := range index {
		fmt.Printf("%d     %.3f  %s:%d-%d\n", i+1, score(query, c.vec), c.path, c.start, c.end)
	}

	answer := cite("sync/mutex.go", 1890)
	fmt.Println("answer cites", answer)
	fmt.Printf("sync/mutex.go has %d lines\n", countLines("/usr/local/go/src/sync/mutex.go"))
}

func countLines(path string) int {
	f, err := os.Open(path)
	if err != nil {
		return -1
	}
	defer f.Close()
	n := 0
	for s := bufio.NewScanner(f); s.Scan(); {
		n++
	}
	return n
}
```
