# Glossary

Two glossaries in one file. **Domain** is Garimpo's ubiquitous language: the words the code,
the tests and the docs use for the problem. The technical glossary, **Design**, **Go and
runtime** and **Infrastructure**, holds the terms the project relies on. Each entry says in which lesson it
first appears.

## Domain

The ubiquitous language, from module 03 on (ADR 0002). The code uses these words, in English;
the lessons use the Portuguese word in parentheses.

| Term | Meaning | First seen |
|---|---|---|
| **corpus** (acervo) | The files Garimpo answers about: in the course, the Go standard library in the `golang` image | 03-01 |
| **chunk** (trecho) | A line range of one corpus file, the unit that is embedded, indexed and cited | 03-01 |
| **`ChunkID`** | The SHA-256 of a chunk's path, line range and text: the same chunk has the same ID in every run (RN02) | 03-06 |
| **vector** (vetor) | A chunk's embedding: the model's dimension and norm 1 (RN07), so a dot product is a cosine similarity | 03-07 |
| **score** (nota) | How close a chunk is to a query: the dot product of their vectors, between -1 and 1 | 03-07 |
| **citation** (citação) | Lines of a chunk that exists in the corpus, printed as `path:line` or `path:start-end` (RN01) | 03-08 |
| **ingestion run** (rodada de coleta) | One pass of reading, cutting and embedding the corpus, which can be paused and resumed | 03-02 |
| **index snapshot** (fotografia do índice) | An immutable, complete version of the index; a query only ever reads one (RN05) | 03-02 |
| **hit** (acerto) | A chunk returned by a search, with its score | 03-02 |
| **answer** (resposta) | Generated text grounded in chunks, with citations | 03-02 |

## Design

| Term | Meaning | First seen |
|---|---|---|
| **bounded context** | A part of the domain with its own words and rules. Garimpo has five: Ingestion, Indexing, Retrieval, Answering, Evaluation | 03-02 |
| **ubiquitous language** | The words the experts, the code and the tests share for the problem; this glossary | 03-02 |
| **shared kernel** | Types several contexts share and change only together: `internal/domain/shared` | 03-03 |
| **anticorruption layer** | An adapter that translates an outside model (Ollama's API) into the domain's words, so it never leaks in | 03-03 |
| **port** | An interface a layer declares for what it needs, where it is consumed (`ChunkFinder`) | 03-04 |
| **adapter** | The code that implements a port with a real technology: HTTP, files, Ollama | 03-04 |
| **composition root** | The one place that builds the adapters and wires them into the use cases: `cmd/garimpo` | 03-04 |
| **value object** | A type defined by its values, immutable, valid from its constructor on: `ChunkID`, `Vector`, `Citation` | 03-06 |
| **invariant** | A rule that holds for every value of a type, such as norm 1 for `Vector` | 03-07 |
| **zero value** | What a variable holds before any assignment. In the value objects it means "none" and is safe to use (`IsZero`) | 03-06 |
| **property test** | A test that checks a rule on many generated inputs instead of a few examples (`testing/quick`) | 03-09 |

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
| **G, M, P** | The scheduler's three pieces: G is a goroutine, M an OS thread, P the right to run Go code. An M runs a G only while it holds a P, and there are `GOMAXPROCS` Ps | 04-02 |
| **run queue** | The runnable goroutines waiting for a P: one local queue per P and a global one | 04-02 |
| **parking** | A goroutine giving up its M and P while it waits on a channel, a timer, a lock or the network | 04-01 |
| **blocking system call** | A call into the kernel the runtime cannot park, such as a read from a disk file: the goroutine keeps its M until it returns | 04-01, INC-04 |
| **thread limit** | `runtime/debug.SetMaxThreads`, 10,000 by default: past it the program dies with `fatal error: thread exhaustion` | 04-01, INC-04 |
| **`sysmon`** | A runtime thread that runs without a P. While any P is busy it checks every 20 µs to 10 ms, takes Ps back from system calls and preempts goroutines that ran too long | 04-03 |
| **preemption** | The runtime stopping a running goroutine so another can use the P, after about 10 ms (`forcePreemptNS`) | 04-05 |
| **asynchronous preemption** | Since Go 1.14, preemption by a signal at almost any instruction. `GODEBUG=asyncpreemptoff=1` turns it off | 04-05 |
| **stack check** | The test at the start of most functions that grows the stack; it is also where cooperative preemption happens. Small leaf functions are `NOSPLIT` and have none | 04-06 |
| **`schedtrace`** | `GODEBUG=schedtrace=N`: one line about the scheduler every N ms | 04-04 |
| **execution trace** | The scheduler's events recorded by `runtime/trace` and read with `go tool trace` | 04-07 |
| **scheduler latency** | Time a goroutine spends runnable before it runs (`go tool trace -pprof=sched`) | 04-07 |

## Infrastructure

| Term | Meaning | First seen |
|---|---|---|
| **`num_thread`** | The Ollama request option that sets its CPU threads. Ollama does not read the container quota, so Garimpo always sends it | 01-06 |
| **ruleset** | GitHub's rules for a branch: here, pull requests only, four checks, squash merge | 01-05 |
| **squash merge** | Merging a pull request as one commit, with the pull request title as its message | 01-05 |
| **`depguard`** | The linter that checks which packages each layer may import (`.golangci.yml`) | 03-05 |
| **`forbidigo`** | The linter that forbids identifiers, such as `time.Now` in the domain | 03-05 |
