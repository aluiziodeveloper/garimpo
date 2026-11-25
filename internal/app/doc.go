// Package app is the root of Garimpo's use cases (RunIngestion, Search, Ask, Evaluate).
// A use case orchestrates the domain and declares the ports it consumes. Adapters
// implement the ports, and cmd/garimpo wires them together.
package app
