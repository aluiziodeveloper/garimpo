# Lab: goroutines and WaitGroup

Counts the lines of every `.go` file in a directory, one goroutine per file. A
`sync.WaitGroup` waits for all of them: `wg.Go` (Go 1.25) starts each goroutine and
`wg.Wait` blocks until every one has returned. Goroutine `i` writes only `results[i]`, so
no two goroutines touch the same memory.

## Run it

```bash
docker compose exec dev go run ./labs/goroutines /usr/local/go/src/errors
docker compose exec dev go test -race ./labs/goroutines
```

## What it shows

A real run on 2026-11-25 (Go 1.27.4), over the `errors` package of the standard library:

```
    90 errors.go
    33 errors_test.go
   238 example_test.go
    63 join.go
    76 join_test.go
   209 wrap.go
   438 wrap_test.go
  1147 lines in 7 files
```

The files come out in the order of the paths, not in the order the goroutines finish:
each result has its own slot.

## The classic mistakes

**Forgetting `wg.Wait()`.** `countLines` returns while the goroutines are still reading,
and every count is 0:

```
     0 errors.go
     0 errors_test.go
     0 example_test.go
     0 join.go
     0 join_test.go
     0 wrap.go
     0 wrap_test.go
     0 lines in 7 files
```

The tests fail, and with `-race` the race detector also names the two accesses: a
goroutine writing `results[i]` (`main.go:60`) and the test reading it (`main_test.go:32`),
with nothing ordering them. An excerpt:

```
WARNING: DATA RACE
Write at 0x00c0000f0140 by goroutine 9:
  github.com/aluiziodeveloper/garimpo/labs/goroutines.countLines.func1()
      /src/labs/goroutines/main.go:60 +0x90

Previous read at 0x00c0000f0140 by goroutine 8:
  github.com/aluiziodeveloper/garimpo/labs/goroutines.TestCountLines()
      /src/labs/goroutines/main_test.go:32 +0x749
...
--- FAIL: TestCountLines (0.00s)
    main_test.go:37: /tmp/TestCountLines2353148018/001/a.txt: 0 lines, want 3
    main_test.go:37: /tmp/TestCountLines2353148018/001/c.txt: 0 lines, want 1
    testing.go:1867: race detected during execution of test
```

**Calling `wg.Add` inside the new goroutine.** The older pattern is `wg.Add(1)` before
`go func`, and `defer wg.Done()` inside it. With `Add` inside the goroutine, `Wait` may run
before any `Add` and return at once. `go vet` (and the `govet` linter in CI) catches it:

```
labs/goroutines/main.go:60:10: WaitGroup.Add called from inside new goroutine
```

`wg.Go` does the `Add` before starting the goroutine, so this mistake cannot happen.
