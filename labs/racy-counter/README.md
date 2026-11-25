# labs/racy-counter

Eight goroutines add 1 to one counter, 100 thousand times each. With a plain `int64`, the
total comes out short and different on every run; with `atomic.Int64`, it is exact (INC-05).

```bash
docker compose run --rm lab go run ./labs/racy-counter
docker compose run --rm lab go run ./labs/racy-counter -counter atomic
docker compose run --rm -e GOMAXPROCS=1 lab go run ./labs/racy-counter
docker compose run --rm lab go run -race ./labs/racy-counter
```

## What it showed

On 2026-11-25, `lab` service (2 CPUs), Go 1.27.4:

```
counter=plain workers=8 GOMAXPROCS=2: want 800000, got 800000 (0 lost)
counter=plain workers=8 GOMAXPROCS=2: want 800000, got 454320 (345680 lost)
counter=plain workers=8 GOMAXPROCS=2: want 800000, got 431025 (368975 lost)
counter=plain workers=8 GOMAXPROCS=1: want 800000, got 800000 (0 lost)
counter=plain workers=2 GOMAXPROCS=2: want 2000, got 2000 (0 lost)
counter=atomic workers=8 GOMAXPROCS=2: want 800000, got 800000 (0 lost)
```

- The plain counter was right on the first run and wrong on the next two. In 20 more runs of
  the built binary, none was right: the total went from 209,463 to 497,410.
- With `GOMAXPROCS=1`, or with 2 goroutines adding a thousand times, it was right: a test
  with little work, or a container with one CPU, does not show the bug.
- `go run -race` reported `Found 2 data race(s)` on `c++` (line 48) and exited with
  status 66.

`c++` is a read, an add and a write. When two goroutines read the same value, both write
value + 1, and one of the adds is lost. The Go memory model calls this a data race: "a write
to a memory location happening concurrently with another read or write to that same
location, unless all the accesses involved are atomic data accesses as provided by the
sync/atomic package" (<https://go.dev/ref/mem>).

The tests never run the plain counter with more than one goroutine: the CI runs them with
`-race`, and the race detector would fail them.
