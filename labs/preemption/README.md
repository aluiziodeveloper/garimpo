# Lab: the loop that does not give the CPU back

One goroutine runs a busy loop; another only wants to sleep 10 ms. Both share one P
(`GOMAXPROCS=1` inside `run`). When the sleeper's timer fires, the loop holds the P, so the
sleeper runs again only when the runtime takes the P from the loop.

Since Go 1.14 the runtime can do that with a signal, anywhere in the loop. With
`GODEBUG=asyncpreemptoff=1` it can stop a goroutine only at a check the compiler puts at the
start of most functions, the stack check. Three loops:

| `-loop` | Each step | Stack check in the loop? |
|---|---|---|
| `spin` | Arithmetic, no calls | No |
| `leaf` | Calls `leaf`, a small function that calls nothing | No: `leaf` is `NOSPLIT` |
| `call` | Calls `step`, which calls `leaf` | Yes, at the start of `step` |

Every loop also stops after `-n` steps (2 billion by default), so no run hangs.

## Run it

```bash
docker compose run --rm lab sh -c 'go build -o /tmp/preemption ./labs/preemption &&
    for l in spin leaf call; do
      /tmp/preemption -loop $l; GODEBUG=asyncpreemptoff=1 /tmp/preemption -loop $l
    done'
docker compose exec dev sh -c 'go build -gcflags=-S -o /dev/null ./labs/preemption 2>&1 |
    grep -A1 "main\.\(leaf\|step\) STEXT"'
docker compose exec dev go test -race ./labs/preemption
```

## What it printed

On 2026-11-25, `lab` service (2 CPUs), Go 1.27.4:

```
$ GODEBUG= /tmp/preemption -loop spin
loop=spin GODEBUG=""
asked to sleep 10ms, woke after 15ms; the loop ran 15ms
$ GODEBUG=asyncpreemptoff=1 /tmp/preemption -loop spin
loop=spin GODEBUG="asyncpreemptoff=1"
asked to sleep 10ms, woke after 1.039s; the loop ran 1.039s
$ GODEBUG= /tmp/preemption -loop leaf
loop=leaf GODEBUG=""
asked to sleep 10ms, woke after 15ms; the loop ran 15ms
$ GODEBUG=asyncpreemptoff=1 /tmp/preemption -loop leaf
loop=leaf GODEBUG="asyncpreemptoff=1"
asked to sleep 10ms, woke after 1.951s; the loop ran 1.951s
$ GODEBUG= /tmp/preemption -loop call
loop=call GODEBUG=""
asked to sleep 10ms, woke after 10ms; the loop ran 10ms
$ GODEBUG=asyncpreemptoff=1 /tmp/preemption -loop call
loop=call GODEBUG="asyncpreemptoff=1"
asked to sleep 10ms, woke after 17ms; the loop ran 17ms
```

With asynchronous preemption, the sleeper woke after 10 to 15 ms with every loop. Without it,
the `spin` and `leaf` loops kept the P until they ran out of steps, 1.039 s and 1.951 s; the
`call` loop gave it back after 17 ms.

The compiler output shows why `leaf` is no better than `spin`:

```
main.leaf STEXT nosplit size=14 align=0x0 args=0x10 locals=0x0 funcid=0x0
	0x0000 00000 (/src/labs/preemption/main.go:48)	TEXT	main.leaf(SB), NOSPLIT|NOFRAME|ABIInternal, $0-16
--
main.step STEXT size=52 align=0x0 args=0x10 locals=0x18 funcid=0x0
	0x0000 00000 (/src/labs/preemption/main.go:54)	TEXT	main.step(SB), ABIInternal, $24-16
```

`leaf` is `NOSPLIT|NOFRAME`: no stack check. `step` has a frame and starts with the check
(`CMPQ SP, 16(R14)` in the full listing). A function call is a preemption point only when
the called function has the check.

## The rule

The `runtime` package documentation: "asyncpreemptoff=1 disables signal-based asynchronous
goroutine preemption. This makes some loops non-preemptible for long periods, which may
delay GC and goroutine scheduling."
