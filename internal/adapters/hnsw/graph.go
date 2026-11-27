package hnsw

import (
	"cmp"
	"container/heap"
	"slices"

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

// insert links the new node n, of the given level, into the graph, starting from the
// entry point of the given top level (algorithm 1 of the paper). Other inserts walk the
// graph at the same time.
func (x *Index) insert(n int32, level int, entry int32, top int) {
	v := x.node(n).vec
	// Above the new node's level, a greedy walk: the closest node of each level is where
	// the next one starts.
	ep := []candidate{{entry, vec.Dot(v, x.node(entry).vec)}}
	for l := top; l > level; l-- {
		ep = x.searchLayer(v, ep, 1, l)
	}
	// On each level of the new node, the EfConstruction closest are the candidates for its
	// links, and the start of the level below.
	for l := min(level, top); l >= 0; l-- {
		found := x.searchLayer(v, ep, x.cfg.EfConstruction, l)
		links := x.selectNeighbors(found, x.maxLinks(l))
		x.setLinks(n, l, links)
		for _, to := range links {
			x.link(to, n, l)
		}
		ep = found
	}
}

// setLinks adds links to the links of the new node n on level l, under its lock, and past
// the limit keeps the ones the heuristic picks. It never overwrites: n is in the graph
// since Add put it in the list, and an insert that reached it from the level above may
// already have linked to it on this level (INC-13).
func (x *Index) setLinks(n int32, l int, links []int32) {
	own := x.node(n)
	own.mu.Lock()
	defer own.mu.Unlock()
	for _, to := range links {
		if !slices.Contains(own.links[l], to) {
			own.links[l] = append(own.links[l], to)
		}
	}
	x.prune(own, l)
}

// appendLinks appends to dst the links of node id on level l, copied under its lock, and
// returns it: the walk reads the copy while other inserts change the links. A walk passes
// the same buffer for every node, so a copy allocates only when the buffer has to grow.
func (x *Index) appendLinks(dst []int32, id int32, l int) []int32 {
	n := x.node(id)
	n.mu.Lock()
	defer n.mu.Unlock()
	return append(dst, n.links[l]...)
}

// link adds a link from the node from to the node to on level l and, past the limit of
// links, keeps the ones the heuristic picks among them, all under the lock of from.
func (x *Index) link(from, to int32, l int) {
	n := x.node(from)
	n.mu.Lock()
	defer n.mu.Unlock()
	n.links[l] = append(n.links[l], to)
	x.prune(n, l)
}

// prune keeps, of the links of n on level l, the ones the heuristic picks if they are past
// the limit. The caller holds the lock of n.
func (x *Index) prune(n *node, l int) {
	if len(n.links[l]) <= x.maxLinks(l) {
		return
	}
	found := make([]candidate, len(n.links[l]))
	for i, id := range n.links[l] {
		found[i] = candidate{id, vec.Dot(n.vec, x.node(id).vec)}
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
	var links []int32
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
		links = x.appendLinks(links[:0], c.id, l)
		for _, id := range links {
			if visited[id] {
				continue
			}
			visited[id] = true
			next := candidate{id, vec.Dot(q, x.node(id).vec)}
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
			if vec.Dot(x.node(c.id).vec, x.node(kept).vec) > c.score {
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
