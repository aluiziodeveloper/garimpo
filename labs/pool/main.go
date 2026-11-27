// Program pool shows sync.Pool, a cache of temporary objects that the GC empties, the way
// the BM25 search of Garimpo reuses the buffers of a search (module 15). Goroutines build a
// report of numbers in a buffer of 64 KiB, in two ways:
//
//   - plain: a new buffer for every report;
//
//   - pooled: a buffer from a sync.Pool, emptied before use and put back after.
//
// It counts the allocations, the bytes allocated and the GC cycles of each, and then what a
// pool keeps across GC cycles: the GC moves what a pool holds to a victim cache, and drops
// the victim cache at the next cycle.
//
//	go run ./labs/pool -goroutines 8 -reports 20000
package main

import (
	"flag"
	"fmt"
	"hash/crc32"
	"runtime"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// size is the capacity of a buffer: a report of lines fits in it.
const size = 64 << 10

// report appends to buf the report numbered n, 4,000 numbers one per line, and returns it.
func report(buf []byte, n int) []byte {
	for i := range 4000 {
		buf = strconv.AppendInt(buf, int64(n*4000+i), 10)
		buf = append(buf, '\n')
	}
	return buf
}

// builder builds report n and returns its checksum.
type builder interface{ Build(n int) uint32 }

type plain struct{}

func (plain) Build(n int) uint32 {
	return crc32.ChecksumIEEE(report(make([]byte, 0, size), n))
}

// pooled keeps *[]byte in the pool, not []byte: a slice in an interface is boxed, and Put
// would allocate the box every time.
type pooled struct {
	pool sync.Pool
	news atomic.Int64 // how many buffers New made
}

func newPooled() *pooled {
	p := &pooled{}
	p.pool.New = func() any {
		p.news.Add(1)
		b := make([]byte, 0, size)
		return &b
	}
	return p
}

func (p *pooled) Build(n int) uint32 {
	bp := p.pool.Get().(*[]byte)
	buf := report((*bp)[:0], n) // [:0]: the last report is still in it
	sum := crc32.ChecksumIEEE(buf)
	*bp = buf
	p.pool.Put(bp)
	return sum
}

// result is what run measured.
type result struct {
	took           time.Duration
	mallocs, bytes uint64
	gcs            uint32
	sum            uint32 // the XOR of every checksum: equal for plain and pooled
}

// run builds reports 0 to n-1 with b from goroutines goroutines.
func run(b builder, goroutines, n int) result {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := time.Now()
	sums := make([]uint32, goroutines) // each goroutine's XOR, folded at the end
	var next atomic.Int64
	var wg sync.WaitGroup
	for g := range goroutines {
		wg.Go(func() {
			for i := int(next.Add(1)) - 1; i < n; i = int(next.Add(1)) - 1 {
				sums[g] ^= b.Build(i)
			}
		})
	}
	wg.Wait()
	took := time.Since(start)
	runtime.ReadMemStats(&after)
	var sum uint32
	for _, s := range sums {
		sum ^= s
	}
	return result{took, after.Mallocs - before.Mallocs, after.TotalAlloc - before.TotalAlloc, after.NumGC - before.NumGC, sum}
}

// puts is how many buffers survivors puts in the pool.
const puts = 100

// survivors puts the buffers in a fresh pool, runs gcs GC cycles, and returns how many of
// them Get still finds before New has to make one.
func survivors(gcs int) int {
	p := newPooled()
	held := make([]*[]byte, puts)
	for i := range held {
		held[i] = p.pool.Get().(*[]byte)
	}
	for _, b := range held {
		p.pool.Put(b)
	}
	for range gcs {
		runtime.GC()
	}
	before, found := p.news.Load(), 0
	for range puts {
		p.pool.Get()
		if p.news.Load() == before {
			found++
		}
	}
	return found
}

func main() {
	goroutines := flag.Int("goroutines", 8, "how many goroutines build reports")
	n := flag.Int("reports", 20000, "how many reports to build")
	flag.Parse()
	fmt.Printf("%d reports of 4,000 numbers by %d goroutines, a buffer of %d KiB each\n", *n, *goroutines, size>>10)
	fmt.Printf("%-7s %10s %12s %12s %6s  %s\n", "kind", "time", "allocations", "MiB", "GCs", "checksum")
	for _, k := range []struct {
		name string
		b    builder
	}{{"plain", plain{}}, {"pooled", newPooled()}} {
		r := run(k.b, *goroutines, *n)
		fmt.Printf("%-7s %10v %12d %12.1f %6d  %08x\n", k.name, r.took.Round(time.Millisecond), r.mallocs, float64(r.bytes)/(1<<20), r.gcs, r.sum)
	}
	fmt.Printf("buffers that Get finds again, of %d put back, after GC cycles:\n", puts)
	for gcs := range 3 {
		fmt.Printf("  %d GC: %d\n", gcs, survivors(gcs))
	}
}
