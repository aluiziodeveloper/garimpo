# labs/load-all

The first ingestion read the whole corpus, the Go standard library, into a slice before it
looked at any file. To find one file of 3.5 KB, it held 28 MB (INC-06). This lab looks for
one file in two ways: `all` loads every file and then searches the slice; `each` reads one
file at a time, hands it to a callback and stops when the callback says so.

```bash
docker compose run --rm lab go run ./labs/load-all
docker compose run --rm lab go run ./labs/load-all -mode each
docker compose run --rm lab go run ./labs/load-all -file strings/nope.go
```

The corpus is every `.go` file under `/usr/local/go/src`, without tests, `testdata`,
`vendor` and the top-level `cmd`.

## What it showed

On 2026-11-25, `lab` service (2 CPUs), Go 1.27.4, three runs of the built binary, all
equal:

```
mode=all read=2972 files found=strings/builder.go (3589 bytes) heap=27.9 MB allocated=57.9 MB
mode=each read=2610 files found=strings/builder.go (3589 bytes) heap=0.1 MB allocated=42.2 MB
```

- `heap` is what is still in use after a collection, at the moment the file was found.
  `all` holds the whole corpus; `each` holds only the file it found.
- `allocated` is everything the walk allocated, freed or not. It is about twice the corpus
  in both modes: `fs.ReadFile` allocates the bytes and `string(b)` copies them. `each`
  allocated less because it stopped at file 2,610 of 2,972.
- The callback works, but stopping early takes a sentinel error (`errFound`) that the
  caller has to filter out. Module 06 replaces the callback with an `iter.Seq`.
