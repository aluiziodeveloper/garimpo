# Lab: select and deadlines

Three replicas answer the same question after 80, 30 and 50 ms. `first` asks all of them,
each in its own goroutine, and a `select` waits for whichever comes first: an answer on
the `answers` channel or the deadline from `time.After`. Since Go 1.23 the garbage
collector recovers an unstopped timer, so `time.After` in a `select` is fine.

## Run it

```bash
docker compose exec dev go run ./labs/select -timeout 100ms
docker compose exec dev go run ./labs/select -timeout 10ms
docker compose exec dev go test -race ./labs/select
```

## What it shows

Real runs on 2026-11-25 (Go 1.27.4). The program waits 200 ms after the answer, so every
replica has finished, and then counts the goroutines.

With a 100 ms deadline, the fastest replica wins:

```
30ms  replica-b
goroutines running: 1 (main included)
```

With 10 ms, no replica makes it, and the deadline case of the `select` runs:

```
10ms  no replica answered in time
goroutines running: 1 (main included)
```

## The classic mistake

**A channel with no room for the losers.** With `make(chan string)`, only one value is
ever received. Every other replica blocks on its send forever: a goroutine leak. Nothing
crashes; the goroutines just stay:

```
30ms  replica-b
goroutines running: 3 (main included)
```

```
10ms  no replica answered in time
goroutines running: 4 (main included)
```

`TestFirstLeavesNoGoroutineBehind` catches it:

```
--- FAIL: TestFirstLeavesNoGoroutineBehind (1.01s)
    main_test.go:66: 2 goroutines still running a second later
```

The fix used here is a buffer with room for every answer, so each send completes even
when nobody receives.
