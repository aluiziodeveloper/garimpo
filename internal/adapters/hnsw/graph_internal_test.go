package hnsw

import (
	"math"
	"math/rand/v2"
	"testing"

	"github.com/aluiziodeveloper/garimpo/internal/app/search/indextest"
)

// The levels thin out by M: a node reaches level l with probability M^-l, so with M of 16
// one node in 16 has a level 1 and one in 256 a level 2.
func TestLevelsThinOutByM(t *testing.T) {
	x := New(4, Config{M: 16, Seed: 1})
	const draws = 100_000
	counts := make([]int, 8)
	for range draws {
		l := x.randomLevel()
		for i := 0; i <= l && i < len(counts); i++ {
			counts[i]++
		}
	}
	for l, want := range []float64{draws, draws / 16, draws / 256} {
		// Three standard deviations of a binomial: the test does not fail by chance.
		p := want / draws
		if got, tol := float64(counts[l]), 3*math.Sqrt(draws*p*(1-p)); math.Abs(got-want) > tol {
			t.Errorf("%v of %d draws reached level %d, want %v ± %.0f", got, draws, l, want, tol)
		}
	}
}

// filled returns an index with n random vectors of 13 dimensions, added one at a time.
func filled(t *testing.T, cfg Config, n int) *Index {
	const dim = 13
	t.Helper()
	x := New(dim, cfg)
	r := rand.New(rand.NewPCG(2, 2))
	for i := range n {
		if err := x.Add(indextest.Chunk(t, i), indextest.Vector(t, r, dim)); err != nil {
			t.Fatal(err)
		}
	}
	return x
}

// A node keeps at most M links on each level above 0 and 2·M on level 0, never a link to
// itself and never the same link twice: the pruning keeps the graph sparse.
func TestNoNodeHasMoreLinksThanItMay(t *testing.T) {
	x := filled(t, Config{M: 4, EfConstruction: 32, Seed: 3}, 2000)
	for id, n := range x.nodes {
		for l, links := range n.links {
			if limit := x.maxLinks(l); len(links) > limit {
				t.Errorf("node %d has %d links on level %d, want at most %d", id, len(links), l, limit)
			}
			seen := map[int32]bool{}
			for _, to := range links {
				if to == int32(id) || seen[to] {
					t.Errorf("node %d links to %d twice or to itself on level %d", id, to, l)
				}
				seen[to] = true
			}
		}
	}
}

// Every node can be reached from the entry point on level 0, the one that holds them all:
// a node no link points to is a vector no search can find.
func TestEveryNodeCanBeReached(t *testing.T) {
	x := filled(t, Config{M: 4, EfConstruction: 32, Seed: 4}, 2000)
	seen := make([]bool, len(x.nodes))
	seen[x.entry] = true
	queue := []int32{x.entry}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		for _, to := range x.nodes[id].links[0] {
			if !seen[to] {
				seen[to] = true
				queue = append(queue, to)
			}
		}
	}
	for id, ok := range seen {
		if !ok {
			t.Fatalf("node %d of %d cannot be reached from the entry point", id, len(seen))
		}
	}
}

// The entry point is a node of the top level, and no node goes above it: every search
// starts at the top.
func TestTheEntryPointIsOnTheTopLevel(t *testing.T) {
	x := filled(t, Config{M: 4, EfConstruction: 32, Seed: 5}, 2000)
	if top := len(x.nodes[x.entry].links) - 1; top != x.top {
		t.Fatalf("the entry point has level %d, the index says %d", top, x.top)
	}
	for id, n := range x.nodes {
		if len(n.links)-1 > x.top {
			t.Errorf("node %d has level %d, above the top %d", id, len(n.links)-1, x.top)
		}
	}
}
