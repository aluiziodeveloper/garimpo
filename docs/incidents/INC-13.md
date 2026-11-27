# INC-13: the recall that fell without a race

- **Date:** 2026-11-26
- **Module:** 13, Concurrent HNSW
- **Reproduction:** `TestRecallOnTheCorpus` at tag `aula-13-06`, on the real vectors of the
  corpus (`garimpo ingest -vectors`, 39,244 vectors of 384 dimensions): 38,744 in the
  graph, the last 500 as queries, `M` 16, `EfConstruction` 200, the graph built by 8
  goroutines on 8 virtual CPUs

## Summary

The HNSW index learned to take `Add` calls at the same time (lesson 13-06): a lock per
node for its links, and the index lock only for a moment. The contract passed, with `-race`,
and the build got 28 to 43% faster. On the corpus, built by 8 goroutines, the graph found
fewer of the 10 best of each query than the same code built by one: 0.946 against 0.982 at
the default `EfSearch`, below the 0.95 of RNF04. Under `-race` it fell to 0.912, and the
race detector reported nothing.

Nothing crashed and no test failed but the one that measures recall on real vectors. The
recall test of the CI, on 2,000 random vectors of 32 dimensions built by 4 goroutines, gave
0.997 to 0.999 in 5 runs of the same code: on random data the loss did not show.

## Impact

```
graph built by 8 goroutines in 22.156s
EfSearch=10  recall@10=0.849
EfSearch=32  recall@10=0.923
EfSearch=64  recall@10=0.946
EfSearch=128 recall@10=0.957
EfSearch=256 recall@10=0.959
recall@10 = 0.946 with the default EfSearch 64, want at least 0.95
```

With one goroutine, the same code gave 0.891, 0.960, 0.982, 0.995 and 0.998: the numbers of
the index with one lock, to the third decimal. In 5 interleaved runs with 8 goroutines, the
recall at the default `EfSearch` went from 0.904 to 0.949, and the test failed in all 5.

A wider search did not help: from `EfSearch` 128 to 256, the recall moved from 0.957 to
0.959. What the search could not find was not far away; the links to it were gone.

## Cause

1. **An `Add` puts its node in the list before it links it.** The node can be reached from
   that moment: an insert that walks the level above and meets it carries it down as a
   start of its search on the levels below.
2. **There, the other insert links to it.** On a level where the new node is still looking
   for its own links, the other one may pick it as a neighbor, and `link` appends its own
   ID to the new node's list, under the node's lock.
3. **Then the new node writes its own list.** `x.nodes[n].links[l] = links`, under the same
   lock, replaced the list and erased the link the other insert had put there. The other
   node still pointed to the new one; the new one no longer pointed back.
4. **No race, so no report.** Every read and write of a list was under its node's lock.
   The race detector finds accesses with no order between them; here the order existed,
   and it was the wrong one. The bug is in what the code does, not in how it synchronizes.

## Fix

1. **Merge, then prune** (lesson 13-10, `fix(index): merge links instead of overwriting`).
   `setLinks` adds the new links to the ones the node already has and, past the limit, keeps
   the ones the heuristic picks, like `link` does (`prune`). In 5 runs with 8 goroutines,
   the recall at the default `EfSearch` went to 0.969 to 0.978; with one goroutine it did
   not change.
2. **A lookup without a lock** (lesson 13-11). Found while profiling the fixed build: 47% of
   its CPU was in the atomic add of the `RLock` that looked each node up. The list of nodes
   is published in an `atomic.Pointer`, and the build by 8 goroutines went from 2.254 to
   0.810 s on 2,000 vectors (−64.07%). On the corpus: 10.6 to 11.3 s against 21.1 to 29.4,
   and a recall of 0.973 to 0.982 in 5 runs.

## Regression tests

- `TestSetLinksKeepsTheLinksOthersAdded` links node 2 to node 0, as another insert would,
  calls `setLinks(2, 0, [1])` and asks for both links. It failed before the fix with
  `node 2 links to [1] on level 0, want both 0 and 1`.
- `TestSetLinksKeepsToTheLimit` checks that a merged list keeps to 2·M on level 0.
- `TestRecallOnTheCorpus` builds the graph with `GOMAXPROCS` goroutines and fails below
  0.95. It needs the vectors of the corpus (`GARIMPO_VECTORS`) and skips in the CI.
- Not a regression test: the concurrent case of `indextest.ApproximateContract`. It judges
  the recall on every vector (600 of the best, PR #98) and no longer on 30: with 30, it
  failed 3 times in 300 runs on the code with the bug, and once in 100 on the fixed code.
  With 600 it failed none of 300 runs on the code with the bug: it no longer fails by
  chance, and it does not catch INC-13 either. The two `setLinks` tests and the corpus test
  do.

## Where Garimpo meets it

- **A metric of the domain caught what the tools did not.** The contract, `-race` and the
  CI were green. Only recall against the exact index, on the vectors Garimpo will really
  search, showed the loss. An index of Garimpo is measured on real vectors before it is
  trusted (RNF04).
- **Random data hides it.** The CI keeps its fast test on random vectors, and the corpus
  test is run by hand at every change of the graph, with the command in the README.
- **A concurrent build is not reproducible.** The graph changes from run to run; a recall
  is given as the range of several runs, not as one number.
