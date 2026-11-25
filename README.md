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

## Layout

| Path | Contents |
|---|---|
| `cmd/garimpo` | The binary: argument handling and wiring |
| `docs/adr/` | Architecture decision records, with an index |
| `docs/incidents/` | Postmortems, one per incident (`INC-01.md`, ...) |
| `docs/glossary.md` | Domain and technical glossary |
| `labs/` | Labs: one small program per Go feature, outside the architecture (each has its README) |
| `docker/dev.Dockerfile` | The development container (Go 1.27.4) |
| `compose.yaml` | Local services: `dev`, `ollama` and `lab` (profile `labs`, CPU quota `LAB_CPUS`) |
| `tools/go.mod` | Pinned development tools (`go tool -modfile=tools/go.mod …`) |
| `.golangci.yml` | Linters and formatter |
| `.github/workflows/ci.yml` | CI: format, lint, race tests, build |
| `.github/workflows/pr-title.yml` | CI: the pull request title follows Conventional Commits |
| `.github/ruleset-main.json` | Protection rules for `main` |
| `.github/dependabot.yml` | Weekly dependency and action updates |
| `scripts/` | `pr.sh` (branch to `main`) and `check-pr-title.sh` |

## License

[MIT](LICENSE)
