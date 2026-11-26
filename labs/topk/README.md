# labs/topk

A search keeps the k best of one score per vector: 10 of 41,980 for Garimpo's corpus. The
lab picks them in three ways and counts the comparisons each one makes:

- **sort**: sort every score and keep the first k, n·log₂(n) comparisons whatever k is;
- **heap**: keep a heap of the k best with the worst at the root (`container/heap`); each
  score is compared with the root, and one that is better takes its place and sinks in
  log₂(k) more;
- **insertion**: keep the k best in a sorted slice; each score is compared with the last,
  and one that is better walks up to k places to find its own.

The scores come in random order, as the scores of a search do, and in ascending order, the
worst case of the heap and the slice: every score is better than the ones kept. Go 1.27.4,
2026-11-26.

| Test | What it holds |
|---|---|
| `TestEverySelectorAgreesWithSort` | the three give the same k best, in order, for k from 1 to twice the scores, in both orders |
| `TestAscendingOrderIsTheWorstCaseOfInsertion` | 10,000 ascending scores, k=1000: the slice makes more than 10 times the comparisons of the heap |

```bash
docker compose exec dev go test -race -count=1 -v ./labs/topk
docker compose exec dev go run ./labs/topk
```

```
41980 scores, random order: comparisons
k      sort       heap       insertion 
10     695607     42380      42501     
100    695607     49075      74310     
1000   695607     121526     2179010   
41980 scores, ascending order: comparisons
k      sort       heap       insertion 
10     41991      254655     461715    
100    41991      523245     4234830   
1000   41991      792764     41520480  
```

In random order, the heap and the slice make little more than one comparison per score
for k=10: past the first scores, few are better than the worst of the 10 kept. The sort
makes about 17 per score. For k=1000, the slice pays for its walks, and makes 18 times the
comparisons of the heap.

In ascending order, every score enters the k kept, and the heap pays log₂(k) for each and
the slice up to k. The sort makes one comparison per score: `slices.SortFunc` (pdqsort)
finds the scores already in order, reversed, and only reverses them. Search scores never
come in order; the case is here to show the bound, not a real search.

## Time

On one virtual CPU (`BENCH_CPUSET=2`), 6 runs, `benchstat`:

```bash
BENCH_CPUSET=2 docker compose run --rm bench go test -run '^$' -bench Select -benchmem -count 6 ./labs/topk
```

| Order | k | sort | heap | insertion |
|---|---|---|---|---|
| random | 10 | 3.616 ms | 26.01 µs | 32.79 µs |
| random | 100 | 3.699 ms | 58.89 µs | 72.11 µs |
| random | 1000 | 3.685 ms | 445.0 µs | 1.660 ms |
| ascending | 10 | 125.8 µs | 605.3 µs | 545.7 µs |
| ascending | 100 | 128.8 µs | 1.909 ms | 3.493 ms |
| ascending | 1000 | 130.9 µs | 3.115 ms | 29.55 ms |

For the 10 best of a search, the heap is 139 times faster than the sort, which also
copies the 41,980 scores (168 KiB per call).

The heap allocates 2·k + 3 times per call (23 for k=10, 2,003 for k=1000), and the slice
once: `heap.Push` takes an `any` and `heap.Pop` returns one, and a `float32` in an `any`
goes to the heap memory. The Garimpo version (lesson 12-06) fills the slice first and
calls `heap.Init` and `heap.Fix`, which take no value.
