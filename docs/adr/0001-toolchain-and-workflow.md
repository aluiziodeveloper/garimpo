# ADR 0001: Toolchain, development environment and workflow

- **Status:** accepted
- **Date:** 2026-11-25
- **Module:** 01, Environment and workflow

## Context

Garimpo is a concurrent program measured on CPU: its lessons show timings, throttling and
race reports, so everyone must run the same Go, with the same tools, under the same CPU
limits. It also talks to a local model server (Ollama) that runs next to it. Every change
should reach `main` the same way, already checked.

## Decision

1. **Go 1.27.4, pinned.** `go.mod` says `go 1.27.4`, and the toolchain only runs inside the
   `dev` container (`docker/dev.Dockerfile`, `FROM golang:1.27.4`), as the host user
   (UID/GID 1000 by default), so generated files belong to you. Nothing else is installed on
   the host apart from Docker, Git and `gh`.
2. **Tools in their own module.** golangci-lint v2.15.0 is pinned in `tools/go.mod` with the
   `tool` directive and runs as `go tool -modfile=tools/go.mod golangci-lint`. gofumpt runs as
   its formatter (`fmt --diff`). The tools' dependencies never enter Garimpo's `go.mod`.
3. **CI on every pull request:** `Lint and format`, `Tests (race)` (`go test -race`),
   `Build` and `PR title` (Conventional Commits, `scripts/check-pr-title.sh`). Only the lint
   job uses the Go cache: the three jobs raced for the same cache key.
4. **`main` is protected** by `.github/ruleset-main.json`: changes only through pull requests,
   the four checks green, squash merge only, no force pushes, no deletion, no bypass.
   `scripts/pr.sh` takes a branch to `main` (commit, push, pull request, checks, squash merge,
   tag) and is the only way changes land. Dependabot proposes updates weekly, with titles
   that pass the same check.
5. **Ollama in Compose**, `ollama/ollama:0.40.16`, with no published ports and a CPU quota
   (`OLLAMA_CPUS`, 4 by default). The same number is sent as `num_thread` on every request
   (`GARIMPO_OLLAMA_NUM_THREAD`), and it does not change between requests.
6. **Container-aware GOMAXPROCS stays on.** The `go` directive stays at 1.25 or newer, and
   nothing sets `GODEBUG=containermaxprocs=0`. CPU-bound pools are sized with
   `runtime.GOMAXPROCS(0)`, never `runtime.NumCPU()` (INC-01).

## Measurements behind it

All taken on 2026-11-25 on a machine with 20 CPUs.

| Decision | Measurement |
|---|---|
| Lint cache only | CI on PR #1: with three jobs sharing the cache, 7.18 MiB saved and lint at 1m19s; with lint alone saving it, 239 MiB and 11s |
| `num_thread` | Ollama with `cpus: 4`, `qwen2.5:1.5b`, 23 tokens, model loaded: 1.74 to 1.90 s per generation without `num_thread` (log: `n_threads = 10`), 0.62 to 0.65 s with `num_thread=4`. Changing `num_thread` reloads the model: 1.0 to 1.6 s of `load_duration` |
| GOMAXPROCS | `labs/maxprocs`, 2-CPU quota, 4,000 jobs: `GOMAXPROCS: 2`, 2.27 to 2.47 s, no throttling; with `containermaxprocs=0`, `GOMAXPROCS: 20`, 3.47 to 4.12 s and 35 to 42 throttled periods (`docs/incidents/INC-01.md`) |

## Alternatives considered

- **Go installed on the host.** Versions drift between machines, and timings stop being
  comparable. Rejected.
- **golangci-lint in Garimpo's `go.mod`.** Its 211 requirements would join the
  project's version selection. Rejected for `tools/go.mod`.
- **A third-party action to check PR titles.** It hides the rule; a short script shows it and
  runs locally too. Rejected.
- **Letting Ollama pick its threads.** It sizes them from the host CPUs and ignores the
  container's limit: about 3× slower with a quota, and far worse with a cpuset. Rejected.
- **Merge commits or rebase merges.** One squash commit per pull request keeps `main` linear,
  with the pull request title as its message, and each lesson tag on one commit. Rejected.

## Consequences

- A fresh clone needs only Docker, Git and `gh`. The first `docker compose up` downloads the
  images, and the models are pulled once into the `ollama-models` volume.
- Nothing reaches `main` without the four checks. Fixing `main` also takes a pull request.
- The Ollama client must always send `num_thread`, and changing the CPU quota means changing
  one variable, `OLLAMA_CPUS`, for both services.
- Moving the `go` directive below 1.25 would silently bring INC-01 back.
