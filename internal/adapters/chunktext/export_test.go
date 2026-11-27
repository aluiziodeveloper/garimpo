package chunktext

import "github.com/aluiziodeveloper/garimpo/internal/domain/shared"

// ReadUncached reads the text of id from the corpus as Text does on a miss, and caches
// nothing: what the store would cost without its cache, for the measures.
func (s *Store) ReadUncached(id shared.ChunkID) (*Text, error) {
	s.mu.Lock()
	p, ok := s.places[id]
	s.mu.Unlock()
	if !ok {
		return nil, ErrUnknownChunk
	}
	return s.read(id, p)
}
