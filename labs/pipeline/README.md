# labs/pipeline

A pipeline of three stages under an `errgroup` (`golang.org/x/sync/errgroup`): a producer
makes the items 0 to n-1, several workers do the slow work on them (fan-out), and the caller
collects the results (fan-in). Measured in the `bench` service: CPUs 2 and 4 of an
i7-13700H, Go 1.27.4, 2026-11-25.

| Test | What it holds |
|---|---|
| `TestEveryItemIsWorkedOnce` | 100 items, 1 or 4 workers: 100 results, nothing lost or repeated |
| `TestTheFirstErrorStopsThePipeline` | an error on item 10 is the error `run` returns, and the producer stops early |
| `TestCancelingTheContextStopsThePipeline` | a canceled context stops it the same way |
| `TestNoGoroutineIsLeftBehind` | after ten failed runs, the same number of goroutines as before |

## Fan-out pays for waiting, not for computing

`BenchmarkPipeline` sends 64 items through a middle stage that waits 1 ms per item, as a
call to a model would, or keeps a CPU busy for about as long:

```bash
docker compose run --rm bench sh -c 'go test -run "^$" -bench Pipeline -count 10 ./labs/pipeline > /tmp/b.txt &&
    go tool -modfile=tools/go.mod benchstat -col /workers /tmp/b.txt'
```

```
goos: linux
goarch: amd64
pkg: github.com/aluiziodeveloper/garimpo/labs/pipeline
cpu: 13th Gen Intel(R) Core(TM) i7-13700H
                     │      1       │                  2                   │                  4                   │                  8                  │
                     │    sec/op    │    sec/op     vs base                │    sec/op     vs base                │   sec/op     vs base                │
Pipeline/kind=wait-2   75.633m ± 0%   37.910m ± 0%  -49.88% (p=0.000 n=10)   19.109m ± 1%  -74.73% (p=0.000 n=10)   9.572m ± 4%  -87.34% (p=0.000 n=10)
Pipeline/kind=cpu-2     80.64m ± 2%    41.19m ± 0%  -48.91% (p=0.000 n=10)    40.50m ± 2%  -49.77% (p=0.000 n=10)   40.03m ± 1%  -50.36% (p=0.000 n=10)
geomean                 78.09m         39.52m       -49.40%                   27.82m       -64.38%                  19.57m       -74.94%

                     │     1      │                  2                  │                  4                   │                  8                   │
                     │  items/s   │   items/s    vs base                │   items/s    vs base                 │   items/s    vs base                 │
Pipeline/kind=wait-2   846.2 ± 0%   1688.5 ± 0%  +99.54% (p=0.000 n=10)   3349.0 ± 1%  +295.77% (p=0.000 n=10)   6686.5 ± 5%  +690.18% (p=0.000 n=10)
Pipeline/kind=cpu-2    793.7 ± 2%   1553.5 ± 0%  +95.74% (p=0.000 n=10)   1580.0 ± 2%   +99.08% (p=0.000 n=10)   1599.0 ± 1%  +101.47% (p=0.000 n=10)
geomean                819.5        1.620k       +97.63%                  2.300k       +180.70%                  3.270k       +299.00%
```

- **A waiting stage scales past the CPUs.** Eight workers on two CPUs did 7.9 times the
  items of one: a worker that waits holds no CPU.
- **A busy stage stops at the CPUs.** Two workers doubled the rate; four and eight added
  nothing.

The same with one run each, in the `lab` service (2 CPUs):

```bash
docker compose run --rm lab go run ./labs/pipeline -kind wait -workers 8
```

## Every send selects on the context

Each stage sends with a `select` that also waits on `ctx.Done()`. Without it, the producer
blocks on its send once the workers stop on an error; `g.Wait` waits for the producer, and
the pipeline hangs instead of returning the error. With the `select` taken out of the
producer, `go test -timeout 5s -run TheFirstError ./labs/pipeline` (trimmed):

```
panic: test timed out after 5s
	running tests:
		TestTheFirstErrorStopsThePipeline (5s)

goroutine 19 [sync.WaitGroup.Wait]:
golang.org/x/sync/errgroup.(*Group).Wait(0x318766598a00)
	/go/pkg/mod/golang.org/x/sync@v0.25.0/errgroup/errgroup.go:56 +0x1e
github.com/aluiziodeveloper/garimpo/labs/pipeline.run({0x6db6e0, 0x3187665b4190}, 0xf4240, 0x4, 0x6dbde8)
	/src/labs/pipeline/main.go:72 +0x353

goroutine 20 [chan send]:
github.com/aluiziodeveloper/garimpo/labs/pipeline.run.func1()
	/src/labs/pipeline/main.go:38 +0x77
```

An `errgroup` turns the leak into a hang: `Wait` returns only when every goroutine has.
