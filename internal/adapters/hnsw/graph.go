package hnsw

import (
	"cmp"
	"container/heap"
	"slices"

	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
	"github.com/aluiziodeveloper/garimpo/internal/topk"
	"github.com/aluiziodeveloper/garimpo/internal/vec"
)

// candidate is a node and its score against the vector being searched for or inserted.
type candidate struct {
	id    int32
	score float32
}

// closer ranks the higher score first and, between equal scores, the older node: the
// order of every list of candidates, so that the same graph gives the same answer.
func closer(a, b candidate) int {
	if c := cmp.Compare(b.score, a.score); c != 0 {
		return c
	}
	return cmp.Compare(a.id, b.id)
}

// insert makes vec the vector of a new node of the chunk id and links it into the graph
// (algorithm 1 of the paper). The caller holds the lock alone.
func (x *Index) insert(id shared.ChunkID, v []float32) {
	n := int32(len(x.nodes))
	level := x.randomLevel()
	x.ids = append(x.ids, id)
	x.nodes = append(x.nodes, node{vec: v, links: make([][]int32, level+1)})
	if x.entry < 0 {
		x.entry, x.top = n, level
		return
	}
	// Above the new node's level, a greedy walk: the closest node of each level is where
	// the next one starts.
	ep := []candidate{{x.entry, vec.Dot(v, x.nodes[x.entry].vec)}}
	for l := x.top; l > level; l-- {
		ep = x.searchLayer(v, ep, 1, l)
	}
	// On each level of the new node, the EfConstruction closest are the candidates for its
	// links, and the start of the level below.
	for l := min(level, x.top); l >= 0; l-- {
		found := x.searchLayer(v, ep, x.cfg.EfConstruction, l)
		links := x.selectNeighbors(found, x.maxLinks(l))
		x.nodes[n].links[l] = links
		for _, to := range links {
			x.link(to, n, l)
		}
		ep = found
	}
	if level > x.top {
		x.entry, x.top = n, level
	}
}

// link adds a link from the node from to the node to on level l and, past the limit of
// links, keeps the ones the heuristic picks among them.
func (x *Index) link(from, to int32, l int) {
	n := &x.nodes[from]
	n.links[l] = append(n.links[l], to)
	if len(n.links[l]) <= x.maxLinks(l) {
		return
	}
	found := make([]candidate, len(n.links[l]))
	for i, id := range n.links[l] {
		found[i] = candidate{id, vec.Dot(n.vec, x.nodes[id].vec)}
	}
	slices.SortFunc(found, closer)
	n.links[l] = x.selectNeighbors(found, x.maxLinks(l))
}

// searchLayer returns the ef nodes closest to q that it finds on level l, starting from
// ep, closest first (algorithm 2 of the paper). It walks from the closest candidate not yet
// expanded, and stops when that candidate is farther than the farthest of the ef found.
func (x *Index) searchLayer(q []float32, ep []candidate, ef, l int) []candidate {
	visited := make(map[int32]bool, ef*4)
	queue := &bestFirst{}
	found := topk.New(ef, closer)
	for _, c := range ep {
		visited[c.id] = true
		queue.push(c)
		found.Offer(c)
	}
	for queue.Len() > 0 {
		c := queue.pop()
		if worst, _ := found.Worst(); found.Len() == ef && closer(c, worst) > 0 {
			break
		}
		for _, id := range x.nodes[c.id].links[l] {
			if visited[id] {
				continue
			}
			visited[id] = true
			next := candidate{id, vec.Dot(q, x.nodes[id].vec)}
			if found.Offer(next) {
				queue.push(next)
			}
		}
	}
	return found.Sorted()
}

// selectNeighbors picks at most limit links among found, closest first (algorithm 4 of the
// paper): a candidate is kept only if it is closer to the base than to every link kept so
// far, so the links point in different directions instead of into one cluster. If that
// keeps fewer than limit, the closest of the ones left out fill the rest.
func (x *Index) selectNeighbors(found []candidate, limit int) []int32 {
	links := make([]int32, 0, limit)
	skipped := make([]int32, 0, len(found))
	for _, c := range found {
		if len(links) == limit {
			break
		}
		diverse := true
		for _, kept := range links {
			if vec.Dot(x.nodes[c.id].vec, x.nodes[kept].vec) > c.score {
				diverse = false
				break
			}
		}
		if diverse {
			links = append(links, c.id)
		} else {
			skipped = append(skipped, c.id)
		}
	}
	for _, id := range skipped {
		if len(links) == limit {
			break
		}
		links = append(links, id)
	}
	return links
}

// bestFirst is a heap.Interface of candidates with the closest at the root: the queue of
// the nodes a search has still to expand. push and pop go through heap.Fix, never
// heap.Push and heap.Pop, so a candidate never passes through an any (module 12).
type bestFirst struct{ s []candidate }

func (h *bestFirst) Len() int           { return len(h.s) }
func (h *bestFirst) Less(i, j int) bool { return closer(h.s[i], h.s[j]) < 0 }
func (h *bestFirst) Swap(i, j int)      { h.s[i], h.s[j] = h.s[j], h.s[i] }

// Push and Pop complete heap.Interface; bestFirst never calls heap.Push or heap.Pop.
func (h *bestFirst) Push(x any) { h.s = append(h.s, x.(candidate)) }

func (h *bestFirst) Pop() any {
	x := h.s[len(h.s)-1]
	h.s = h.s[:len(h.s)-1]
	return x
}

func (h *bestFirst) push(c candidate) {
	h.s = append(h.s, c)
	heap.Fix(h, len(h.s)-1)
}

func (h *bestFirst) pop() candidate {
	c := h.s[0]
	last := len(h.s) - 1
	h.s[0] = h.s[last]
	h.s = h.s[:last]
	if last > 0 {
		heap.Fix(h, 0)
	}
	return c
}
