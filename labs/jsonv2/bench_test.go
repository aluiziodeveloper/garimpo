package main

import (
	"io"
	"testing"
)

// BenchmarkEncode writes the response of a search the way the server does, to a writer.
func BenchmarkEncode(b *testing.B) {
	r, err := corpusResponse()
	if err != nil {
		b.Fatal(err)
	}
	for _, e := range []struct {
		name   string
		encode func(io.Writer, Response) error
	}{{"enc=v1", encodeV1}, {"enc=v2", encodeV2}} {
		b.Run(e.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if err := e.encode(io.Discard, r); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
