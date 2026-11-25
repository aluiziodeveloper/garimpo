// Package shared is the shared kernel of Garimpo's domain: the value objects that every
// bounded context uses (ChunkID, Vector and Citation).
//
// The domain is pure and sequential: no goroutines, no I/O, no global clock. The lint
// enforces it with depguard and forbidigo (.golangci.yml, ADR 0002).
package shared
