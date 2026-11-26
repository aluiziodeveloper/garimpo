package flatindex

import (
	"math/rand/v2"
	"testing"

	"github.com/aluiziodeveloper/garimpo/internal/app/search/indextest"
)

// The shard of a chunk comes from a hash of its ID, so the shards fill evenly.
func TestShardedSpreadsTheVectors(t *testing.T) {
	x := NewSharded(4, 4)
	r := rand.New(rand.NewPCG(1, 1))
	for i := range 4000 {
		if err := x.Add(indextest.Chunk(t, i), indextest.Vector(t, r, 4)); err != nil {
			t.Fatal(err)
		}
	}
	for i, s := range x.shards {
		if n := s.Len(); n < 900 || n > 1100 {
			t.Errorf("shard %d has %d of 4000 vectors, want about 1000", i, n)
		}
	}
}
