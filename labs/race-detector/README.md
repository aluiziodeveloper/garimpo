# labs/race-detector

Five small programs that share memory between goroutines. Two have a data race, one has a
logic error made only of atomics, and two are correct.

| Case | What it does | Without `-race` | With `-race` |
|---|---|---|---|
| `sleep` | a goroutine writes, `main` sleeps 10 ms and reads | `hello` in 10 of 10 runs | `WARNING: DATA RACE`, exit status 66 |
| `chan` | the same hand-off, closing a channel | `hello` | silent |
| `map` | two goroutines write to one map | `fatal error: concurrent map writes` in 10 of 10 runs | `WARNING: DATA RACE`, then the same fatal error |
| `flag` | 8 goroutines check an `atomic.Bool` before a 1 ms load | `load ran 2 times` in 10 of 10 runs | silent, `load ran 2 times` |
| `once` | the same load with `sync.Once` | `load ran 1 times` | silent |

Measured on 2026-11-25, `lab` service (2 CPUs), Go 1.27.4.

```bash
docker compose run --rm lab go run -race ./labs/race-detector -case sleep
docker compose run --rm lab go run -race ./labs/race-detector -case flag
```

## What the race detector sees

- **The code that runs.** `go test -race ./labs/race-detector` passes, and `main.go` has two
  data races: the tests never run `sleep` or `map`. In the words of the
  [race detector article](https://go.dev/doc/articles/race_detector): "The race detector only
  finds races that happen at runtime, so it can't find races in code paths that are not
  executed."
- **Order, not time.** In `sleep` the write comes first on every run, and it is still a race:
  a sleep orders nothing. The detector tracks happens-before, so it reports the race even
  when the two accesses are 10 ms apart.
- **Not logic.** In `flag` every access is atomic, so there is no data race to report. Two
  goroutines read `false` before either stores `true`, and the load runs twice.

The map check without `-race` is the runtime's own, from Go 1.6: "lightweight, best-effort
detection of concurrent misuse of maps" (<https://go.dev/doc/go1.6#runtime>).

## The cost

The whole test suite, same container, warm build cache: 1,550 to 1,605 ms with `go test`,
11,164 to 11,248 ms with `go test -race`, about 7 times slower. The article's range for a
typical program is "memory usage may increase by 5-10x and execution time by 2-20x". The
binary of this lab goes from 2,796,376 to 3,470,339 bytes.
