# labs/synctest

`testing/synctest` (Go 1.25) runs a test in a bubble with a fake clock: time only moves
when every goroutine of the bubble is durably blocked, and then it jumps to the next timer.
The lab retries a flaky operation with exponential backoff (250 ms, 500 ms, 1 s...) and tests
the same waits on the real clock and on the fake one. Go 1.27.4, 2026-11-25.

| Test | What it holds |
|---|---|
| `TestRetryOnTheRealClock` | three failures, then success: waits 1.75 s for real |
| `TestRetryOnTheFakeClock` | the same in a bubble: exactly 1.75 s on the bubble's clock, no time on the wall |
| `TestRetryGivesUpAfterTheLastAttempt` | three attempts, no wait after the last one: exactly 750 ms |
| `TestRetryStopsWhenTheContextEnds` | a 600 ms timeout ends the wait in the middle: exactly 600 ms |
| `TestWaitShowsWhereTheGoroutineIs` | `synctest.Wait` returns when the retry is blocked on its timer: 1 call, then 2 after 250 ms |

```bash
docker compose exec dev go test -race -count=1 -v ./labs/synctest
```

```
--- PASS: TestRetryOnTheRealClock (1.75s)
--- PASS: TestRetryOnTheFakeClock (0.00s)
--- PASS: TestRetryGivesUpAfterTheLastAttempt (0.00s)
--- PASS: TestRetryStopsWhenTheContextEnds (0.00s)
--- PASS: TestWaitShowsWhereTheGoroutineIs (0.00s)
```

On the real clock, the program shows the waits, with the jitter of a real timer:

```bash
docker compose exec dev go run ./labs/synctest -fail 3
```

```
call 1 at 0s: flaky
call 2 at 251ms: flaky
call 3 at 751ms: flaky
call 4 at 1.751s: <nil>
```

## Wait is not a memory barrier

`synctest.Wait` says the other goroutines are blocked; it does not say their writes are
visible. With a plain `int` for the count of calls, the race detector reports the read
after `Wait` against the `calls++` of the retry's goroutine (`WARNING: DATA RACE`,
`race detected during execution of test`). The test counts with an `atomic.Int64`.

## No real network in a bubble

A goroutine blocked on a socket is not durably blocked: something outside the bubble may
wake it. A test that calls a slow handler over `httptest.NewServer` (loopback) inside a
bubble never lets the clock move, and hangs until `go test` times out
(`panic: test timed out after 10s`). Over the in-memory network of
`httptest.NewTestServer` (Go 1.27), the same call takes 1 s on the bubble's clock and no
time on the wall.
