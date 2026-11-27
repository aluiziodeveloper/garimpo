// Package search holds what the searches of the retrieval context (ADR 0002) consume: the
// ports VectorIndex and KeywordIndex, which the indexes implement. The ports came first,
// with the contract suite of VectorIndex in indextest, so every index is held to the same
// rules from its first line; the use case of the hybrid search comes in module 15.
package search
