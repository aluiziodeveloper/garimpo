package main

import (
	"slices"
	"testing"
)

func TestChunkWords(t *testing.T) {
	tests := []struct {
		name string
		text string
		size int
		want []string
	}{
		{name: "empty text", text: "", size: 3, want: nil},
		{name: "exact multiple", text: "a b c d", size: 2, want: []string{"a b", "c d"}},
		{name: "remainder in last chunk", text: "a b c d e", size: 2, want: []string{"a b", "c d", "e"}},
		{name: "size larger than text", text: "a b", size: 10, want: []string{"a b"}},
		{name: "extra spaces", text: "  a   b\n c ", size: 2, want: []string{"a b", "c"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := chunkWords(tt.text, tt.size)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("chunkWords(%q, %d) = %q, want %q", tt.text, tt.size, got, tt.want)
			}
		})
	}
}

func TestChunkWordsRejectsSizeBelowOne(t *testing.T) {
	for _, size := range []int{0, -1} {
		if _, err := chunkWords("a b", size); err == nil {
			t.Errorf("size %d: no error, want one", size)
		}
	}
}
