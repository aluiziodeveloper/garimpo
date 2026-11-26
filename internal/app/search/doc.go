// Package search holds what the searches of the retrieval context (ADR 0002) consume: the
// port VectorIndex, which the indexes implement. The use case Search comes in module 16;
// the port comes first, with the contract suite in indextest, so every index is held to
// the same rules from its first line.
package search
