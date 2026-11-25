# labs/iterators

The two shapes of a Go iterator, and the rule that holds them together.

- **Push**, `iter.Seq[V]`: a function that calls `yield` for each value. `for v := range seq`
  turns the loop body into `yield`; `break` makes `yield` return false.
- **Pull**, `iter.Pull(seq)`: returns `next` and `stop`. Each `next` call hands out one
  value, so two sequences can be walked side by side (`merge`). `stop` must be called.

```bash
docker compose run --rm lab go run ./labs/iterators -case push
docker compose run --rm lab go run ./labs/iterators -case merge
docker compose run --rm lab go run ./labs/iterators -case cleanup
docker compose run --rm lab go run ./labs/iterators -case deaf
docker compose run --rm bench sh -c 'go test -run "^$" -bench . -benchmem -count 10 ./labs/iterators > /tmp/b.txt &&
    go tool -modfile=tools/go.mod benchstat -col /kind /tmp/b.txt'
```

| Case | What it shows |
|---|---|
| `push` | `take(evens(numbers(100)), 4)` yields 0, 2, 4, 6 and stops `numbers` early |
| `merge` | Two sorted sequences merged with two `iter.Pull` |
| `cleanup` | A `defer` inside the iterator runs when the loop breaks: `closed: true` |
| `deaf` | An iterator that ignores what `yield` returns panics on `break` |

## What it showed

On 2026-11-25, Go 1.27.4. The `deaf` case, `lab` service:

```
value 0
value 1
panic: runtime error: range function continued iteration after function for loop body returned false
```

Summing 1,000 values, `bench` service (CPUs 2 and 4 of an i7-13700H), 10 runs:

| Kind | Time per sum | Allocations |
|---|---|---|
| `slice` | 614.0 ns | 0 |
| `push` | 1,218.5 ns (+98%) | 0 |
| `pull` | 68,858.5 ns (about 69 ns per value) | 7 (224 B) |

- A push iterator costs about one nanosecond per value here, and allocates nothing.
- `iter.Pull` switches between two goroutines on every `next`: use it only when the loop
  needs it, as in `merge`.
