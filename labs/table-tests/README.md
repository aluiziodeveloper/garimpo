# Lab: table tests and subtests

`chunkWords` cuts a text into chunks of at most `size` words, with `strings.Fields` and
`slices.Chunk` (Go 1.23). The function is small on purpose: the lab is about its test.

`TestChunkWords` is a table test: a slice of cases, each with a name, the input and the
expected output, and one loop that runs every case as a subtest with `t.Run`. Adding a
case is adding a line.

## Run it

```bash
docker compose exec dev go run ./labs/table-tests -size 4 "a goroutine is a function running concurrently with other goroutines"
docker compose exec dev go test -v ./labs/table-tests
docker compose exec dev go test -v -run 'TestChunkWords/extra_spaces' ./labs/table-tests
```

## What it shows

Real runs on 2026-11-25 (Go 1.27.4):

```
1: a goroutine is a
2: function running concurrently with
3: other goroutines
```

Each case is a subtest with its own name. `t.Run` replaces the spaces in a name with
underscores:

```
=== RUN   TestChunkWords
=== RUN   TestChunkWords/empty_text
=== RUN   TestChunkWords/exact_multiple
=== RUN   TestChunkWords/remainder_in_last_chunk
=== RUN   TestChunkWords/size_larger_than_text
=== RUN   TestChunkWords/extra_spaces
--- PASS: TestChunkWords (0.00s)
    --- PASS: TestChunkWords/empty_text (0.00s)
    --- PASS: TestChunkWords/exact_multiple (0.00s)
    --- PASS: TestChunkWords/remainder_in_last_chunk (0.00s)
    --- PASS: TestChunkWords/size_larger_than_text (0.00s)
    --- PASS: TestChunkWords/extra_spaces (0.00s)
=== RUN   TestChunkWordsRejectsSizeBelowOne
--- PASS: TestChunkWordsRejectsSizeBelowOne (0.00s)
PASS
```

## When a case fails

With a bug, `strings.Split(text, " ")` instead of `strings.Fields(text)`, two cases fail.
Each subtest reports on its own, by name, and the others still run:

```
--- FAIL: TestChunkWords (0.00s)
    --- FAIL: TestChunkWords/empty_text (0.00s)
        main_test.go:28: chunkWords("", 3) = [""], want []
    --- FAIL: TestChunkWords/extra_spaces (0.00s)
        main_test.go:28: chunkWords("  a   b\n c ", 2) = [" " "a " " b\n" "c "], want ["a b" "c"]
```

`-run` takes the test name and the subtest name, separated by `/`, and runs one case:

```
=== RUN   TestChunkWords
=== RUN   TestChunkWords/extra_spaces
    main_test.go:28: chunkWords("  a   b\n c ", 2) = [" " "a " " b\n" "c "], want ["a b" "c"]
--- FAIL: TestChunkWords (0.00s)
    --- FAIL: TestChunkWords/extra_spaces (0.00s)
```

## The classic mistake

**A table without subtests, and `t.Fatalf` in the loop.** The same bug, but the loop
checks the cases directly. `t.Fatalf` stops the whole test at the first failure: the
second broken case never runs, and the output does not say which case failed:

```
--- FAIL: TestChunkWords (0.00s)
    main_test.go:27: chunkWords("", 3) = [""], want []
```

Inside `t.Run`, `t.Fatal` stops only that subtest.
