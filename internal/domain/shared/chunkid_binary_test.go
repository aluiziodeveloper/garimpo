package shared

import (
	"errors"
	"testing"
)

// A ChunkID goes into a file as its 32 bytes, and comes back the same.
func TestAChunkIDComesBackFromItsBytes(t *testing.T) {
	id, err := NewChunkID("sort/sort.go", 0, 1, 9, "package sort")
	if err != nil {
		t.Fatal(err)
	}
	b, err := id.AppendBinary([]byte("prefix"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) != len("prefix")+ChunkIDSize || string(b[:6]) != "prefix" {
		t.Fatalf("AppendBinary gave %d bytes, want the prefix and %d", len(b), ChunkIDSize)
	}
	var back ChunkID
	if err := back.UnmarshalBinary(b[6:]); err != nil {
		t.Fatal(err)
	}
	if back != id {
		t.Errorf("UnmarshalBinary gave %s, want %s", back, id)
	}
}

func TestTheBytesOfAChunkIDRefuseWhatIsNoChunk(t *testing.T) {
	if _, err := (ChunkID{}).AppendBinary(nil); !errors.Is(err, ErrInvalidChunkID) {
		t.Errorf("AppendBinary of the zero ChunkID: err = %v, want %v", err, ErrInvalidChunkID)
	}
	var id ChunkID
	for _, data := range [][]byte{nil, make([]byte, ChunkIDSize-1), make([]byte, ChunkIDSize)} {
		if err := id.UnmarshalBinary(data); !errors.Is(err, ErrInvalidChunkID) {
			t.Errorf("UnmarshalBinary of %d bytes %v: err = %v, want %v", len(data), data, err, ErrInvalidChunkID)
		}
	}
}
