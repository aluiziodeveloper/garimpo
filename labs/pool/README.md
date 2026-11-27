# labs/pool

`sync.Pool` keeps temporary objects for reuse, the way the BM25 search of Garimpo reuses
the buffers of a search (module 15). Goroutines build reports of 4,000 numbers in buffers
of 64 KiB, in two ways: `plain` makes a new buffer for each report; `pooled` takes one from
a pool, empties it with `[:0]` and puts it back after.

Three rules come with a pool:

| Rule | Why |
|---|---|
| Empty what `Get` returns | a buffer comes back as the last caller left it; without `[:0]`, a report carries the one before it (`TestABufferFromThePoolStartsEmpty`) |
| Keep pointers in it, `*[]byte` and not `[]byte` | `Put` takes an `any`: a slice would be boxed, and the box allocated on every `Put` |
| Expect it to empty itself | each GC moves what a pool holds to a victim cache, and the next GC drops it (`TestTheGCEmptiesThePoolInTwoCycles`) |

The checksums of the reports are the same both ways, from one goroutine and from four, with
`-race` (`TestPooledBuildsTheSameReports`). Under `-race`, `Put` drops one object in four at
random, on purpose, so the test of the GC runs without it.

```bash
BENCH_CPUSET=2,4,6,8,10,12,14,16 docker compose run --rm bench go run ./labs/pool
BENCH_CPUSET=2,4,6,8,10,12,14,16 docker compose run --rm bench go test -run '^$' -bench Build -benchmem ./labs/pool
```

Measured in the `bench` service: 8 virtual CPUs of an i7-13700H, Go 1.27.4, 2026-11-27:

```text
20000 reports of 4,000 numbers by 8 goroutines, a buffer of 64 KiB each
kind          time  allocations          MiB    GCs  checksum
plain        299ms        20223       1250.1    396  e95d1cd8
pooled       124ms           44          0.5      0  e95d1cd8
buffers that Get finds again, of 100 put back, after GC cycles:
  0 GC: 100
  1 GC: 99
  2 GC: 0
```

Without the pool, 20,000 reports allocate 1,250 MiB and run 396 GC cycles; with it, 44
allocations, 0.5 MiB and no cycle, in 124 ms instead of 299. After one GC, `Get` still finds
99 of the 100 buffers, in the victim cache; after two, none.

One goroutine at a time, the benchmark (3 runs) shows less of a gain: 37.5 to 41.9 µs and
65,536 B in 1 allocation per report without the pool, 32.7 to 33.5 µs and 0 allocations
with it. Building the report is most of the cost; what the pool saves is the allocation and
the GC cycles it brings, which count more when 8 goroutines allocate at once.
