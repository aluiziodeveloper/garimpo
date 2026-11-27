// Package search holds the searches of the retrieval context (ADR 0002): the ports
// VectorIndex and KeywordIndex, which the indexes implement, and Hybrid, which searches both
// and fuses their rankings (module 15). The ports came first, with the contract suite of
// VectorIndex in indextest, so every index is held to the same rules from its first line;
// the use case Search, which serves the queries, comes in module 16.
package search
