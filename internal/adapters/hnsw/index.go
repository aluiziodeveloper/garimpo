// Package hnsw is the approximate index of Garimpo: a Hierarchical Navigable Small World
// graph (Malkov and Yashunin, 2018). A search walks the graph from node to closer node and
// reads a few thousand vectors instead of all of them; it may miss some of the k best, and
// recall measures how many, against the exact flatindex.
package hnsw

import (
	"fmt"
	"math"
	"math/rand/v2"
	"sync"

	"github.com/aluiziodeveloper/garimpo/internal/app/search"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// Config holds the parameters of the graph. Zero fields take the defaults.
type Config struct {
	// M is how many links a node keeps on each level above 0; on level 0, twice as many.
	// More links: better recall, more memory, slower inserts. Default 16.
	M int
	// EfConstruction is how many candidates an insert considers on each level before it
	// picks the links. Default 200.
	EfConstruction int
	// EfSearch is how many candidates a search keeps on level 0, at least k. Default 64.
	EfSearch int
	// Seed seeds the levels of the nodes, so that a test builds the same graph twice.
	Seed uint64
}

// Index is an HNSW search.VectorIndex in memory. Every node is one vector on level 0 and,
// with probability M^-l, on every level up to l: the levels above 0 are a few long links
// that a search crosses first, from the entry point, the node of the top level.
//
// Add and Search are safe for concurrent calls: one lock guards the whole graph, which an
// Add takes alone and the searches share.
type Index struct {
	dim int
	cfg Config
	ml  float64 // 1/ln(M): the level of a node is floor(-ln(U)·ml), U uniform in (0, 1]

	mu    sync.RWMutex
	rng   *rand.Rand
	ids   []shared.ChunkID // the chunk of node i is ids[i]
	has   map[shared.ChunkID]bool
	nodes []node
	entry int32 // the node every search starts from; -1 while the index is empty
	top   int   // the level of the entry point
}

// node is a vector and its links: links[l] holds the nodes it points to on level l, so a
// node of level 2 has three lists.
type node struct {
	vec   []float32
	links [][]int32
}

// New returns an empty index for vectors of dim dimensions. It panics if dim is below 1 or
// a field of cfg is negative.
func New(dim int, cfg Config) *Index {
	if dim < 1 || cfg.M < 0 || cfg.EfConstruction < 0 || cfg.EfSearch < 0 {
		panic(fmt.Sprintf("hnsw.New: dimension %d, %+v", dim, cfg))
	}
	if cfg.M == 0 {
		cfg.M = 16
	}
	if cfg.EfConstruction == 0 {
		cfg.EfConstruction = 200
	}
	if cfg.EfSearch == 0 {
		cfg.EfSearch = 64
	}
	return &Index{
		dim: dim, cfg: cfg, ml: 1 / math.Log(float64(max(cfg.M, 2))),
		rng: rand.New(rand.NewPCG(cfg.Seed, cfg.Seed)),
		has: make(map[shared.ChunkID]bool), entry: -1,
	}
}

// Add keeps v as the vector of the chunk id: it becomes a node of the graph, linked to the
// closest nodes it finds on each of its levels.
func (x *Index) Add(id shared.ChunkID, v shared.Vector) error {
	switch {
	case id.IsZero():
		return search.ErrZeroChunk
	case v.Dim() != x.dim:
		return fmt.Errorf("%w: vector of %d, the index has %d", search.ErrWrongDimension, v.Dim(), x.dim)
	}
	values := v.Values()
	x.mu.Lock()
	defer x.mu.Unlock()
	if x.has[id] {
		return fmt.Errorf("%w: %s", search.ErrDuplicateChunk, id)
	}
	x.has[id] = true
	x.insert(id, values)
	return nil
}

// Len returns how many vectors the index keeps.
func (x *Index) Len() int {
	x.mu.RLock()
	defer x.mu.RUnlock()
	return len(x.ids)
}

// randomLevel draws the level of a new node: 0 with probability 1 - 1/M, and each level
// above with 1/M of the one below.
func (x *Index) randomLevel() int {
	return int(math.Floor(-math.Log(1-x.rng.Float64()) * x.ml))
}

// maxLinks is how many links a node keeps on level l.
func (x *Index) maxLinks(l int) int {
	if l == 0 {
		return 2 * x.cfg.M
	}
	return x.cfg.M
}
