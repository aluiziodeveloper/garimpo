# Lab: interfaces and errors as values

`load` reads a document through the `Source` interface and parses its first line,
`title: <title>`. There are two sources: `mapSource`, in memory, for the tests, and
`dirSource`, files in a directory. `load` does not know which one it got.

Errors are values with a type and a chain:

- `ErrNotFound` is a sentinel: a fixed value that `errors.Is` finds in the chain.
- `*ParseError` is an error type that carries the line. `errors.AsType` (Go 1.26) finds it
  in the chain and returns it typed.
- `load` wraps every error with `fmt.Errorf("load %s: %w", ...)`. The message gains the
  document name, and the chain keeps the cause.

## Run it

```bash
docker compose exec dev go run ./labs/interfaces-errors goroutines.txt missing.txt broken.txt
docker compose exec dev go test -race ./labs/interfaces-errors
```

## What it shows

A real run on 2026-11-25 (Go 1.27.4), with the files in `testdata/`. Each error is handled
by what it is, never by its text:

```
goroutines.txt: title "Goroutines"
missing.txt: skipped, load missing.txt: document not found
broken.txt: fix line 1, load broken.txt: line 1: want "title: <title>"
```

## The classic mistakes

**`%v` instead of `%w`.** The message is the same, but the chain is gone: `errors.Is` no
longer finds `ErrNotFound`, and the program stops at the missing file instead of skipping
it:

```
goroutines.txt: title "Goroutines"
interfaces-errors: load missing.txt: document not found
exit status 1
```

The `errorlint` linter in CI catches it:

```
labs/interfaces-errors/main.go:47:49: non-wrapping format verb for fmt.Errorf. Use `%w` to format errors (errorlint)
```

**Returning a nil pointer as an `error`.** An interface value is nil only when it holds no
type at all. If `parse` keeps a `var perr *ParseError` and returns it, a valid document
comes back with an `error` that holds the type `*ParseError` and a nil pointer: not nil.
`load` sees an error where there is none, and the program crashes when it reads
`perr.Line`:

```
panic: runtime error: invalid memory address or nil pointer dereference
[signal SIGSEGV: segmentation violation code=0x1 addr=0x0 pc=0x4ab54e]

goroutine 1 [running]:
main.run({0x585bb8, 0x311b47902010}, {0x585bd8, 0x4b5d30}, {0x311b47918090?, 0x70?, 0x0?})
	/src/labs/interfaces-errors/main.go:110 +0x1ce
```

The test prints the strangest message of all, an error whose text is `<nil>`:

```
--- FAIL: TestLoadReadsTheTitle (0.00s)
    main_test.go:13: load a.txt: <nil>
```

`go vet` says nothing here; `staticcheck`, in the CI lint, does:

```
labs/interfaces-errors/main.go:50:5: SA4023: this comparison is always true (staticcheck)
labs/interfaces-errors/main.go:57:6: SA4023(related information): github.com/aluiziodeveloper/garimpo/labs/interfaces-errors.parse never returns a nil interface value (staticcheck)
```

The rule: a function that returns `error` returns a literal `nil` on success.
