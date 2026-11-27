// Program weak-unique shows three tools of the runtime for memory that many parts of a
// program reach, the way Garimpo reaches the paths and the texts of its chunks (module 14):
//
//   - unique.Make interns a value: equal values get the same Handle, the value is kept
//     once, and two Handles compare as fast as two pointers;
//
//   - weak.Make points to a value without keeping it alive: once the last ordinary pointer
//     to it is gone, the GC frees it and the weak pointer's Value returns nil;
//
//   - runtime.AddCleanup runs a function after the GC frees a value: here it takes the
//     dead entry out of a cache of weak pointers.
//
// It measures the heap of the paths as strings and as Handles, and what the GC leaves of a
// cache of texts of which the program keeps holding a few.
//
//	go run ./labs/weak-unique -paths 1000000 -distinct 1000 -texts 10000 -keep 1000
package main

import (
	"flag"
	"fmt"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"
	"unique"
	"weak"
)

// paths returns n paths with distinct different ones, each in its own allocation, the way
// the paths of the chunks arrive from the files and the journal: equal strings, not one
// string shared.
func paths(n, distinct int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("internal/adapters/package%04d/file.go", i%distinct)
	}
	return out
}

// heap returns the bytes of the heap in use after a full GC.
func heap() uint64 {
	runtime.GC()
	return inUse()
}

// inUse returns the bytes of the heap in use now, with no GC first: dead objects count
// until a GC frees them.
func inUse() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

// count returns how many of vals are equal to v, and the shortest of reps scans.
func count[T comparable](vals []T, v T, reps int) (int, time.Duration) {
	n, best := 0, time.Duration(1<<63-1)
	for range reps {
		start := time.Now()
		n = 0
		for _, x := range vals {
			if x == v {
				n++
			}
		}
		best = min(best, time.Since(start))
	}
	return n, best
}

// text is the text of a chunk: what the cache holds.
type text struct{ s string }

// cache maps a chunk to its text without keeping the text alive: an entry lives while some
// part of the program holds the text, and AddCleanup takes it out once the GC frees it.
type cache struct {
	mu sync.Mutex
	m  map[int]weak.Pointer[text]
}

func newCache() *cache { return &cache{m: make(map[int]weak.Pointer[text])} }

// Put keeps t as the text of chunk k, until nothing else holds it.
func (c *cache) Put(k int, t *text) {
	w := weak.Make(t)
	c.mu.Lock()
	c.m[k] = w
	c.mu.Unlock()
	// The cleanup takes the entry out only if it is still this one: k may have been put
	// again since. Its argument must not reach t, or t would never be freed.
	runtime.AddCleanup(t, c.forget, entry{k, w})
}

// entry is what the cleanup of a text needs to find it in the cache.
type entry struct {
	k int
	w weak.Pointer[text]
}

func (c *cache) forget(e entry) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.m[e.k] == e.w {
		delete(c.m, e.k)
	}
}

// Get returns the text of chunk k, if some part of the program still holds it.
func (c *cache) Get(k int) (*text, bool) {
	c.mu.Lock()
	w, ok := c.m[k]
	c.mu.Unlock()
	if !ok {
		return nil, false
	}
	t := w.Value()
	return t, t != nil
}

// Len returns how many entries the cache has, dead or alive.
func (c *cache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.m)
}

// settle runs the GC until the cache has want entries or d passes, and returns how long it
// took. The cleanups run in a goroutine of their own, after the GC that frees the texts.
func settle(c *cache, want int, d time.Duration) (time.Duration, bool) {
	start := time.Now()
	for c.Len() != want {
		if time.Since(start) > d {
			return time.Since(start), false
		}
		runtime.GC()
		time.Sleep(time.Millisecond)
	}
	return time.Since(start), true
}

func main() {
	n := flag.Int("paths", 1_000_000, "how many chunk paths")
	distinct := flag.Int("distinct", 1000, "how many of them are different")
	texts := flag.Int("texts", 10_000, "how many texts the cache gets")
	size := flag.Int("size", 4096, "the bytes of each text")
	keep := flag.Int("keep", 1000, "how many texts the program keeps holding")
	flag.Parse()
	if *distinct < 1 || *n < *distinct || *keep > *texts {
		fmt.Fprintln(os.Stderr, "weak-unique: want 1 <= distinct <= paths and keep <= texts")
		os.Exit(2)
	}
	mib := func(b uint64) float64 { return float64(b) / (1 << 20) }

	fmt.Printf("unique: %d paths, %d different\n", *n, *distinct)
	base := heap()
	strs := paths(*n, *distinct)
	asStrings := heap() - base
	handles := make([]unique.Handle[string], len(strs))
	for i, s := range strs {
		handles[i] = unique.Make(s)
	}
	target := strs[*distinct/2]
	ns, ts := count(strs, target, 10) // the last use of strs: the GC below may free them
	asHandles := heap() - base
	nh, th := count(handles, unique.Make(target), 10)
	fmt.Printf("  %-26s %8.1f MiB   %d equal in %v\n", "[]string", mib(asStrings), ns, ts.Round(time.Microsecond))
	fmt.Printf("  %-26s %8.1f MiB   %d equal in %v\n", "[]unique.Handle[string]", mib(asHandles), nh, th.Round(time.Microsecond))
	runtime.KeepAlive(handles)

	fmt.Printf("weak: %d texts of %d bytes in the cache, %d still held\n", *texts, *size, *keep)
	c := newCache()
	held := make([]*text, 0, *keep)
	base = heap()
	for k := range *texts {
		t := &text{s: strings.Repeat(string(rune('a'+k%26)), *size)}
		c.Put(k, t)
		if k < *keep {
			held = append(held, t)
		}
	}
	before := inUse() - base
	took, ok := settle(c, *keep, 5*time.Second)
	if !ok {
		fmt.Fprintf(os.Stderr, "weak-unique: the cache has %d entries after %v, want %d\n", c.Len(), took, *keep)
		os.Exit(1)
	}
	alive := 0
	for k := range *texts {
		if _, ok := c.Get(k); ok {
			alive++
		}
	}
	fmt.Printf("  heap after the puts, no GC %8.1f MiB\n", mib(before))
	fmt.Printf("  heap after the GC          %8.1f MiB   %d texts alive, %d entries, the cleanups took %v\n",
		mib(heap()-base), alive, c.Len(), took.Round(time.Millisecond))
	runtime.KeepAlive(held)
}
