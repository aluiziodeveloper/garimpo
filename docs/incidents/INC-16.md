# INC-16: a hundred equal questions

- **Date:** 2026-11-27
- **Module:** 16, Query server
- **Reproduction:** a probe at the `main` of the end of module 15 (`aula-15-10` and PR #120),
  `dev-env` capture `16-01-probe_test.go`, kept out of the repository: the hybrid search of
  module 15 over the real vectors of the corpus (39,244) and the BM25 index of its 41,980
  chunks, with the real model (`all-minilm:22m`, 4 CPUs for Ollama); one question at a time,
  10 times, and then 100 equal questions at once; 8 virtual CPUs for the probe

## Summary

A server of searches gets the same question many times at once: a link shared in a chat, a
page that many open, a client that retries. The probe asked the hybrid search one question
100 times at once. All 100 answers were the same, and each one was computed: 100 calls to
the model to embed the same text, 100 vector searches and 100 keyword searches. Each
question took 25 times as long as alone.

No test failed. Nothing was wrong with any one search: the waste is in doing the same one
100 times.

## Impact

The probe, 2026-11-27 (`dev-env` capture `16-01-probe.txt`):

| Questions | All done in | Each, p50 | Each, p99 | Work |
|---|---|---|---|---|
| 1 at a time, 10 times | | 10.219 ms | max 11.674 ms | 1 call to the model per question |
| 100 equal, at once | 488 ms | 258 ms | 482 ms | 100 calls to the model, 100 vector searches, 100 keyword searches |

The model is the slow part, and it has 4 CPUs: 100 calls wait for each other there. A burst
of equal questions takes the model from every other question that comes meanwhile.

## Cause

1. **Every request runs its own search.** The use case, and the server of lesson 16-03,
   compute what the request asks, with no knowledge of the requests under way.
2. **The answer does not depend on who asks.** The same query with the same `k`, at the same
   moment, over the same snapshot, gives the same hits: the 99 others could have waited for
   the first one.

## Fix

1. **One search for the equal ones at once** (lesson 16-09, `feat(app): collapse duplicate
   searches`). `search.Collapsed` wraps the ranker with `singleflight`: a search with the
   query and `k` of one under way waits for it and gets its hits. It is not a cache: a search
   after the last one ended runs again.
2. **The shared search does not belong to the first caller.** It runs with
   `context.WithoutCancel` of the first caller's context and a deadline of its own, the one
   of a request (`-timeout`): a client that leaves does not cancel it for the others
   (`labs/singleflight` shows what `Group.Do` with the caller's context does: the first
   cancel fails all 100). Each caller waits for the hits or for its own context.
3. **On by default in `garimpo serve`**, with `-collapse=false` to measure without it.

The server on the corpus, 100 requests at once, two runs (`dev-env` captures
`16-10-serve.txt` and `16-10-serve-2.txt`):

| `-collapse` | 100 at once | All in | Each, p50 | Calls to the model |
|---|---|---|---|---|
| `false` | equal | 1.498 s, 1.547 s | 799 ms, 885 ms | 100 |
| `true` | equal | 13 ms, 14 ms | 12 ms, 13 ms | 1 |
| `false` | different | 1.399 s, 1.58 s | 711 ms, 709 ms | 100 |
| `true` | different | 1.398 s, 1.702 s | 797 ms, 905 ms | 100 |

Through the server, the 100 equal questions without the fix took 1.5 s, against 488 ms in
the probe, which called the search directly: the server also reads the texts, encodes the
JSON and serves 100 connections. That breakdown was not measured.

## Regression tests

- `TestEqualSearchesAtOnceRunOneSearch`: 100 equal searches at once, in a `synctest` bubble,
  run 1 search and take exactly its time. Before the fix, there was no `Collapsed`, and the
  server ran 100.
- `TestACallerThatGivesUpLeavesAlone`: the caller that started the search gives up, and the
  other 9 still get the hits, from 1 search.
- `TestTheSearchEndsAtItsOwnDeadline`: with every caller gone, the shared search stops at the
  deadline of `Collapse`.
- `TestOtherQueriesOrKsAreOtherSearches` and `TestCollapseIsNotACache`: only equal searches
  at once share.

## Where Garimpo meets it

- **Equal work at once is done once.** `singleflight` shares a call, not a result: there is
  nothing to invalidate.
- **A shared call needs a context of its own.** The context of the first caller would make
  every other caller depend on it. `context.WithoutCancel` keeps its values, drops its
  cancel, and a deadline puts back a bound.
- **What is shared is read only.** The callers get the same slice of hits.
- **The time of a request is the time of the slowest thing it waits for.** Here, a model
  with 4 CPUs: the server sends it as few calls as it can.
