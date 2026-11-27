# labs/weak-unique

Three tools of the runtime for memory that many parts of a program reach, the way Garimpo
reaches the paths and the texts of its chunks (module 14):

| Tool | What it does |
|---|---|
| `unique.Make` | interns a value: equal values get the same `unique.Handle`, the value is kept once, and two handles compare as two pointers |
| `weak.Make` | points to a value without keeping it alive: once the last ordinary pointer is gone, the GC frees it and `Value` returns `nil` |
| `runtime.AddCleanup` | runs a function after the GC frees a value: here it takes the dead entry out of a cache of weak pointers |

The tests check each one with `-race`: equal strings in different allocations get the
same handle (`TestEqualValuesGetTheSameHandle`), a weak pointer does not keep its value
(`TestAWeakPointerDoesNotKeepItsValue`), the cache keeps only the texts still held
(`TestTheCacheForgetsWhatNobodyHolds`), and the cleanup of an old text leaves the entry of
the text put after it under the same key (`TestACleanupLeavesANewerEntry`).

```bash
BENCH_CPUSET=2,4,6,8,10,12,14,16 docker compose run --rm bench go run ./labs/weak-unique
```

Measured in the `bench` service: 8 virtual CPUs of an i7-13700H, Go 1.27.4, 2026-11-27, one
of two runs (the other differs only in the times and in the heap before the GC, 9.0 MiB):

```text
unique: 1000000 paths, 1000 different
  []string                       61.1 MiB   1000 equal in 3.2ms
  []unique.Handle[string]         7.9 MiB   1000 equal in 253µs
weak: 10000 texts of 4096 bytes in the cache, 1000 still held
  heap after the puts, no GC      8.1 MiB
  heap after the GC               3.9 MiB   1000 texts alive, 1000 entries, the cleanups took 2ms
```

A million paths, a thousand of them different, each in its own allocation, take 61.1 MiB
as strings and 7.9 MiB as handles: 8 bytes each, plus one copy of each path. Counting the
paths equal to one of them takes 3.2 ms comparing strings, whose first 26 bytes are the
same, and 253 µs comparing handles.

The cache got 10,000 texts of 4 KiB, 39 MiB, and the program kept holding 1,000. The heap
had 8.1 MiB in use right after the puts, not 39: the GC already ran while they were made.
After a GC, only the 1,000 held remain (3.9 MiB), and the cleanups took the 9,000 dead
entries out of the map within 2 ms.

The cleanup must not reach the value it watches, or the value never becomes unreachable.
The function and its argument are what the runtime keeps for it; a closure over the value
makes the runtime panic at `AddCleanup`:

```text
panic: runtime.AddCleanup: cleanup function closes over ptr, cleanup will never run
```
