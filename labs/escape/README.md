# labs/escape

Where Go keeps a value: on the goroutine's stack, freed for free when the function
returns, or on the heap, where only the garbage collector frees it. The compiler decides,
and `-gcflags=-m` says what it decided; `testing.AllocsPerRun` counts what each decision
costs at run time.

```sh
go build -gcflags=-m ./labs/escape      # the decisions
go build -gcflags='-m -m' ./labs/escape # and why, step by step
go run ./labs/escape                    # the allocations of each call
go test ./labs/escape
```

Measured on 26/11/2026 (Go 1.27.4, linux/amd64):

| Call | What `-gcflags=-m` says | Allocations |
|---|---|---|
| `byValue` | nothing: `p` stays in the frame | 0 |
| `byPointer` | `moved to heap: p` | 1 |
| `boxed` | `p escapes to heap` | 1 |
| `fixedBuffer` | `make([]byte, 64) does not escape` | 0 |
| `sizedBuffer(32)` | `make([]byte, n) does not escape` | 0 |
| `sizedBuffer(33)` | the same line | 1 |
| `counter` | `moved to heap: n` and `func literal escapes to heap` | 2 |
| `localClosure` | nothing: the closure is inlined | 0 |

"Does not escape" is not "does not allocate". A `make` of a size known only at run time
gets a 32-byte buffer on the stack, and a larger one goes to the heap: the threshold is
`VariableMakeThreshold` in `cmd/compile/internal/base/flag.go`.

The functions are `//go:noinline`. Inlined into the caller, each one would be decided
again with the caller's code, and the lab shows the decision of the function itself.
