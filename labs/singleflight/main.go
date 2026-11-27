// Program singleflight asks the same slow search many times at once, in three ways, and
// counts how many searches ran: plain, one search per question; do, singleflight.Group.Do
// with the context of the question that came first; and detached, DoChan with a context of
// the search's own, which each question waits for with its own context.
//
//	go run ./labs/singleflight
//	go test -v ./labs/singleflight
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"golang.org/x/sync/singleflight"
)

// slow is a search that takes took, or less if its context ends first. It counts its calls.
type slow struct {
	took  time.Duration
	calls atomic.Int64
}

func (s *slow) search(ctx context.Context, q string) ([]string, error) {
	s.calls.Add(1)
	select {
	case <-time.After(s.took):
		return []string{"hits of " + q}, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// searcher is one way to ask the search.
type searcher interface {
	Search(ctx context.Context, q string) ([]string, error)
}

// plain runs one search per question.
type plain struct{ s *slow }

func (p *plain) Search(ctx context.Context, q string) ([]string, error) { return p.s.search(ctx, q) }

// do runs one search per query at a time, with Do. The search runs with the context of the
// question that started it: when that question is canceled, the search is, and every
// question waiting for it gets the error.
type do struct {
	s *slow
	g singleflight.Group
}

func (d *do) Search(ctx context.Context, q string) ([]string, error) {
	v, err, _ := d.g.Do(q, func() (any, error) { return d.s.search(ctx, q) })
	if err != nil {
		return nil, err
	}
	return v.([]string), nil
}

// detached runs one search per query at a time, with DoChan. The search runs with a context
// of its own: the values of the first question's, none of its cancel, and a deadline of
// timeout. Each question waits for the result or for its own context: a question that
// leaves leaves alone, and the search goes on for the others, up to timeout.
type detached struct {
	s       *slow
	g       singleflight.Group
	timeout time.Duration
}

func (d *detached) Search(ctx context.Context, q string) ([]string, error) {
	ch := d.g.DoChan(q, func() (any, error) {
		own, cancel := context.WithTimeout(context.WithoutCancel(ctx), d.timeout)
		defer cancel()
		return d.s.search(own, q)
	})
	select {
	case r := <-ch:
		if r.Err != nil {
			return nil, r.Err
		}
		return r.Val.([]string), nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// outcome is what n equal questions at once got.
type outcome struct {
	searches, errs int64
	took           time.Duration
}

// ask asks q n times at once through s, and returns how many searches ran, how many
// questions failed and how long they all took. With cancelFirst, the first question is
// canceled after that long, once the search it started is under way.
func ask(s searcher, counter *slow, q string, n int, cancelFirst time.Duration) outcome {
	before := counter.calls.Load()
	var errs atomic.Int64
	var wg sync.WaitGroup
	start := time.Now()
	run := func(ctx context.Context) {
		if _, err := s.Search(ctx, q); err != nil {
			errs.Add(1)
		}
	}
	first, cancel := context.WithCancel(context.Background())
	defer cancel()
	wg.Go(func() { run(first) })
	for counter.calls.Load() == before { // the first question started its search
		time.Sleep(time.Millisecond)
	}
	for range n - 1 {
		wg.Go(func() { run(context.Background()) })
	}
	if cancelFirst > 0 {
		time.Sleep(cancelFirst)
		cancel()
	}
	wg.Wait()
	return outcome{counter.calls.Load() - before, errs.Load(), time.Since(start)}
}

func main() {
	n := flag.Int("n", 100, "equal questions at once")
	took := flag.Duration("took", 50*time.Millisecond, "how long a search takes")
	flag.Parse()
	if *n < 1 {
		fmt.Fprintln(os.Stderr, "singleflight: -n must be at least 1")
		os.Exit(2)
	}
	fmt.Printf("%d equal questions at once, a search of %v\n", *n, *took)
	fmt.Printf("%-9s %-17s %9s %7s %8s\n", "way", "first question", "searches", "errors", "took")
	for _, cancelFirst := range []time.Duration{0, *took / 5} {
		label := "waits"
		if cancelFirst > 0 {
			label = fmt.Sprint("canceled at ", cancelFirst)
		}
		for _, way := range []string{"plain", "do", "detached"} {
			s := &slow{took: *took}
			var x searcher
			switch way {
			case "plain":
				x = &plain{s: s}
			case "do":
				x = &do{s: s}
			default:
				x = &detached{s: s, timeout: 10 * *took}
			}
			o := ask(x, s, "sort a slice", *n, cancelFirst)
			fmt.Printf("%-9s %-17s %9d %7d %8v\n", way, label, o.searches, o.errs, o.took.Round(time.Millisecond))
		}
	}
}
