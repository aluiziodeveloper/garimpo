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
| **document** (documento) | One file of the corpus: a relative, slash-separated path and valid UTF-8 text (`ingestion.Document`) | 06-06 |
| **chunk** (trecho) | A line range of one corpus file, the unit that is embedded, indexed and cited | 03-01 |
| **window** (janela) | The most bytes a chunk holds: 700 by default (D15). Chunks take whole lines; only a line longer than the window is split, never inside a character (`ingestion.Cutter`) | 06-07 |
| **`ChunkID`** | The SHA-256 of a chunk's path, byte offset, line range and text: the same chunk has the same ID in every run (RN02), and two chunks never share one (the offset since module 07) | 03-06 |
| **vector** (vetor) | A chunk's embedding: the model's dimension and norm 1 (RN07), so a dot product is a cosine similarity | 03-07 |
| **score** (nota) | How close a chunk is to a query: the dot product of their vectors, between -1 and 1 | 03-07 |
| **citation** (citação) | Lines of a chunk that exists in the corpus, printed as `path:line` or `path:start-end` (RN01) | 03-08 |
| **ingestion run** (rodada de coleta) | One pass of reading, cutting and embedding the corpus, which can be paused and resumed | 03-02 |
| **pending chunk** (trecho pendente) | A chunk cut and not yet embedded or failed. RN03 bounds how many exist at once (`Config.MaxPending`, 256 by default) | 07-09 |
| **concurrency limit** (limite de concorrência) | How many calls may go to the embedding model at the same time, moved by the adaptive limiter (`Config.Limiter`). RN04: no new call starts above it; when it goes down, the calls in flight finish | 09-07 |
| **journal** (diário) | The file of a run's events, in the order the run recorded them, one line each with the CRC-32C of its JSON (`journal.File`). A chunk confirmed in it is never embedded again (RN02) | 10-05 |
| **progress** (progresso) | What an ingestion run has done so far: documents read, chunks cut, embedded and failed, and the pending ones. A snapshot of it is one moment of the run (`ingestion.Progress`) | 05-07 |
| **skipped chunk** (trecho pulado) | A chunk a resumed run cuts and does not embed, because the journal has it as confirmed or failed (`skipped=` in the progress) | 10-07 |
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
| **aggregate** | A cluster of objects changed only through one root, which checks the invariants and the state machine: `ingestion.Run`, the `IngestionRun`. One goroutine owns it | 07-01 |
| **domain event** | Something that happened to an aggregate, in the past tense (`ChunkEmbedded`), recorded by each accepted command and handed out with `TakeEvents`; module 10 writes them to the journal | 07-03 |
| **fuzzing** | A property test whose inputs Go generates and mutates, guided by coverage (`go test -fuzz`). A failing input is saved under `testdata/fuzz` and runs as a test from then on | 06-08 |
| **contract test** | One suite every implementation of a port must pass, such as `embeddertest.Contract` for the fake, the Ollama adapter, the `Batcher` and a real Ollama. It lives outside the port's package, so the port's own tests do not import it back | 08-02 |
| **decorator** | A type that implements a port by wrapping another implementation of it and adding a behavior, such as the `Batcher` in front of the Ollama `Embedder` | 08-07 |
| **micro-batching** | Gathering the calls of concurrent callers into one request, sent when it is full or a short wait after its first call, whichever comes first. It pays when a request costs more than its work (lesson 08-09) | 08-07 |
| **Little's law** | L = λ·W: the calls in a system are its throughput times the time each one spends in it. With the throughput fixed by the slowest stage, more calls in the system only make each one wait longer (INC-09) | 09-02 |
| **closed loop** | Callers that wait for each answer before the next call, such as the ingestion's workers: more of them never raise the throughput past the service's capacity, only the queue in front of it | 09-02 |
| **AIMD** | Additive increase, multiplicative decrease: a limit that goes up by one while things go well and is multiplied by a factor under 1 when they do not, as in TCP's congestion control | 09-04 |
| **adaptive limiter** | A concurrency limit that follows the latency of the calls instead of a fixed number: `internal/app/limiter`, AIMD with a tolerance of 1.5 times the base and a backoff of 0.9 | 09-05 |
| **base latency** | The latency of a call with no queue. The limiter takes the lowest smoothed mean it saw (weight 1/8), so a service that gets slower for good pins the limit at `Min` | 09-05 |
| **permit** | The right to make one call, from `Acquire`. `Done` ends a call that worked and gives its latency to the limiter; `Discard` ends one that failed or was canceled, with no sample | 09-05 |
| **write-ahead log (WAL)** | A log written before the state it records is acted on, so that a crash loses nothing that was confirmed: Garimpo's journal | 10-02 |
| **event sourcing** | Keeping the events as the truth and the state as what they add up to: `ingestion.RestoreRun` rebuilds a run by running again the command each event records | 10-02 |
| **graceful shutdown** | Stopping on request without losing work: no new work starts, the work in flight finishes and is recorded, the run pauses. `SIGTERM` does it for `garimpo ingest`; a second signal ends the process | 10-08 |

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
| **data race** | Two goroutines accessing the same memory, at least one writing and not both atomically, with no happens-before order between them. `c++` from two goroutines loses adds | 05-01 |
| **happens before** | The memory model's order between two operations: within a goroutine, program order; across goroutines, a synchronization (channel, lock, atomic, `go`). A read sees a write only if the write happens before it | 05-02 |
| **synchronized before** | The edge a synchronization creates between goroutines, such as a send and the matching receive, or an `Unlock` and the next `Lock` | 05-03 |
| **DRF-SC** | The memory model's promise: a program with no data races behaves as if its goroutines ran interleaved on one processor | 05-03 |
| **typed atomic** | `atomic.Int64`, `atomic.Bool`, `atomic.Pointer[T]` and the others: every access is atomic, and `c++` does not compile | 05-05 |
| **compare-and-swap (CAS)** | An atomic "write the new value only if it is still the old one"; a loop of them updates a value lock-free | 05-05 |
| **`noCopy`** | A field of the atomic and `sync` types that makes `go vet` (copylocks) report a copy, such as passing a `Progress` by value | 05-07 |
| **false sharing** | Goroutines writing different variables on the same 64-byte cache line, which slows them down as if they shared one | 05-06 |
| **check-then-act** | Reading a value and acting on it as two steps: correct for one goroutine, wrong for two even when both steps are atomic (`sync.Once` does it in one) | 05-04 |
| **range-over-func** | Since Go 1.23, `for v := range f` with a function `f(yield func(V) bool)`: the loop body becomes `yield`, and `break` makes `yield` return false | 06-02 |
| **iterator** (`iter.Seq`, `iter.Seq2`) | A function that pushes the values of a sequence to `yield`, one at a time, and stops when `yield` returns false. Calling `yield` again after false panics | 06-02 |
| **pull iterator** (`iter.Pull`) | A push iterator turned into `next` and `stop` calls, so two sequences can be walked side by side. Each `next` switches goroutines: about 69 ns per value against 1 ns for a push iterator (lesson 06-04). `stop` must be called | 06-03 |
| **single-use iterator** | An iterator over a stream that cannot be rewound, such as lines read from a connection: calling it again does not start over. Its doc comment says so | 06-03 |
| **generic method** | Since Go 1.27, a method with type parameters of its own, such as `Map[W any]` on `Seq[V]`. It must be instantiated before use as a value, and it satisfies no interface | 06-05 |
| **pipeline** | Stages connected by channels, each one a goroutine or a group of them, so that one stage works while the next handles what came before | 07-04 |
| **fan-out, fan-in** | Fan-out: several goroutines take work from one channel. Fan-in: their results go into one channel. A waiting stage scales past the CPUs; a busy one stops at them (lesson 07-05) | 07-04 |
| **`errgroup`** | `golang.org/x/sync/errgroup`: goroutines whose first error cancels the group's context and is the one `Wait` returns. `Wait` waits for all of them, so a stage that never returns hangs the group | 07-05 |
| **backpressure** | A slow stage making the fast one before it wait, through a bounded channel, instead of letting the work pile up in memory (INC-07) | 07-08 |
| **peak live heap** | The largest heap the garbage collector found alive during a run, sampled from `/gc/heap/live:bytes` (`runtime/metrics`) | 06-10 |
| **`synctest` bubble** | The goroutines started inside `synctest.Test`, with a fake clock of their own, starting at 2000-01-01, that moves only when all of them are durably blocked | 08-05 |
| **durably blocked** | A goroutine of a bubble blocked on something only the bubble can wake: a channel made in the bubble, `sync.WaitGroup.Wait`, `sync.Cond.Wait`, `time.Sleep`. A `sync.Mutex` and a real socket are not | 08-05 |
| **`synctest.Wait`** | Waits until every other goroutine of the bubble is durably blocked; what they did before blocking happens before it returns. The fake clock moving orders nothing: a read before it and a write it wakes up race (lesson 08-06) | 08-06 |
| **in-memory network** | Since Go 1.27, `httptest.NewTestServer`: a server and client that talk without a socket or a port, so a bubble can move its clock across an HTTP call | 08-08 |
| **page cache** | The kernel's copy of file data in memory. A write returns once the bytes are there, not on the disk: they survive a `kill -9`, not a power cut | 10-03 |
| **`fsync`** (`File.Sync`) | The system call that returns once the file's data are on the disk. About a millisecond here, whatever it carries, so batching writes per sync is what makes it affordable (`labs/fsync`) | 10-03 |
| **directory sync** | A new file's name lives in its directory: it is durable only after a `Sync` of the directory too, as `journal.Open` does | 10-03 |
| **torn write** | A write a crash cut in the middle, leaving part of a record or bytes the disk never got. The journal's CRC finds it, and `Open` drops it when it is the last line | 10-06 |
| **`signal.NotifyContext`** | A context canceled when the process gets one of the given signals. While it is in place the signal does not end the process; `stop` gives the default back | 10-08 |
| **`context.WithoutCancel`** | A context with the values of its parent and none of its cancellation: the calls in flight finish after a stop | 10-08 |
| **escape analysis** | The compiler's decision, made per function, of what can live in the goroutine's stack (freed for free when the function returns) and what must go to the heap (freed by the garbage collector): a value whose address outlives the frame, or that goes into an interface, escapes. `-gcflags=-m` prints the decisions (`labs/escape`) | 11-02 |
| **does not escape** | What `-gcflags=-m` says of a value that stays in the frame. It is not "does not allocate": a `make` of a size known only at run time gets a 32-byte buffer on the stack, and a larger one goes to the heap (`VariableMakeThreshold` in the compiler) | 11-03 |
| **bounds check** (checagem de limites) | The comparison Go makes before `s[i]`, which panics when `i` is out of the slice. The compiler drops it when it can prove the index is in range: in `vec.Dot`, the check that the lengths are equal proves it for `b[i]`. `-gcflags=-d=ssa/check_bce/debug=1` lists the ones left | 11-04 |
| **`benchstat`** | `golang.org/x/perf/cmd/benchstat`: compares two sets of benchmark runs and says whether the difference is beyond the noise (`p` below 0.05). Ten runs of each, on a fixed CPU, in the course | 11-05 |
| **dependency chain** (cadeia de dependência) | Additions where each waits for the result of the one before, as in a sum with one accumulator. Four accumulators break it into four chains the processor runs at once: 26% off `vec.Dot` | 11-05 |
| **SIMD** | One instruction on several values: a 256-bit AVX2 register holds 8 float32, and one FMA (`MulAdd`, a*b+c) handles them all | 11-06 |
| **`simd/archsimd`** | Go 1.27's experimental package of the amd64 vector registers as types (`Float32x8`), with `archsimd.X86` to ask the processor what it has. It exists only with `GOEXPERIMENT=simd` and is outside the Go 1 promise; `simd` is the portable one, as wide as the machine | 11-06 |
| **`ClearAVXUpperBits`** | `VZEROUPPER`: clears the upper half of the vector registers after AVX code, before SSE instructions use them. Without it, the AVX2 dot product went from 58 to 145 ns (`labs/simd`) | 11-07 |
| **memory-bound** (preso à memória) | A loop whose pace is set by how fast the data come from memory, not by the arithmetic. The exact search over the whole corpus reads 64 MB per query, more than the 24 MiB of L3: SIMD took 44% off a search that fits in the cache and 20% off this one | 11-09 |

