# labs/generic-methods

Go 1.27 lets a method declare type parameters of its own: "A method declaration may declare
type parameters" (the spec's Go 1.27 section). This lab builds one lazy pipeline twice:

```go
Take(Map(Filter(Numbers(10), even), strconv.Itoa), 3) // generic functions, inside out
Numbers(10).Filter(even).Map(strconv.Itoa).Take(3)    // generic methods, left to right
```

`Seq[V]` is an `iter.Seq[V]` with methods. `Filter` and `Take` keep the type `V`, so they
could be methods before 1.27. `Map` returns a `Seq[W]`: `W` is the method's own type
parameter, and before 1.27 `Map` could only be a function.

```bash
docker compose run --rm lab go run ./labs/generic-methods
docker compose run --rm bench sh -c 'go test -run "^$" -bench . -benchmem -count 10 ./labs/generic-methods > /tmp/b.txt &&
    go tool -modfile=tools/go.mod benchstat -col /form /tmp/b.txt'
```

## What it showed

On 2026-11-25, Go 1.27.4. The three rules, each one a real compiler message:

| Attempt | Compiler |
|---|---|
| `slices.Collect(seq)` with a `Seq[string]` | `type Seq[string] ... does not match inferred type iter.Seq[string] for iter.Seq[E]`: hence the method `All` |
| `var _ Mapper = Numbers(3)`, with `Map(func(int) string) Seq[string]` in `Mapper` | `Seq[int] does not implement Mapper (wrong type for method Map)`: a generic method satisfies no interface |
| `Map[W any](...)` in an interface | `interface method must have no type parameters` |
| `toText := Numbers(10).Map` | `cannot use generic function Numbers(10).Map without instantiation` |

The cost, summing a 1,000-value chain, `bench` service (CPUs 2 and 4 of an i7-13700H),
10 runs: method 1.210 µs, function 1.234 µs (+1.94%, p=0.014), no allocations in either.
The methods call the functions; the form is a matter of reading, not of speed.
