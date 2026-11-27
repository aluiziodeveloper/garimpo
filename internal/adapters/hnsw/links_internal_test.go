package hnsw

import (
	"slices"
	"testing"
)

// The links a node gets from other inserts before it sets its own are kept (INC-13): an
// insert that reached the new node from the level above links to it on the levels below,
// while the new node is still choosing its own links there.
func TestSetLinksKeepsTheLinksOthersAdded(t *testing.T) {
	x := filled(t, Config{M: 4, EfConstruction: 32, Seed: 9}, 3)
	x.nodes[2].links[0] = nil
	x.link(2, 0, 0) // another insert links node 2 to node 0 first
	x.setLinks(2, 0, []int32{1})
	if got := x.nodes[2].links[0]; !slices.Contains(got, 0) || !slices.Contains(got, 1) {
		t.Fatalf("node 2 links to %v on level 0, want both 0 and 1", got)
	}
}

// Merged, the links still keep to the limit: a node that already has 2·M links on level 0
// and gets more keeps 2·M.
func TestSetLinksKeepsToTheLimit(t *testing.T) {
	x := filled(t, Config{M: 2, EfConstruction: 32, Seed: 10}, 12)
	x.nodes[11].links[0] = []int32{0, 1, 2, 3}
	x.setLinks(11, 0, []int32{4, 5, 6})
	if got, limit := len(x.nodes[11].links[0]), x.maxLinks(0); got != limit {
		t.Fatalf("node 11 has %d links on level 0, want %d", got, limit)
	}
}
