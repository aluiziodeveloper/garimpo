# labs/cow

A list that grows while goroutines scan it, the way Garimpo's searches read the index
while the ingestion adds to it (module 14), shared in three ways. The value appended is
always the length of the list before it, so every scan can check that it saw a whole
version, and every kind passes that check with `-race` (`TestEveryScanSeesAWholeVersion`).
Measured in the `bench` service: 8 virtual CPUs of an i7-13700H, Go 1.27.4, 2026-11-27.

| Kind | How it shares |
|---|---|
| `rwmutex` | one `sync.RWMutex`: a scan holds it shared, an append holds it alone |
| `copy` | an `atomic.Pointer` to a slice that never changes: an append copies it, adds the value and publishes the copy |
| `share` | an `atomic.Pointer` to a slice: an append writes just past its end, in the same array while it has room, and publishes a slice one longer |

`share` is safe because no scan reads past the length of the slice it loaded, so none reads
the place being written, and because only one goroutine appends. Two appends to the same
slice, with room in its array, write to the same place: the second overwrites what the
first published (`TestAppendToAnOldSliceOverwritesTheNewOne`).

```bash
BENCH_CPUSET=2,4,6,8,10,12,14,16 docker compose run --rm bench sh -c 'go run ./labs/cow && go run ./labs/cow -size 15000000'
```

Four goroutines scan as fast as they can, and one appends a value and sleeps 100 µs, for
3 s per kind. Here the sleep lasts about a millisecond: `share`, whose appends take a
microsecond, made 2,662 in 3 s, one every 1.13 ms. One real run; the tails change from run
to run:

| Values | Kind | Scans | Scan p50 | Scan max | Appends | Append p50 | Append max |
|---|---|---|---|---|---|---|---|
| 1,000,000 (3.8 MiB) | `rwmutex` | 47,473 | 234 µs | 4.286 ms | 2,405 | 143 µs | 1.622 ms |
| | `copy` | 45,560 | 225 µs | 2.079 ms | 2,563 | 452 µs | 4.831 ms |
| | `share` | 50,003 | 229 µs | 1.296 ms | 2,751 | 0 s | 1.341 ms |
| 15,000,000 (57.2 MiB) | `rwmutex` | 1,996 | 5.879 ms | 30.127 ms | 500 | 5.404 ms | 24.288 ms |
| | `copy` | 1,808 | 6.633 ms | 11.81 ms | 271 | 10.027 ms | 30.996 ms |
| | `share` | 2,279 | 5.193 ms | 8.651 ms | 2,662 | 1 µs | 11.621 ms |

With 57.2 MiB, about the size of the corpus index, an append under `rwmutex` waits for the
scans under way: 5.4 ms instead of microseconds, and 500 appends in 3 s against 2,662
under `share`. Under `copy`, the scans never wait, and each append copies the whole
list: 10 ms, 271 appends. Under `share`, an append takes a microsecond, except when the
array is full and `append` moves it to a larger one (the 11.6 ms at the top).

The time of one scan while another goroutine appends a value and sleeps 100 µs, 6 runs:

```bash
BENCH_CPUSET=2,4,6,8,10,12,14,16 docker compose run --rm bench sh -c 'go test -run "^$" -bench Scan -count 6 ./labs/cow > /tmp/b.txt &&
    go tool -modfile=tools/go.mod benchstat /tmp/b.txt'
```

| Kind | 1,000,000 values |
|---|---|
| `rwmutex` | 224.8 µs ± 4% |
| `copy` | 254.0 µs ± 4% |
| `share` | 217.5 µs ± 1% |

A scan under `copy` is 13% slower than under `rwmutex`: the copies take memory bandwidth
from the scans, and garbage to collect. `share` is the fastest of the three, by 3%.
