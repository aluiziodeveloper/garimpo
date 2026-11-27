package flatindex

import (
	"math/rand/v2"
	"testing"

	"github.com/aluiziodeveloper/garimpo/internal/app/search/indextest"
)

// The shard of a chunk comes from a hash of its ID, so the shards fill evenly. The seed of
// the hash is new in every process and cannot be fixed (hash/maphash): with 4000 chunks in
// 4 shards, a shard gets 1000 ± 27 (one standard deviation), and the band of 800 to 1200
// is 7 deviations wide on each side: an even spread falls outside it far less than once in
// a billion runs. Every chunk in one shard still fails it.
func TestShardedSpreadsTheVectors(t *testing.T) {
	x := NewSharded(4, 4)
	r := rand.New(rand.NewPCG(1, 1))
	for i := range 4000 {
		if err := x.Add(indextest.Chunk(t, i), indextest.Vector(t, r, 4)); err != nil {
			t.Fatal(err)
		}
	}
	for i, s := range x.shards {
		if n := s.Len(); n < 800 || n > 1200 {
			t.Errorf("shard %d has %d of 4000 vectors, want about 1000", i, n)
		}
	}
}