## Infrastructure

| Term | Meaning | First seen |
|---|---|---|
| **`num_thread`** | The Ollama request option that sets its CPU threads. Ollama does not read the container quota, so Garimpo always sends it | 01-06 |
| **ruleset** | GitHub's rules for a branch: here, pull requests only, four checks, squash merge | 01-05 |
| **squash merge** | Merging a pull request as one commit, with the pull request title as its message | 01-05 |
| **`depguard`** | The linter that checks which packages each layer may import (`.golangci.yml`) | 03-05 |
| **`forbidigo`** | The linter that forbids identifiers, such as `time.Now` in the domain | 03-05 |
| **`num_ctx`** | The Ollama request option that sets the model's context in tokens. `all-minilm` gets 256 unless asked, out of the 512 it supports; Garimpo sends 512 | 08-04 |
| **`truncate`** | The `/api/embed` field that cuts an input over the context: true by default, and silent. Garimpo always sends false, and a longer text fails with `ErrTextTooLong` (INC-08) | 08-04 |
| **`SIGTERM`, `SIGKILL`** | The polite and the final stop. `docker stop` sends `SIGTERM`, waits 10 s and then sends `SIGKILL`, which no process can catch: nothing drains, and only what reached the journal remains | 10-08 |
| **virtual CPU** (CPU virtual) | A CPU that WSL 2 shows Linux. The i7-13700H shows as 10 cores of two threads, not 6 P-cores and 8 E-cores: a `cpuset` fixes a virtual CPU, and Hyper-V picks the physical core. Benchmarks compare A and B side by side, in one session | 11-09 |
