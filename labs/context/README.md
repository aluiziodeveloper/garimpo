# Lab: context, cancellation and deadlines

`embedAll` embeds five texts, one at a time. Each call stands in for a request to the model
and takes 40 ms. The whole batch runs under `context.WithTimeout`. Each call waits in a
`select` for the work or for `ctx.Done()`, so the batch stops as soon as the deadline
passes, and `ctx.Err()` says why.

## Run it

```bash
docker compose exec dev go run ./labs/context -timeout 1s
docker compose exec dev go run ./labs/context -timeout 100ms
docker compose exec dev go test -race ./labs/context
```

## What it shows

Real runs on 2026-11-25 (Go 1.27.4). The times change a little from run to run.

With a 1 s deadline, the five texts fit:

```
embedded 5 of 5 texts in 202ms
```

With 100 ms, the third call is cut short by the deadline:

```
embedded 2 of 5 texts in 101ms, then: context deadline exceeded
```

The tests use `t.Context()` (Go 1.24), a context that ends when the test ends.

## The classic mistakes

**Work that ignores the context.** With `time.Sleep(d)` in `embed` instead of the
`select`, the context ends and nobody looks. The batch runs to the end, past the deadline,
and returns no error:

```
embedded 5 of 5 texts in 202ms
```

```
--- FAIL: TestEmbedAllStopsAtTheDeadline (0.25s)
    main_test.go:31: err = <nil>, want context.DeadlineExceeded
--- FAIL: TestEmbedAllStopsWhenCanceled (0.25s)
    main_test.go:48: err = <nil>, want context.Canceled
```

A context only cancels code that checks it.

**Discarding `cancel`.** `ctx, _ := context.WithTimeout(...)` keeps the context's
resources until the deadline. `go vet` (and the `govet` linter in CI) catches it:

```
labs/context/main.go:28:7: the cancel function returned by context.WithTimeout should be called, not discarded, to avoid a context leak
```
