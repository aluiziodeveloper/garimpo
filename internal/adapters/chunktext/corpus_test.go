package chunktext_test

import (
	"io/fs"
	"math/rand/v2"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/aluiziodeveloper/garimpo/internal/adapters/chunktext"
	"github.com/aluiziodeveloper/garimpo/internal/adapters/corpus"
	"github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// TestStoreOnTheCorpus measures the store on the corpus in the directory GARIMPO_CORPUS
// (the Go library, /usr/local/go/src in the container); without it, it skips. It logs:
//
//   - the heap of the store, which keeps where the chunks are, against holding every text;
//   - the reads, the bytes allocated and the texts cached while 8 goroutines run 500
//     queries each: a query takes the texts of 10 chunks, drawn so that a few are much more
//     frequent than the rest, as the hits of real queries are, and holds them for 1 ms;
//   - the heap of 1,000 texts held as substrings of their files, against copies.
//
// Run it in the container, where the corpus is:
//
//	GARIMPO_CORPUS=/usr/local/go/src go test -run TestStoreOnTheCorpus -v ./internal/adapters/chunktext
func TestStoreOnTheCorpus(t *testing.T) {
	dir := os.Getenv("GARIMPO_CORPUS")
	if dir == "" {
		t.Skip("set GARIMPO_CORPUS to the corpus directory")
	}
	fsys := os.DirFS(dir)
	type spot struct {
		path           string
		offset, length int
	}
	var ids []shared.ChunkID
	var spots []spot
	textBytes := 0
	s := chunktext.New(fsys)
	base := heap()
	for d, err := range corpus.Walk(fsys) {
		if err != nil {
			t.Fatal(err)
		}
		for c := range (ingestion.Cutter{}).Cut(d) {
			if err := s.Add(c); err != nil {
				t.Fatal(err)
			}
			ids = append(ids, c.ID())
			spots = append(spots, spot{c.Path(), c.Offset(), len(c.Text())})
			textBytes += len(c.Text())
		}
	}
	t.Logf("%d chunks, %.1f MiB of text; the store, the IDs and the places: %.1f MiB of heap",
		len(ids), mib(uint64(textBytes)), mib(heap()-base))

	all := make([]*chunktext.Text, len(ids))
	base = heap()
	for i, id := range ids {
		if all[i], _ = text(t, s, id); all[i] == nil {
			return
		}
	}
	t.Logf("every text held: %.1f MiB of heap more", mib(heap()-base))
	runtime.KeepAlive(all) // until here: the measure above needs the texts alive
	settle(t, s)

	const goroutines, queries, k = 8, 500, 10
	// run runs the queries, each one taking its texts with get, and returns the bytes
	// allocated meanwhile and the most texts the store had cached at once, roughly.
	run := func(get func(i int) (*chunktext.Text, bool)) (uint64, int64) {
		allocs := totalAlloc()
		var peak atomic.Int64
		var wg sync.WaitGroup
		for g := range goroutines {
			wg.Go(func() {
				r := rand.New(rand.NewPCG(uint64(g), 14))
				zipf := rand.NewZipf(r, 1.1, 1, uint64(len(ids)-1))
				for range queries {
					held := make([]*chunktext.Text, 0, k)
					for range k {
						tx, ok := get(int(zipf.Uint64()))
						if !ok {
							return
						}
						held = append(held, tx)
					}
					time.Sleep(time.Millisecond)
					if n := int64(s.Cached()); n > peak.Load() {
						peak.Store(n) // two goroutines may race here: a rough peak
					}
					runtime.KeepAlive(held)
				}
			})
		}
		wg.Wait()
		return totalAlloc() - allocs, peak.Load()
	}
	lookups := goroutines * queries * k
	// Without the cache: every lookup reads the corpus.
	reads := s.Reads()
	fresh, _ := run(func(i int) (*chunktext.Text, bool) {
		tx, err := s.ReadUncached(ids[i])
		if err != nil {
			t.Error(err)
			return nil, false
		}
		return tx, true
	})
	t.Logf("without the cache: %d lookups, %d reads, %.1f MiB allocated", lookups, s.Reads()-reads, mib(fresh))
	reads = s.Reads()
	cached, peak := run(func(i int) (*chunktext.Text, bool) { return text(t, s, ids[i]) })
	t.Logf("with the cache:    %d lookups, %d reads, %.1f MiB allocated, about %d entries at most",
		lookups, s.Reads()-reads, mib(cached), peak)

	settle(t, s) // the texts of the queries, freed: they would shrink the heap measured below
	const held = 1000
	substring := func(i int) string {
		sp := spots[i*len(spots)/held]
		b, err := fs.ReadFile(fsys, sp.path)
		if err != nil {
			t.Fatal(err)
		}
		return string(b)[sp.offset : sp.offset+sp.length]
	}
	copies := make([]string, held)
	base = heap()
	for i := range copies {
		copies[i] = strings.Clone(substring(i))
	}
	asCopies := int64(heap()) - int64(base)
	subs := make([]string, held)
	base = heap()
	for i := range subs {
		subs[i] = substring(i)
	}
	asSubs := int64(heap()) - int64(base)
	t.Logf("%d texts held as substrings of their files: %.1f MiB of heap; as copies: %.2f MiB", held, float64(asSubs)/(1<<20), float64(asCopies)/(1<<20))
	runtime.KeepAlive(subs)
	runtime.KeepAlive(copies)
	runtime.KeepAlive(ids) // and the store: dying above, they would shrink the heap measured
	runtime.KeepAlive(s)
}

// text returns the text of id in s, or fails the test and returns false.
func text(t *testing.T, s *chunktext.Store, id shared.ChunkID) (*chunktext.Text, bool) {
	tx, err := s.Text(id)
	if err != nil {
		t.Error(err)
		return nil, false
	}
	return tx, true
}

// settle runs the GC until the store caches nothing, the texts held before freed.
func settle(t *testing.T, s *chunktext.Store) {
	t.Helper()
	for start := time.Now(); s.Cached() > 0; runtime.GC() {
		if time.Since(start) > 10*time.Second {
			t.Fatalf("%d texts still cached after 10 s", s.Cached())
		}
		time.Sleep(time.Millisecond)
	}
}

// heap returns the bytes of the heap in use after a full GC.
func heap() uint64 {
	runtime.GC()
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.HeapAlloc
}

// totalAlloc returns the bytes allocated so far, freed or not.
func totalAlloc() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.TotalAlloc
}

func mib(b uint64) float64 { return float64(b) / (1 << 20) }
