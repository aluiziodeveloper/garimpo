# labs/singleflight

`golang.org/x/sync/singleflight` runs one call per key at a time: the callers that come
while it runs wait for it and get its result. Garimpo uses it for equal searches at once
(module 16, INC-16). A fake search of 50 ms, which counts its calls, is asked the same
question many times at once, in three ways:

| Way | What it does |
|---|---|
| `plain` | one search per question |
| `do` | `Group.Do`, the search with the context of the question that came first |
| `detached` | `Group.DoChan`, the search with a context of its own (`context.WithoutCancel` of the first question's, plus a deadline), and each question waits with its own context |

```bash
BENCH_CPUSET=2,4,6,8,10,12,14,16 docker compose run --rm bench go run ./labs/singleflight
docker compose exec dev go test -race -v ./labs/singleflight
```

Measured in the `bench` service: 8 virtual CPUs of an i7-13700H, Go 1.27.4, 2026-11-27:

```text
100 equal questions at once, a search of 50ms
way       first question     searches  errors     took
plain     waits                   100       0     52ms
do        waits                     1       0     51ms
detached  waits                     1       0     51ms
plain     canceled at 10ms        100       1     53ms
do        canceled at 10ms          1     100     12ms
detached  canceled at 10ms          1       1     51ms
```

The fake search only waits, so 100 of them take as long as one: what `plain` costs is the
100 searches, which with a real model are 100 calls to it (`16-01`, the probe of INC-16).

The rules, each in a test (in `testing/synctest` bubbles, so the times are exact):

- One search for the questions that come while it runs (`TestDoRunsOneSearchForEqualQuestions`).
- With `Do`, the search runs with the context of the first question: when that one is
  canceled, every question fails, though nobody canceled them
  (`TestDoGivesTheCancelOfTheFirstQuestionToAll`).
- With a context of its own, a question leaves alone (`TestDetachedLetsAQuestionLeaveAlone`),
  and the search still ends at its own deadline when every question has left
  (`TestDetachedEndsTheSearchAtItsDeadline`).
- It is not a cache: a question after the search ended runs a new one
  (`TestAQuestionAfterTheSearchRunsANewOne`).
- The callers share the result itself: the same slice. Nobody may write to it
  (`TestEqualQuestionsShareTheSameSlice`).
