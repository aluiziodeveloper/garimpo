// Package ingestion is the domain of the ingestion context: the documents of the corpus,
// the chunks cut from them and the ingestion run, the aggregate that tracks one pass over
// the corpus.
//
// Like the whole domain, it is pure and sequential. Reading the corpus is an adapter's
// job (internal/adapters/corpus); this package only says what a valid document is.
package ingestion
