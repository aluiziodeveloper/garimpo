# Glossary

Two glossaries in one file. **Domain** is Garimpo's ubiquitous language: the words the code,
the tests and the docs use for the problem. The technical glossary, **Go and runtime** and
**Infrastructure**, holds the terms the project relies on. Each entry says in which lesson it
first appears.

## Domain

The domain language starts in module 03, with the bounded contexts. Until then, Garimpo has
no domain code.

## Go and runtime

| Term | Meaning | First seen |
|---|---|---|
| **toolchain** | The Go compiler, linker and `go` command of one version. Garimpo pins `go1.27.4` in `go.mod` and runs it only in the `dev` container | 01-02 |
| **`go` directive** | The `go 1.27.4` line in `go.mod`: the language version of the module. It also turns on version-gated runtime behaviour, such as container-aware `GOMAXPROCS` from 1.25 | 01-02, INC-01 |
| **`tool` directive** | A `tool` line in a `go.mod` that pins a program run with `go tool`. Garimpo keeps them in `tools/go.mod` | 01-04 |
| **race detector** | The `-race` instrumentation that reports two goroutines touching the same memory without synchronization while one writes. It proves a race happened, not that the code is correct | 01-04 |
| **`GOMAXPROCS`** | How many OS threads run Go code at the same time. Read it with `runtime.GOMAXPROCS(0)` | 01-07 |
| **`runtime.NumCPU`** | The CPUs the process may be scheduled on. A CPU quota does not change it | 01-07 |
| **container-aware `GOMAXPROCS`** | Since Go 1.25, the default `GOMAXPROCS` follows the container's CPU limit. Off with a Go older than 1.25, a `go` directive older than 1.25, `GODEBUG=containermaxprocs=0` or a `GOMAXPROCS` set by hand | 01-07 |
| **`GODEBUG`** | An environment variable that switches runtime behaviours on or off, one `name=value` per setting | 01-07 |
| **CPU quota** | A cgroup limit on CPU time per period (`cpus: 2` in Compose: 200 ms every 100 ms) | 01-06 |
| **throttling** | The kernel stopping a cgroup that used its CPU quota until the next period. Counted in `nr_throttled` of `/sys/fs/cgroup/cpu.stat` | 01-07 |

## Infrastructure

| Term | Meaning | First seen |
|---|---|---|
| **`num_thread`** | The Ollama request option that sets its CPU threads. Ollama does not read the container quota, so Garimpo always sends it | 01-06 |
| **ruleset** | GitHub's rules for a branch: here, pull requests only, four checks, squash merge | 01-05 |
| **squash merge** | Merging a pull request as one commit, with the pull request title as its message | 01-05 |
