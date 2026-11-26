# labs/little

A server that serves a few calls at a time, as the Ollama on the CPU does, measured against
more and more clients in a closed loop: each client calls, waits for the answer and calls
again. Past the capacity, a client more is a call more in the queue: the throughput stays
and the latency grows. Little's law ties the three numbers: the calls in the system (L) are
the throughput (λ) times the mean time each one spends in it (W), L = λ·W. Go 1.27.4,
2026-11-26.

| Test | What it holds |
|---|---|
| `TestOneClientWaitsOnlyForTheService` | one client, two slots of 100 ms: 10 calls/s, every call takes 100 ms |
| `TestClientsUpToTheCapacityRaiseTheThroughput` | two clients: 20 calls/s, still 100 ms |
| `TestClientsOverTheCapacityOnlyWait` | eight clients: still 20 calls/s, and the median call takes 400 ms |
| `TestLittlesLawHolds` | from 1 to 16 clients, λ·W is between 90% and 100% of the clients |

The tests run in a `testing/synctest` bubble (module 08): the fake clock makes every
latency exact, and the tests take no time.

```bash
docker compose exec dev go test -race -count=1 -v ./labs/little
```

```
--- PASS: TestOneClientWaitsOnlyForTheService (0.00s)
--- PASS: TestClientsUpToTheCapacityRaiseTheThroughput (0.00s)
--- PASS: TestClientsOverTheCapacityOnlyWait (0.00s)
--- PASS: TestLittlesLawHolds (0.00s)
```

On the real clock:

```bash
docker compose exec dev go run ./labs/little
```

```
capacity 2, service 100ms, 20 calls per client
clients  calls/s    p50       p99       W         L = λ·W
1        10.0       100ms     101ms     100ms     1.00
2        19.9       100ms     101ms     100ms     2.00
4        19.9       201ms     201ms     198ms     3.95
8        19.9       402ms     402ms     394ms     7.85
16       19.9       803ms     804ms     785ms     15.65
```

From two clients on, the server is full: 20 calls/s is its capacity (2 slots / 100 ms).
Every client more waits in the queue, and W grows by the service time for each two
clients. L stays a little under the clients because the first calls of each client find
a shorter queue.

The Ollama on the CPU behaves the same (INC-09): `all-minilm:22m` on 4 CPUs embedded the
1,038 chunks of `encoding/json` at 31.7 to 33.2 chunks/s with 1 to 16 workers, and the
median call went from 29 ms with one worker to 496 ms with sixteen.
