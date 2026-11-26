# INC-09: sixteen workers, one model

- **Date:** 2026-11-26
- **Module:** 09, Adaptive concurrency and Little's law
- **Reproduction:** the ingestion of `encoding/json` (48 documents, 1,038 chunks) at tag
  `aula-08-10`, on `all-minilm:22m` with Ollama on 4 CPUs (`OLLAMA_CPUS` 4), with 1, 2, 4,
  8 and 16 workers; a temporary test timed every call to the model

## Summary

The ingestion has a fixed number of workers, and every worker calls the model. More
workers looked like more speed: a stage that waits scales past the CPUs (lesson 07-05).
But the model is not a stage that waits. Ollama on the CPU serves about one call at a time,
and the other calls wait in a queue in front of it. Sixteen workers embedded the corpus as
fast as one, and each call took sixteen times as long.

Nothing failed and nothing was slower in total, so no check saw it. The cost is in the
latency: from module 16 on, a search query calls the same model, and it would wait behind
that queue.

## Impact

The same 1,038 chunks, one run per row:

| Workers | Chunks/s | p50 | p99 | Mean latency W | L = λ·W |
|---|---|---|---|---|---|
| 1 | 33.2 | 29 ms | 47 ms | 30 ms | 1.00 |
| 2 | 33.0 | 60 ms | 86 ms | 61 ms | 2.00 |
| 4 | 32.8 | 123 ms | 162 ms | 122 ms | 4.00 |
| 8 | 30.1 | 264 ms | 376 ms | 265 ms | 7.98 |
| 16 | 31.7 | 496 ms | 796 ms | 502 ms | 15.90 |

The throughput λ stays near 33 chunks/s; the latency grows with the workers. Little's law,
L = λ·W, gives the calls in the system: with 16 workers there are 15.90, and the model
serves one of them while the others queue. A call of 502 ms is about 30 ms of work and
470 ms of waiting.

## Cause

1. **A fixed limit for a capacity nobody knows.** `-workers` says how many calls may run at
   the same time, but the right number depends on the model, the CPUs and the length of the
   text, and it changes while the ingestion runs. Any fixed number is too low somewhere or
   too high somewhere else.
2. **A closed loop hides the queue.** Each worker waits for its call before it makes the
   next one, so the extra workers never raise the throughput or fail: they only stand in
   line. The total time is the same, and the latency is the only sign.

## Fix

1. **An adaptive limiter.** `internal/app/limiter` lets at most `Limit` calls go to the
   model at the same time and moves the limit with their latency (AIMD). It keeps a
   smoothed mean of the latency (weight 1/8) and, as the base, the lowest mean seen: the
   latency with no queue. The limit goes up by one after a limit's worth of fast calls made
   with the limit in use, and goes down by 0.9 when the mean passes 1.5 times the base. A
   call admitted before a cut does not cut again, and a call that fails is not a sample
   (`Discard`): a text over the context fails in milliseconds and would lower the base.
2. **RN04 in the ingestion.** `Config.Limiter` is optional: with it, each worker takes a
   permit before each call to the embedder, and no new call starts above the limit. When the
   limit goes down, the calls in flight finish, so for a moment there may be more calls
   than the new limit. `garimpo ingest -adaptive` builds the limiter with `Max` set to
   `-workers` and prints the limit at the end.

`BenchmarkAdaptive`, the same corpus on the `bench` service, 6 runs each:

| Workers | Chunks/s | p50 | p99 | Limit at the end |
|---|---|---|---|---|
| 1 | 32.52 ± 2% | 30.3 ms | 47.8 ms | |
| 16 | 33.34 ± 1% | 479.2 ms | 592.8 ms | |
| 16, `-adaptive` | 32.59 ± 1% (-2.25%, p=0.002) | 38.3 ms (-92%) | 95.5 ms (-84%) | 1 or 2 |

The limiter gives up 2.25% of the throughput and takes the latency back near the one of a
single worker. `-adaptive` stays off by default until the queries of module 16 need it.

## Regression tests

The limiter's tests run on the fake clock of a `synctest` bubble, against a service that
serves a fixed number of calls at a time, 30 ms each, with 16 workers making 50 calls each:

```
=== RUN   TestWithoutALimitEveryCallWaitsInTheQueue
    time_test.go:122: 24s, mean 475.5ms, 16 at the service at most
--- PASS: TestWithoutALimitEveryCallWaitsInTheQueue (0.00s)
=== RUN   TestTheLimitStaysNearTheCapacity
    time_test.go:138: 24s for 24s of service, mean 45.2625ms, limit at most 3, 3 at the service at most
--- PASS: TestTheLimitStaysNearTheCapacity (0.01s)
=== RUN   TestTheLimitGrowsWithTheCapacity
    time_test.go:160: 3.15s, mean 39.1875ms, limit at most 13, 13 at the service at most
--- PASS: TestTheLimitGrowsWithTheCapacity (0.01s)
```

`TestCallsAdmittedBeforeACutDoNotCutAgain` keeps the guard: without it, eight slow calls
admitted together cut the limit eight times, down to 1, instead of once, to 4.
`TestTheLimiterBoundsTheCallsToTheEmbedder` checks RN04 in the ingestion: 8 workers, a
limit of 2, never more than 2 calls to the embedder at once.

## Where Garimpo meets it

A slow service has a capacity the caller cannot read, so the caller measures it: the
latency says when a queue starts. The limiter does it for the embedding model, and it
lives in the application, not in the Ollama adapter: it knows calls and latencies, not
HTTP. Its base is the lowest mean ever
seen, so a model that gets slower for good, such as one sharing its CPUs with another
load, looks like a queue that never drains: the limit goes down to `Min` and stays
(`TestASlowerServicePinsTheLimitAtMin`). That is safe, fewer calls and never more, and it
is the limit of this AIMD.
