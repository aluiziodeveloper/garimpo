// Program contention shares many small lists between goroutines that read and append to
// them, as the nodes of a graph share their links, under four kinds of lock, and prints
// how long each kind took and how long the goroutines waited for its locks:
//
//   - mutex: one sync.Mutex for every list;
//
//   - rwmutex: one sync.RWMutex, which readers share and a writer takes alone;
//
//   - striped: 64 sync.Mutex, list i under lock i%64;
//
//   - perlist: one sync.Mutex in each list, the way the HNSW nodes lock their links.
//
//     go run ./labs/contention -lists 10000 -ops 200000 -reads 90 -goroutines 1,2,4,8
package main

import (
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"runtime"
	"runtime/metrics"
	"strconv"
	"strings"
	"sync"
	"time"
)

// listLen is how many values a list keeps, the links of a node on level 0 with M = 16.
const listLen = 32

// store is a set of lists that many goroutines read and append to.
type store interface {
	// Read copies list i into dst and returns how many values it copied.
	Read(i int, dst []int32) int
	// Append adds v to list i; a full list drops its oldest value.
	Append(i int, v int32)
}

// list is a ring of the last listLen values appended.
type list struct {
	vals [listLen]int32
	n    int // how many values were ever appended
}

func (l *list) read(dst []int32) int {
	return copy(dst, l.vals[:min(l.n, listLen)])
}

func (l *list) append(v int32) {
	l.vals[l.n%listLen] = v
	l.n++
}

type mutexStore struct {
	mu    sync.Mutex
	lists []list
}

func (s *mutexStore) Read(i int, dst []int32) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lists[i].read(dst)
}

func (s *mutexStore) Append(i int, v int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lists[i].append(v)
}

type rwmutexStore struct {
	mu    sync.RWMutex
	lists []list
}

func (s *rwmutexStore) Read(i int, dst []int32) int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lists[i].read(dst)
}

func (s *rwmutexStore) Append(i int, v int32) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lists[i].append(v)
}

// stripes is how many locks the striped store has.
const stripes = 64

type stripedStore struct {
	mu    [stripes]sync.Mutex
	lists []list
}

func (s *stripedStore) Read(i int, dst []int32) int {
	mu := &s.mu[i%stripes]
	mu.Lock()
	defer mu.Unlock()
	return s.lists[i].read(dst)
}

func (s *stripedStore) Append(i int, v int32) {
	mu := &s.mu[i%stripes]
	mu.Lock()
	defer mu.Unlock()
	s.lists[i].append(v)
}

type lockedList struct {
	mu sync.Mutex
	list
}

type perListStore struct{ lists []lockedList }

func (s *perListStore) Read(i int, dst []int32) int {
	l := &s.lists[i]
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.read(dst)
}

func (s *perListStore) Append(i int, v int32) {
	l := &s.lists[i]
	l.mu.Lock()
	defer l.mu.Unlock()
	l.append(v)
}

// kinds are the kinds of store, in the order the program prints them.
var kinds = []string{"mutex", "rwmutex", "striped", "perlist"}

func newStore(kind string, lists int) (store, error) {
	switch kind {
	case "mutex":
		return &mutexStore{lists: make([]list, lists)}, nil
	case "rwmutex":
		return &rwmutexStore{lists: make([]list, lists)}, nil
	case "striped":
		return &stripedStore{lists: make([]list, lists)}, nil
	case "perlist":
		return &perListStore{lists: make([]lockedList, lists)}, nil
	}
	return nil, fmt.Errorf("no store %q", kind)
}

// work runs ops operations on s from goroutines at once, each picking a list at random
// and reading it with probability reads/100, or appending to it.
func work(s store, lists, ops, reads, goroutines int) {
	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Go(func() {
			r := newRand(uint64(g))
			var dst [listLen]int32
			for range ops / goroutines {
				i := r.IntN(lists)
				if r.IntN(100) < reads {
					s.Read(i, dst[:])
				} else {
					s.Append(i, int32(g))
				}
			}
		})
	}
	wg.Wait()
}

// mutexWait reads how long goroutines have spent blocked on locks, all told.
func mutexWait() time.Duration {
	sample := []metrics.Sample{{Name: "/sync/mutex/wait/total:seconds"}}
	metrics.Read(sample)
	return time.Duration(sample[0].Value.Float64() * float64(time.Second))
}

func main() {
	lists := flag.Int("lists", 10000, "how many lists")
	ops := flag.Int("ops", 200000, "operations in all, split among the goroutines")
	reads := flag.Int("reads", 90, "the percentage of operations that read")
	counts := flag.String("goroutines", "1,2,4,8", "the numbers of goroutines to try")
	flag.Parse()
	var gs []int
	for f := range strings.SplitSeq(*counts, ",") {
		g, err := strconv.Atoi(f)
		if err != nil || g < 1 {
			fmt.Fprintf(os.Stderr, "contention: bad -goroutines %q\n", *counts)
			os.Exit(2)
		}
		gs = append(gs, g)
	}
	fmt.Printf("%d lists, %d operations, %d%% reads, GOMAXPROCS=%d\n", *lists, *ops, *reads, runtime.GOMAXPROCS(0))
	fmt.Printf("%-10s %-11s %-12s %s\n", "kind", "goroutines", "time", "waiting for locks")
	for _, kind := range kinds {
		for _, g := range gs {
			s, err := newStore(kind, *lists)
			if err != nil {
				fmt.Fprintln(os.Stderr, "contention:", err)
				os.Exit(2)
			}
			before, start := mutexWait(), time.Now()
			work(s, *lists, *ops, *reads, g)
			took, waited := time.Since(start), mutexWait()-before
			fmt.Printf("%-10s %-11d %-12v %v\n", kind, g, took.Round(time.Microsecond), waited.Round(time.Microsecond))
		}
	}
}

func newRand(seed uint64) *rand.Rand { return rand.New(rand.NewPCG(seed, 1)) }
