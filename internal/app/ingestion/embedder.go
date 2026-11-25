package ingestion

import (
	"context"
	"errors"

	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// ErrBrokenEmbedder is wrapped when an Embedder breaks its contract, such as returning a
// number of vectors other than the number of texts. It is a bug in the adapter: the run
// fails instead of recording a wrong vector.
var ErrBrokenEmbedder = errors.New("embedder broke its contract")

// Embedder turns texts into vectors: the port to the embedding model, in Garimpo's words.
// The adapter translates the model's API, which never reaches the use case (anticorruption
// layer, ADR 0002).
//
// Embed returns one vector per text, in the order of texts, all of one dimension. Empty
// texts return no vectors and no error. When ctx is done, Embed returns an error that
// wraps ctx's error. An error fails every text of the call. Embed is safe for concurrent
// calls. embeddertest.Contract checks all of this.
type Embedder interface {
	Embed(ctx context.Context, texts []string) ([]shared.Vector, error)
}
