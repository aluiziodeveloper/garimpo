# labs/atomic-vs-mutex

Five ways to share a counter between goroutines, all exact (`TestEveryCounterIsExact`, with
`-race`), measured in the `bench` service: CPUs 2 and 4 of an i7-13700H, Go 1.27.4,
2026-11-25.

| Counter | How it adds |
|---|---|
| `atomic` | `atomic.Int64.Add`: one `LOCK XADDQ` instruction on amd64 |
| `mutex` | `sync.Mutex` around a plain `int64` |
| `cas` | `Load`, then `CompareAndSwap`, again until no one else wrote in between |
| `packed` | one `atomic.Int64` per goroutine, 8 bytes apart; `Load` adds them up |
| `padded` | the same, with each slot filling a 64-byte cache line |

```bash
docker compose run --rm bench sh -c 'go test -run "^$" -bench . -cpu 1,2 -count 10 ./labs/atomic-vs-mutex > /tmp/b.txt &&
    go tool -modfile=tools/go.mod benchstat -col /gomaxprocs /tmp/b.txt'
```

`-cpu 1,2` runs each benchmark with one goroutine and with two (`RunParallel` starts
GOMAXPROCS of them). The time is per add, counting the adds of both goroutines:

```
                    │              │                   2                    │
                    │    sec/op    │    sec/op      vs base                 │
Counter/kind=atomic   5.185n ±  1%   11.630n ±  9%  +124.32% (p=0.000 n=10)
Counter/kind=mutex    12.42n ± 12%    12.42n ± 12%         ~ (p=0.470 n=10)
Counter/kind=cas      6.823n ±  2%   15.760n ±  5%  +130.98% (p=0.000 n=10)
Counter/kind=packed   5.224n ±  2%   10.195n ±  3%   +95.18% (p=0.000 n=10)
Counter/kind=padded   5.197n ±  2%    2.663n ±  5%   -48.76% (p=0.000 n=10)
geomean               6.536n          9.082n         +38.95%
```

- **Sharing costs more than the instruction.** Alone, an atomic add took 5.2 ns. Two
  goroutines adding to the same counter took 11.6 ns per add: two CPUs did less, together,
  than one did alone. The cache line with the counter moves from one core to the other on
  every add.
- **The CAS loop is the worst under contention** (+131%): a failed swap is work thrown away.
- **The mutex barely changed.** It was the slowest alone and stayed at the same 12.4 ns.
- **Separate counters on the same line are still shared.** `packed` gives each goroutine its
  own slot, and it is almost as slow as one counter (+95%): this is false sharing. With one
  line per slot, `padded` took 2.7 ns per add, half the time of one goroutine.

The command adds 10 million times from each of 2 goroutines with every counter:

```
atomic   total=20000000 in 210ms
mutex    total=20000000 in 245ms
cas      total=20000000 in 320ms
packed   total=20000000 in 119ms
padded   total=20000000 in 52ms
```

Only `padded` scales, and its `Load` reads every slot: a sum, not a snapshot (module 05,
lesson 05-08).
