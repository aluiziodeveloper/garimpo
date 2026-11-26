// Package retrieval is the domain of the retrieval context (ADR 0002): what a search finds
// and how its results are ranked.
//
// Like the whole domain, it is pure and sequential. Searching an index is an adapter's
// job (internal/adapters/flatindex); this package only says what a hit is and which hit
// comes first.
package retrieval
