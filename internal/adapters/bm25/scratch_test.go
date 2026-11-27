package bm25_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/aluiziodeveloper/garimpo/internal/adapters/bm25"
)

// A search allocates the same few times however many chunks hold its terms: what it
// computes the scores in is reused from one search to the next. Under -race, sync.Pool
// drops a Put in four on purpose, so this test runs without it.
func TestASearchAllocatesAlikeForAFewChunksOrMany(t *testing.T) {
	if raceEnabled {
		t.Skip("sync.Pool drops a Put in four at random under -race")
	}
	allocs := func(chunks int) float64 {
		x := bm25.New()
		for i := range chunks {
			if err := x.Add(chunk(t, i), fmt.Sprint("func f", i, " returns error")); err != nil {
				t.Fatal(err)
			}
		}
		find(t, x, "returns error", 10) // the first search makes what the next ones reuse
		return testing.AllocsPerRun(100, func() { find(t, x, "returns error", 10) })
	}
	few, many := allocs(10), allocs(10_000)
	if many > few {
		t.Errorf("a search allocates %v times over 10 chunks and %v over 10,000, want no more", few, many)
	}
}

// What one search leaves in the reused buffers never reaches the next: searches in any
// order, and after Adds that make the index longer, find what they find on a fresh index.
func TestASearchIsNotChangedByTheOnesBefore(t *testing.T) {
	texts := []string{"lock unlock mutex", "mutex", "channel select", "select lock", "goroutine channel"}
	fresh := func(query string) []int {
		return chunks(t, find(t, filled(t, texts...), query, 10), len(texts))
	}
	x := filled(t, texts[:3]...)
	find(t, x, "lock mutex channel select goroutine", 10)
	for i := 3; i < len(texts); i++ {
		if err := x.Add(chunk(t, i), texts[i]); err != nil {
			t.Fatal(err)
		}
	}
	for _, q := range []string{"lock", "channel select", "mutex", "goroutine lock", "lock"} {
		if got, want := chunks(t, find(t, x, q, 10), len(texts)), fresh(q); !slices.Equal(got, want) {
			t.Errorf("chunks for %q = %v after other searches, want %v as on a fresh index", q, got, want)
		}
	}
}
