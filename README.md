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
