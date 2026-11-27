package hnsw_test

import (
	"fmt"
	"testing"

	"github.com/aluiziodeveloper/garimpo/internal/adapters/hnsw"
	"github.com/aluiziodeveloper/garimpo/internal/app/search"
	"github.com/aluiziodeveloper/garimpo/internal/app/search/indextest"
)

// minRecall is the recall the contract asks of the graph in each subtest: the 0.95 of
// RNF04.
const minRecall = 0.95

// The contract runs with the default EfSearch, 64, and with 1, below every k it asks for: a
// search keeps at least k candidates, whatever EfSearch says.
func TestIndexMeetsTheApproximateContract(t *testing.T) {
	for _, ef := range []int{64, 1} {
		cfg := hnsw.Config{EfSearch: ef, Seed: 1}
		t.Run(fmt.Sprint("EfSearch=", ef), func(t *testing.T) {
			indextest.ApproximateContract(t, minRecall, func(_ *testing.T, dim int) search.VectorIndex {
				return hnsw.New(dim, cfg)
			})
		})
	}
}
