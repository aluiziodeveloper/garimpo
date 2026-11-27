# Garimpo

A local semantic search and RAG engine written in Go: it reads a corpus, cuts it into chunks,
turns them into vectors with a local model, indexes them and answers questions with citations
you can check. Everything runs on your machine.

## Requirements

- Docker with Compose. The Go toolchain runs inside the `dev` container; nothing else is installed on your machine.
- Git on the host.

## Getting started

```bash
docker compose up -d --build dev
docker compose exec dev go test ./...
docker compose exec dev go build -o bin/garimpo ./cmd/garimpo
docker compose exec dev ./bin/garimpo version
```

The container runs as a user with UID and GID 1000, so the files it creates in the project
belong to you. If your user has other IDs (`id -u`, `id -g`), build with them:

```bash
UID=$(id -u) GID=$(id -g) docker compose build dev
```

## Local models (Ollama)

Garimpo talks to a local [Ollama](https://ollama.com) (`ollama/ollama:0.40.16`), the `ollama`
service in `compose.yaml`. It publishes no ports: the `dev` container reaches it at
`http://ollama:11434` (`GARIMPO_OLLAMA_URL`). The models live in the `ollama-models` volume,
so they are downloaded once:

```bash
docker compose up -d --wait ollama
docker compose exec ollama ollama pull all-minilm:22m   # embeddings, 384 dimensions
docker compose exec ollama ollama pull qwen2.5:1.5b     # generation
```

Ollama gets `OLLAMA_CPUS` CPUs (default 4). It does not read that limit from the cgroup: it
sizes its thread pool from the host CPUs, so every request sends `num_thread` with the same
number (`GARIMPO_OLLAMA_NUM_THREAD`, set from the same variable). Keep it fixed: a request
with a different `num_thread` reloads the model.

```bash
docker compose exec dev sh -c 'curl -s $GARIMPO_OLLAMA_URL/api/generate -d "{\"model\":\"qwen2.5:1.5b\",\"prompt\":\"Explain what a goroutine is in one sentence.\",\"stream\":false,\"options\":{\"temperature\":0,\"num_predict\":32,\"num_thread\":$GARIMPO_OLLAMA_NUM_THREAD}}"'
```

To give Ollama more CPUs, change the variable for both services: `OLLAMA_CPUS=8 docker compose up -d`.

`garimpo ingest` runs the ingestion over a corpus with `all-minilm:22m` on that Ollama
(`internal/adapters/ollama`), and the adapter's tests run the `Embedder` contract against it
when `GARIMPO_OLLAMA_URL` is set, as it is in the `dev` container (the CI skips them):

```bash
docker compose exec dev go run ./cmd/garimpo ingest -corpus /usr/local/go/src/errors
docker compose exec dev go test -run Ollama -v ./internal/adapters/ollama
```

`-batch 8` gathers up to 8 chunks of concurrent workers into one request (`ollama.Batcher`):
a batch goes out when it is full or `-batch-wait` (10 ms) after its first chunk.

`BenchmarkBatch` measures it on that Ollama: 256 chunks of `net/http`, 32 workers, one chunk
per call, on the `bench` service (Ollama with its default 4 CPUs), 6 runs each:

| `-batch` | Time for 256 chunks | Chunks/s | vs one by one |
|---|---|---|---|
| 1 | 13.86 s ± 2% | 18.46 | |
| 8 | 13.45 s ± 1% | 19.03 | +3.06% (p=0.002) |
| 32 | 13.34 s ± 2% | 19.19 | +3.93% (p=0.004) |

A batch saves the cost of each request, and here the model's CPU is almost all of the cost:
the gain is 3 to 4%. So `-batch` stays 1 by default. Batching pays when each request costs
much more than the work in it (a remote model, a GPU idle between requests); it does not pay
when the model is already busy with the requests it has.

```bash
docker compose run --rm bench go test -run '^$' -bench Batch -benchtime 1x -count 6 ./internal/adapters/ollama
```

`-adaptive` puts an adaptive limiter (`internal/app/limiter`) in front of the model: the calls
at the same time start at 1 and follow their latency, up to `-workers` (RN04). On the CPU,
more calls only make a queue in front of the model, and the runs on `encoding/json` ended
with a limit of 1 or 2:

```bash
docker compose exec dev go run ./cmd/garimpo ingest -corpus /usr/local/go/src/encoding/json -workers 16 -adaptive
```

```
completed documents=48 chunks=1038 embedded=1038 failed=0 pending=0 in 32.162s (adaptive limit 2 of 16)
```

`BenchmarkAdaptive` measures it on that Ollama: the ingestion of `encoding/json` (1,038 chunks),
on the `bench` service (Ollama with its default 4 CPUs), 6 runs each; the latency is of one
call to the model:

| Workers | Chunks/s | p50 | p99 | Limit at the end |
|---|---|---|---|---|
| 1 | 32.52 ± 2% | 30.3 ms | 47.8 ms | |
| 16 | 33.34 ± 1% | 479.2 ms | 592.8 ms | |
| 16, `-adaptive` | 32.59 ± 1% (-2.25%, p=0.002) | 38.3 ms (-92%) | 95.5 ms (-84%) | 1 or 2 |

Sixteen workers keep the model as busy as one does: the other fifteen calls only wait in
its queue, and each call takes sixteen times as long (INC-09). The limiter gives up 2.25% of
the throughput and takes the latency of a call back near the one of a single worker. The
queue it removes is where a search query, from module 16 on, would wait behind the
ingestion. `-adaptive` is off by default until then.

```bash
docker compose run --rm bench go test -run '^$' -bench Adaptive -benchtime 1x -count 6 ./cmd/garimpo
```

## Recall on the corpus

`-vectors` writes the vector of every chunk the ingestion embeds to a file
(`internal/adapters/vectorfile`). The HNSW index (`internal/adapters/hnsw`) is measured on
it against the exact index (`internal/adapters/flatindex`): the share of the 10 best of each
query that the graph finds, recall@10. Without `GARIMPO_VECTORS`, the test skips, as in CI:

```bash
docker compose exec dev go run ./cmd/garimpo ingest -vectors /src/.bs/corpus.vec
BENCH_CPUSET=2,4,6,8,10,12,14,16 docker compose run --rm -e GARIMPO_VECTORS=/src/.bs/corpus.vec bench go test -count=1 -run TestRecallOnTheCorpus -v ./internal/adapters/hnsw
```

The whole corpus took 22m48s and gave 39,244 vectors (2,736 chunks are over the model's
context). The last 500 are the queries, outside the index; the graph holds the other 38,744,
built with `M` 16 and `EfConstruction` 200, on 8 virtual CPUs, 2026-11-26:

| `EfSearch` | recall@10 | Time per search |
|---|---|---|
| 10 | 0.891 | 168 µs |
| 32 | 0.960 | 268 µs |
| 64 (default) | 0.982 | 388 µs |
| 128 | 0.995 | 611 µs |
| 256 | 0.998 | 1.01 ms |

The exact index took 5.922 ms per search. The default `EfSearch` reaches the 0.95 of RNF04
at 15 times the speed of the exact index; the test fails below it.

## Building the graph concurrently

`Add` calls run at the same time: the index lock is taken alone only to put a new node in
the list, and each node has a lock of its own for its links. `BenchmarkBuild` measures the
build of 2,000 random vectors of 384 dimensions, one lock for the whole graph (the code of
`aula-13-05`) against a lock per node, 6 runs each, interleaved, on 8 virtual CPUs,
2026-11-26:

| Goroutines | One lock | A lock per node | |
|---|---|---|---|
| 1 | 3.666 s ± 3% | 3.964 s ± 3% | +8.13% (p=0.002) |
| 2 | 3.671 s ± 4% | 2.647 s ± 6% | -27.89% (p=0.002) |
| 4 | 3.578 s ± 5% | 2.045 s ± 16% | -42.84% (p=0.002) |
| 8 | 3.632 s ± 4% | 2.215 s ± 25% | -39.01% (p=0.002) |

With one lock, more goroutines change nothing: they take turns. With a lock per node, one
goroutine pays for the locks (8% slower, with 2.1 times the allocations: a walk copies the links
it reads), and eight goroutines are slower than four.

```bash
BENCH_CPUSET=2,4,6,8,10,12,14,16 docker compose run --rm bench go test -run '^$' -bench Build -benchmem -count 6 ./internal/adapters/hnsw
```

Built by 8 goroutines, the graph lost links: a node set its own list after other inserts,
which had reached it from the level above, had already linked to it there, and the list
it wrote erased theirs (INC-13). The race detector saw nothing, since every write was under
the node's lock. `setLinks` now merges the links and prunes them. The recall on the corpus,
2026-11-27:

| `EfSearch` | 8 goroutines, overwriting | 8 goroutines, merging | 1 goroutine |
|---|---|---|---|
| 10 | 0.849 | 0.855 | 0.891 |
| 32 | 0.923 | 0.951 | 0.960 |
| 64 (default) | 0.946 | 0.979 | 0.982 |
| 128 | 0.957 | 0.993 | 0.995 |
| 256 | 0.959 | 0.997 | 0.998 |

A wider search did not bring the lost links back: overwriting, the recall stopped at 0.959.
In 5 interleaved runs with 8 goroutines, overwriting gave 0.904 to 0.949 at the default
`EfSearch` (the test failed in all 5), and merging 0.969 to 0.978.

Eight goroutines were still slower than four. The CPU profile of the build by 8 goroutines
put 47% of the time in `sync/atomic.(*Int32).Add`, inside the `RLock` that looked each
node up: a reader that never waits still adds to the counter of readers, and all of them
add to the same one. The mutex profile did not show it, since it counts only waiting. The
list of nodes is now published in an `atomic.Pointer` by `Add`, and a walk reads it without
a lock. The same benchmark, 6 runs each, interleaved, on 8 virtual CPUs, 2026-11-27:

| Goroutines | `RLock` for each node | `atomic.Pointer` | |
|---|---|---|---|
| 1 | 4.167 s ± 7% | 3.940 s ± 12% | ~ (p=0.310) |
| 2 | 2.719 s ± 8% | 2.137 s ± 10% | -21.41% (p=0.002) |
| 4 | 2.089 s ± 13% | 1.216 s ± 16% | -41.81% (p=0.002) |
| 8 | 2.254 s ± 5% | 0.810 s ± 3% | -64.07% (p=0.002) |

Eight goroutines now build 4.9 times as fast as one, and the CPU profile puts 78% of the
time in the dot product. On the corpus, in 5 interleaved runs with 8 goroutines, the graph
took 10.6 to 11.3 s to build (21.1 to 29.4 s before), with a recall@10 of 0.973 to 0.982 at
the default `EfSearch`: a graph built concurrently differs from run to run, so its recall is
a range.

```bash
BENCH_CPUSET=2,4,6,8,10,12,14,16 docker compose run --rm bench sh -c 'go test -c -o /tmp/hnsw.test ./internal/adapters/hnsw &&
    cd /tmp && ./hnsw.test -test.run "^$" -test.bench "Build/workers=8" -test.cpuprofile cpu.out -test.mutexprofile mutex.out &&
    go tool pprof -top -nodecount 10 hnsw.test cpu.out'
```

## Searching while ingesting

Garimpo will search the index while the ingestion adds to it. The exact index of module
12, `flatindex.Index`, guards its vectors with a `sync.RWMutex`: a search holds it shared
for the whole scan, and an `Add` waits for every scan under way (INC-14).
`flatindex.Published` takes no lock to search: every `Add` publishes a new version of the
index, an `indexing.IndexSnapshot` that never changes, in an `atomic.Pointer`, and a search
reads the version it loaded. The two, on the real vectors of the corpus (38,744 in the
index), 4 goroutines searching for 5 s, 8 virtual CPUs, 2026-11-27:

| Index | Adds | Search p50 | Search p99 | Adds in 5 s | Add p50 | Add p99 |
|---|---|---|---|---|---|---|
| `Index` | none | 6.379 ms | 9.609 ms | | | |
| `Index` | one every 35 ms | 6.442 ms | 9.811 ms | 128 | 3.388 ms | 7.749 ms |
| `Published` | none | 6.645 ms | 9.737 ms | | | |
| `Published` | one every 35 ms | 6.842 ms | 10.547 ms | 142 | 24 µs | 47 µs |

At the pace of an ingestion (39,244 vectors in 22m48s), an `Add` of `Published` takes
microseconds instead of the scan it waited for, and the searches do not change beyond the
noise. With no pause between `Add` calls, `Index` took 749 in 5 s, and `Published` all
200,000 the test offers: its searches slowed down because the index grew to 238,744
vectors, not because they waited.

```bash
BENCH_CPUSET=2,4,6,8,10,12,14,16 docker compose run --rm bench sh -c 'GARIMPO_VECTORS=/src/.bs/corpus.vec go test -run TestSearchDuringIngestOnTheCorpus -v ./internal/adapters/flatindex'
```

A search sees every `Add` that returned before it started
(`TestASearchSeesEveryAddThatReturned`): the consistency is eventual only for the `Add`
under way. How long it stays under way is the staleness, from the start of an `Add` to the
moment another goroutine sees the vector, measured the same way, 100 vectors, one every 35
ms:

| Index | Staleness p50 | Staleness max | Add p50 | Add max |
|---|---|---|---|---|
| `Index` | 4.783 ms | 8 ms | 4.633 ms | 7.923 ms |
| `Published` | 25 µs | 63 µs | 25 µs | 63 µs |

In both, a vector is seen as soon as its `Add` returns; what `Published` takes away is the
wait for the scans.

```bash
BENCH_CPUSET=2,4,6,8,10,12,14,16 docker compose run --rm bench sh -c 'GARIMPO_VECTORS=/src/.bs/corpus.vec go test -run TestStalenessOnTheCorpus -v ./internal/adapters/flatindex'
```

## Checks

The same commands run in CI on every pull request:

```bash
docker compose exec dev go tool -modfile=tools/go.mod golangci-lint fmt --diff   # format (gofumpt)
docker compose exec dev go tool -modfile=tools/go.mod golangci-lint run          # lint
docker compose exec dev go test -race -count=1 ./...                            # tests, race detector on
docker compose exec dev go build ./...
```

Development tools are pinned in `tools/go.mod`, apart from the project's `go.mod`, so their
dependencies never mix with Garimpo's.

## Workflow

`main` is protected by the ruleset in `.github/ruleset-main.json`: no direct pushes, changes
only through pull requests, squash merge only, and the checks `Lint and format`,
`Tests (race)`, `Build` and `PR title` must be green. The pull request title follows
[Conventional Commits](https://www.conventionalcommits.org/) and becomes the commit on `main`.

Every change goes from a branch to `main` with one script, run on the host (it needs `git`
and `gh`):

```bash
git switch -c feat/some-change
# ... edit, test ...
scripts/pr.sh "feat: add some change"            # commit, push, PR, checks, squash merge
scripts/pr.sh "feat: add some change" aula-02-01 # same, and tags the merge commit
```

Check a title before opening the pull request with `scripts/check-pr-title.sh "<title>"`.
Dependabot (`.github/dependabot.yml`) opens weekly updates for the Go modules and the
GitHub Actions, with titles that pass the same check.

## Architecture

Hexagonal, in layers. `cmd/garimpo` wires adapters into use cases; use cases depend on the
domain and on ports; the domain depends on nothing. The lint enforces the boundaries
(`depguard` and `forbidigo` in `.golangci.yml`): the domain cannot import `sync`, `net`, `os`
or an outer layer, nor call `time.Now`; use cases cannot import adapters or `net/http`;
labs cannot import `internal/`.

## Layout

| Path | Contents |
|---|---|
| `cmd/garimpo` | The binary: argument handling and wiring |
| `internal/domain/` | The domain: pure and sequential, no goroutines, no I/O, no global clock. `shared` is the shared kernel |
| `internal/app/` | Use cases: orchestrate the domain and declare the ports they consume |
| `internal/adapters/` | Adapters: implement the ports (Ollama, journal, indexes, HTTP) |
| `docs/adr/` | Architecture decision records, with an index |
| `docs/incidents/` | Postmortems, one per incident (`INC-01.md`, ...) |
| `docs/glossary.md` | Domain and technical glossary |
| `labs/` | Labs: one small program per Go feature, outside the architecture (each has its README) |
| `docker/dev.Dockerfile` | The development container (Go 1.27.4) |
| `compose.yaml` | Local services: `dev`, `ollama` and `lab` (profile `labs`, CPU quota `LAB_CPUS`) |
| `tools/go.mod` | Pinned development tools (`go tool -modfile=tools/go.mod …`) |
| `.golangci.yml` | Linters, formatter and the layer boundaries |
| `.github/workflows/ci.yml` | CI: format, lint, race tests, build |
| `.github/workflows/pr-title.yml` | CI: the pull request title follows Conventional Commits |
| `.github/ruleset-main.json` | Protection rules for `main` |
| `.github/dependabot.yml` | Weekly dependency and action updates |
| `scripts/` | `pr.sh` (branch to `main`) and `check-pr-title.sh` |

## License

[MIT](LICENSE)
