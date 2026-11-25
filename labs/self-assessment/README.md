# Lab: the Base firme self-assessment

The final exercise of module 02. It puts together the concurrency bases of the module:
goroutines and a `WaitGroup`, channels, `select`, `context` and errors as values.

## The exercise

Write `embedBatch`:

```go
type Embedder func(ctx context.Context, text string) (int, error)

func embedBatch(ctx context.Context, texts []string, workers int, embed Embedder) ([]int, error)
```

It calls `embed` for every text and must:

1. return the results in the order of `texts`;
2. run at most `workers` calls at the same time;
3. stop at the first error: cancel the calls still running and return that error, wrapped;
4. stop when `ctx` ends, and return why.

`main_test.go` is the statement: one test per rule. Copy it into your project, see it fail
(`undefined: embedBatch`), and write `main.go` until this passes:

```bash
docker compose exec dev go test -race -v ./labs/self-assessment
```

If all four pass with `-race` and you can explain each line, your base is firm. If one
rule stops you, the lesson that covers it is: order and the `WaitGroup`, 02-02; the pool
of workers fed by a channel, 02-03; `select` on two channels, 02-04; `context` and
cancellation, 02-05; wrapping with `%w`, 02-06.

## The solution in this repository

`main.go` starts `workers` goroutines that read indexes from an unbuffered `jobs`
channel. Each one writes only `results[i]`. The loop that feeds `jobs` also watches
`ctx.Done()`, so it stops feeding once the batch is canceled.

The first error cancels the batch with `context.WithCancelCause` (Go 1.20), and
`context.Cause` returns it at the end. Only the first cause counts: later calls fail with
`context canceled`, and those causes are ignored.

## Run it

```bash
docker compose exec dev go run ./labs/self-assessment
docker compose exec dev go run ./labs/self-assessment -workers 8
docker compose exec dev go run ./labs/self-assessment -workers 1
docker compose exec dev go run ./labs/self-assessment -fail select
```

Real runs on 2026-11-25 (Go 1.27.4), eight texts, 20 ms per call. The times change a
little from run to run:

```
[2 4 6 7 5 4 7 4] in 61ms (workers: 3)
[2 4 6 7 5 4 7 4] in 21ms (workers: 8)
[2 4 6 7 5 4 7 4] in 162ms (workers: 1)
```

```
self-assessment: embed "select": model unavailable (after 0s)
exit status 1
```

## The classic mistake

**Canceling without a cause.** With `context.WithCancel` and `ctx.Err()` at the end, the
batch still stops at the first error, but the error it returns is the cancellation, not
what went wrong:

```
self-assessment: context canceled (after 0s)
```

```
--- FAIL: TestEmbedBatchStopsAtTheFirstError (0.00s)
    main_test.go:73: err = context canceled, want one that wraps errModel
```
