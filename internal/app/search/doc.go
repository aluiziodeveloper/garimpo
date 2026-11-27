// Package search holds the searches of the retrieval context (ADR 0002): the ports
// VectorIndex and KeywordIndex, which the indexes implement; Hybrid, which searches both and
// fuses their rankings (module 15); and Searcher, the use case Search, which turns a ranking
// into results a reader can open (module 16). The ports came first, with the contract suite
// of VectorIndex in indextest, so every index is held to the same rules from its first line.
package search
