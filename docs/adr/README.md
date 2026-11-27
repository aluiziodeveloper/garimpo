# Architecture decision records

One file per decision, numbered in order. A decision is never edited away: when it changes, a
new record supersedes it and the old one says so in its status.

| ADR | Decision | Status |
|---|---|---|
| [0001](0001-toolchain-and-workflow.md) | Toolchain, development environment and workflow | Accepted |
| [0002](0002-bounded-contexts-and-layers.md) | Bounded contexts, layers and the shared kernel | Accepted |
| [0003](0003-vector-index-port-and-exact-index.md) | The VectorIndex port and the exact index | Accepted; the `heap.Init` detail of `topk` superseded by 0004 |
| [0004](0004-concurrent-hnsw-index.md) | The concurrent HNSW index | Accepted |
| [0005](0005-snapshot-reads-and-weak-texts.md) | Searches that read snapshots, and texts held by weak pointers | Accepted |
| [0006](0006-hybrid-search.md) | Hybrid search, BM25 and Reciprocal Rank Fusion | Accepted |
