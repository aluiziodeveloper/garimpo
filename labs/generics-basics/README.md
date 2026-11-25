# Lab: generics basics

Two pieces of generic code:

- `Max[T cmp.Ordered](xs ...T) (T, bool)`: one function for any ordered type. The type
  parameter `T` has a constraint, `cmp.Ordered`, the set of types that support `<`, `<=`,
  `>=` and `>`.
- `Scored[T any]`, a generic type (any item with a score), and
  `TopK[T any](items []Scored[T], k int)`, which returns the `k` best items without
  changing the input. `TopK` never compares two items, only their scores, so `T` can be
  `any`.

The compiler infers `T` from the arguments: `Max(3, 9, 4)` is `Max[int]`.

## Run it

```bash
docker compose exec dev go run ./labs/generics-basics
docker compose exec dev go test -race ./labs/generics-basics
```

## What it shows

A real run on 2026-11-25 (Go 1.27.4):

```
Max ints: 9
Max strings: select
0.91  context/context.go
0.77  time/sleep.go
```

## The classic mistakes

**A constraint that is too wide.** With `Max[T any]`, the body uses `>`, and `any` does not
promise it. The code does not compile:

```
labs/generics-basics/main.go:22:6: invalid operation: x > best (type parameter T cannot use operator >)
```

The constraint is the contract: the body can only do what every type in it supports.

**Nothing to infer from.** `Max()` has no arguments, so the compiler has nothing to infer
`T` from:

```
labs/generics-basics/main_test.go:15:17: in call to Max, cannot infer T (declared at labs/generics-basics/main.go:15:10)
```

The test names the type: `Max[float64]()`.
