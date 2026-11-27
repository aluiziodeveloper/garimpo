// Package retrieval is the domain of the retrieval context (ADR 0002): what a search finds
// and how its results are ranked.
//
// Like the whole domain, it is pure and sequential. Searching an index is an adapter's
// job (internal/adapters/flatindex, internal/adapters/bm25); this package only says what a
// hit is, which hit comes first, and how rankings of different searches become one (Fuse).
package retrieval
