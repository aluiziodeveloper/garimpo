package main

import (
	"fmt"
	"sync"
	"testing"
)

// Every store keeps every append: 8 goroutines append to the same 4 lists while 4 read
// them, and each list ends with the count of appends it got, with -race watching.
func TestEveryStoreKeepsEveryAppend(t *testing.T) {
	for _, kind := range kinds {
		t.Run(kind, func(t *testing.T) {
			s, err := newStore(kind, 4)
			if err != nil {
				t.Fatal(err)
			}
			var wg sync.WaitGroup
			for g := range 8 {
				wg.Go(func() {
					for i := range 1000 {
						s.Append(i%4, int32(g))
					}
				})
			}
			for range 4 {
				wg.Go(func() {
					var dst [listLen]int32
					for i := range 1000 {
						if n := s.Read(i%4, dst[:]); n > listLen {
							t.Errorf("Read copied %d values, more than %d", n, listLen)
						}
					}
				})
			}
			wg.Wait()
			for i := range 4 {
				if n := count(s, i); n != 2000 {
					t.Errorf("list %d got %d appends, want 2000", i, n)
				}
			}
		})
	}
}

// count returns how many values were ever appended to list i of s.
func count(s store, i int) int {
	switch s := s.(type) {
	case *mutexStore:
		return s.lists[i].n
	case *rwmutexStore:
		return s.lists[i].n
	case *stripedStore:
		return s.lists[i].n
	case *perListStore:
		return s.lists[i].n
	}
	panic(fmt.Sprintf("no count for %T", s))
}

// BenchmarkStore measures one operation, a read or an append of a list picked at random,
// with GOMAXPROCS goroutines at once (-cpu), for every kind of store and three shares of
// reads.
//
//	go test -run '^$' -bench Store -cpu 1,2,4,8 ./labs/contention
func BenchmarkStore(b *testing.B) {
	const lists = 10000
	for _, reads := range []int{50, 90, 99} {
		for _, kind := range kinds {
			b.Run(fmt.Sprintf("reads=%d/kind=%s", reads, kind), func(b *testing.B) {
				s, err := newStore(kind, lists)
				if err != nil {
					b.Fatal(err)
				}
				var seq sync.Mutex
				seed := uint64(0)
				b.RunParallel(func(pb *testing.PB) {
					seq.Lock()
					seed++
					r := newRand(seed)
					seq.Unlock()
					var dst [listLen]int32
					for pb.Next() {
						i := r.IntN(lists)
						if r.IntN(100) < reads {
							s.Read(i, dst[:])
						} else {
							s.Append(i, 1)
						}
					}
				})
			})
		}
	}
}
