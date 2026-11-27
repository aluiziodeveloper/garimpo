package hnsw

import (
	"fmt"
	"math/rand/v2"
	"testing"

	"github.com/aluiziodeveloper/garimpo/internal/app/search/indextest"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// BenchmarkBuild measures the time to build a graph of 2000 random vectors of 384
// dimensions, the dimension of all-minilm, with 1 to 8 goroutines calling Add at once.
// Run it with as many CPUs as goroutines:
//
//	BENCH_CPUSET=2,4,6,8,10,12,14,16 docker compose run --rm bench go test -run '^$' -bench Build ./internal/adapters/hnsw
func BenchmarkBuild(b *testing.B) {
	r := rand.New(rand.NewPCG(8, 8))
	vectors := make([]shared.Vector, 2000)
	for i := range vectors {
		vectors[i] = indextest.Vector(b, r, 384)
	}
	for _, workers := range []int{1, 2, 4, 8} {
		b.Run(fmt.Sprint("workers=", workers), func(b *testing.B) {
			for b.Loop() {
				build(b, Config{Seed: 8}, vectors, workers)
			}
		})
	}
}
