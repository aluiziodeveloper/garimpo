// Command channels sends five lines through a channel to a slow consumer and shows how
// many lines wait in the channel at each receive, without a buffer and with one.
//
//	docker compose exec dev go run ./labs/channels -buf 0
//	docker compose exec dev go run ./labs/channels -buf 5
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

const text = "one\ntwo\nthree\nfour\nfive\n"

func main() {
	buf := flag.Int("buf", 0, "channel buffer size")
	work := flag.Duration("work", 20*time.Millisecond, "time the consumer spends on each line")
	flag.Parse()

	if err := run(os.Stdout, *buf, *work); err != nil {
		fmt.Fprintln(os.Stderr, "channels:", err)
		os.Exit(1)
	}
}

func run(w io.Writer, buf int, work time.Duration) error {
	start := time.Now()
	ch := lines(text, buf)

	n := 0
	for line := range ch {
		time.Sleep(work) // the consumer is slower than the producer
		n++
		_, err := fmt.Fprintf(w, "%7v  handled %-7q waiting in the channel: %d\n",
			since(start), line, len(ch))
		if err != nil {
			return err
		}
	}
	_, err := fmt.Fprintf(w, "%7v  %d lines, buffer %d\n", since(start), n, buf)
	return err
}

// lines sends each line of text on a new channel with room for buf values, from its own
// goroutine, and closes the channel after the last one. With buf 0, every send waits for
// a receiver; with buf > 0, the producer only waits when the buffer is full.
func lines(text string, buf int) <-chan string {
	ch := make(chan string, buf)
	go func() {
		for line := range strings.Lines(text) {
			ch <- strings.TrimSuffix(line, "\n")
		}
		close(ch)
	}()
	return ch
}

func since(start time.Time) time.Duration {
	return time.Since(start).Round(time.Millisecond)
}
