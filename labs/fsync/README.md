# labs/fsync

A write returns when the bytes are in the kernel's page cache, not on the disk: if the
machine loses power, they are gone. `Sync` (the `fsync` system call) waits until the disk
has them. The lab appends records to a file, one write each, and calls `Sync` after every
n records, for several n, to show what each sync costs and how a batch spreads that cost.
Go 1.27.4, 2026-11-26.

| Test | What it holds |
|---|---|
| `TestEachRecordIsOneWrite` | 100 records of 64 bytes are 100 writes, one line each |
| `TestSyncsAfterEveryNRecords` | 100 records: a sync after every n and one more for the last ones (every 8: 13 syncs) |
| `TestZeroNeverSyncs` | `-every 0` never syncs |
| `TestTheFileHoldsEveryRecord` | on a real file, the 50 records are there after `Close` |

```bash
docker compose exec dev go test -race -count=1 -v ./labs/fsync
```

```
--- PASS: TestEachRecordIsOneWrite (0.00s)
--- PASS: TestSyncsAfterEveryNRecords (0.00s)
--- PASS: TestZeroNeverSyncs (0.00s)
--- PASS: TestTheFileHoldsEveryRecord (0.01s)
```

On the `dev` container, whose `/tmp` is an overlay on the ext4 disk of WSL 2:

```bash
docker compose exec dev go run ./labs/fsync
```

```
2000 records of 128 bytes in /tmp
every   records/s    syncs   sync p50   sync p99   total
1       929          2000    926µs      3.203ms    2.152s
8       6112         250     1.18ms     2.451ms    327ms
64      49365        32      995µs      1.42ms     41ms
512     286500       4       1.402ms    1.447ms    7ms
never   2099094      0       0s         0s         1ms
```

A sync costs about a millisecond whatever it carries: 128 bytes or 64 KB. A sync per
record caps the file at about a thousand records per second; without syncs, the same
records take 1 ms. Batching is what makes durability affordable: 64 records per sync are
53 times faster than one. In three runs, a sync took from 0.6 to 1.4 ms at the median.

The same program on `/dev/shm`, a `tmpfs` in memory, shows the other side: there is no
disk, and `Sync` returns at once.

```
2000 records of 128 bytes in /dev/shm
every   records/s    syncs   sync p50   sync p99   total
1       1393368      2000    0s         0s         1ms
```

The Garimpo's journal (module 10) gets about 33 events per second, one per chunk the
model embeds: at a millisecond each, a sync per event would take about 3% of the time of
the goroutine that writes them. The lessons ahead measure it on the ingestion.
