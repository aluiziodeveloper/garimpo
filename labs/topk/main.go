// Program topk picks the k best of n scores in three ways, sorting them all, keeping a
// heap of k and keeping a sorted slice of k, and prints how many comparisons each one
// made. The scores come in random order or in ascending order, the worst case of the
// last two: every score is better than the ones kept.
//
//	go run ./labs/topk -n 41980 -k 10,100,1000
package main

import (
	"cmp"
	"container/heap"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"strings"
)

// counter counts the comparisons of a selector.
type counter struct{ n int }

// better reports whether score a ranks before score b, and counts the call.
func (c *counter) better(a, b float32) bool {
	c.n++
	return a > b
}

// bySort sorts every score and keeps the first k: n·log₂(n) comparisons, whatever k is.
func bySort(scores []float32, k int, c *counter) []float32 {
	all := slices.Clone(scores)
	slices.SortFunc(all, func(a, b float32) int {
		c.n++
		return cmp.Compare(b, a)
	})
	return all[:min(k, len(all))]
}

// minHeap holds the k best scores so far, the worst of them at the root.
type minHeap struct {
	s []float32
	c *counter
}

func (h *minHeap) Len() int           { return len(h.s) }
func (h *minHeap) Less(i, j int) bool { return h.c.better(h.s[j], h.s[i]) }
func (h *minHeap) Swap(i, j int)      { h.s[i], h.s[j] = h.s[j], h.s[i] }
func (h *minHeap) Push(x any)         { h.s = append(h.s, x.(float32)) }
func (h *minHeap) Pop() any {
	x := h.s[len(h.s)-1]
	h.s = h.s[:len(h.s)-1]
	return x
}

// byHeap keeps a heap of k: one comparison with the root per score and, when the score
// is better, log₂(k) more to put it in place.
func byHeap(scores []float32, k int, c *counter) []float32 {
	if k < 1 {
		return nil
	}
	h := &minHeap{s: make([]float32, 0, k), c: c}
	for _, s := range scores {
		switch {
		case h.Len() < k:
			heap.Push(h, s)
		case c.better(s, h.s[0]):
			h.s[0] = s
			heap.Fix(h, 0)
		}
	}
	out := make([]float32, h.Len())
	for i := len(out) - 1; i >= 0; i-- {
		out[i] = heap.Pop(h).(float32)
	}
	return out
}

// byInsertion keeps the k best in a sorted slice: one comparison with the last per score
// and, when the score is better, up to k more to find its place.
func byInsertion(scores []float32, k int, c *counter) []float32 {
	if k < 1 {
		return nil
	}
	kept := make([]float32, 0, k)
	for _, s := range scores {
		if len(kept) == k && !c.better(s, kept[k-1]) {
			continue
		}
		i := len(kept)
		for i > 0 && c.better(s, kept[i-1]) {
			i--
		}
		if len(kept) < k {
			kept = append(kept, 0)
		}
		copy(kept[i+1:], kept[i:len(kept)-1])
		kept[i] = s
	}
	return kept
}

// selectors are the three ways, in the order the table prints them.
var selectors = []struct {
	name string
	pick func([]float32, int, *counter) []float32
}{
	{"sort", bySort},
	{"heap", byHeap},
	{"insertion", byInsertion},
}

// scoresOf returns n scores between -1 and 1, at random or in ascending order.
func scoresOf(n int, order string, seed uint64) ([]float32, error) {
	r := rand.New(rand.NewPCG(seed, seed))
	s := make([]float32, n)
	for i := range s {
		s[i] = r.Float32()*2 - 1
	}
	switch order {
	case "random":
	case "ascending":
		slices.Sort(s)
	default:
		return nil, fmt.Errorf("order %q: want random or ascending", order)
	}
	return s, nil
}

func main() {
	n := flag.Int("n", 41_980, "number of scores, as many as the chunks of the corpus")
	ks := flag.String("k", "10,100,1000", "comma-separated values of k")
	seed := flag.Uint64("seed", 1, "seed of the scores")
	flag.Parse()

	for _, order := range []string{"random", "ascending"} {
		scores, err := scoresOf(*n, order, *seed)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(2)
		}
		fmt.Printf("%d scores, %s order: comparisons\n", *n, order)
		fmt.Printf("%-6s %-10s %-10s %-10s\n", "k", "sort", "heap", "insertion")
		for f := range strings.SplitSeq(*ks, ",") {
			k, err := strconv.Atoi(f)
			if err != nil || k < 1 {
				fmt.Fprintf(os.Stderr, "k %q: want a number above 0\n", f)
				os.Exit(2)
			}
			fmt.Printf("%-6d", k)
			var want []float32
			for _, sel := range selectors {
				var c counter
				got := sel.pick(scores, k, &c)
				if want == nil {
					want = got
				} else if !slices.Equal(got, want) {
					fmt.Fprintf(os.Stderr, "%s disagrees with sort for k=%d\n", sel.name, k)
					os.Exit(1)
				}
				fmt.Printf(" %-10d", c.n)
			}
			fmt.Println()
		}
	}
}
