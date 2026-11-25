# Lab: channels with and without a buffer

A producer goroutine sends five lines on a channel and closes it. The consumer ranges over
the channel and spends 20 ms on each line. At every line it prints how many values are
waiting in the channel (`len(ch)`).

## Run it

```bash
docker compose exec dev go run ./labs/channels -buf 0
docker compose exec dev go run ./labs/channels -buf 5
docker compose exec dev go test -race ./labs/channels
```

## What it shows

Real runs on 2026-11-25 (Go 1.27.4). The times change a little from run to run.

Without a buffer, a send waits until the consumer receives. Nothing ever waits in the
channel: the producer is parked on the send.

```
   20ms  handled "one"   waiting in the channel: 0
   41ms  handled "two"   waiting in the channel: 0
   61ms  handled "three" waiting in the channel: 0
   81ms  handled "four"  waiting in the channel: 0
  102ms  handled "five"  waiting in the channel: 0
  102ms  5 lines, buffer 0
```

With a buffer of 5, the producer puts all five lines in the buffer and finishes. The
consumer still needs 20 ms per line:

```
   20ms  handled "one"   waiting in the channel: 4
   41ms  handled "two"   waiting in the channel: 3
   61ms  handled "three" waiting in the channel: 2
   81ms  handled "four"  waiting in the channel: 1
  102ms  handled "five"  waiting in the channel: 0
  102ms  5 lines, buffer 5
```

The total is the same, about 100 ms. A buffer lets the producer run ahead; it does not make
a slow consumer faster.

## The classic mistake

**Forgetting `close(ch)`.** `for line := range ch` only ends when the channel is closed.
Without `close`, the consumer handles the five lines and then waits forever. In the
program, no other goroutine can wake it, and the runtime stops it:

```
fatal error: all goroutines are asleep - deadlock!

goroutine 1 [chan receive]:
main.run({0x580ad0, 0x18b42b6ec030}, 0x0, 0x1312d00)
	/src/labs/channels/main.go:35 +0x98
main.main()
	/src/labs/channels/main.go:24 +0xd5
exit status 2
```

In a test, the same mistake does not crash. The testing package arms a timer for
`-timeout` (10 minutes by default), and the runtime does not report a deadlock while a
timer is pending (`checkdead` in `runtime/proc.go`). The test hangs until the timer fires:

```
panic: test timed out after 10s
	running tests:
		TestLinesSendsEveryLineInOrder (10s)
```

The rule: the goroutine that sends is the one that closes, once, after the last send.
