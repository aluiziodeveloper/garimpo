package shared

import (
	"errors"
	"testing"
)

func TestNewChunkIDIsDeterministic(t *testing.T) {
	a, err := NewChunkID("sync/mutex.go", 1, 30, "package sync")
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewChunkID("sync/mutex.go", 1, 30, "package sync")
	if err != nil {
		t.Fatal(err)
	}
	if a != b {
		t.Errorf("same chunk, two IDs: %s and %s", a, b)
	}
	if got := len(a.String()); got != 64 {
		t.Errorf("len(String()) = %d, want 64 hex digits", got)
	}
}

func TestNewChunkIDDependsOnEveryField(t *testing.T) {
	base, _ := NewChunkID("sync/mutex.go", 1, 30, "package sync")
	tests := []struct {
		name       string
		path       string
		start, end int
		text       string
	}{
		{"other path", "sync/once.go", 1, 30, "package sync"},
		{"other start", "sync/mutex.go", 2, 30, "package sync"},
		{"other end", "sync/mutex.go", 1, 31, "package sync"},
		{"other text", "sync/mutex.go", 1, 30, "package sync // edited"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := NewChunkID(tt.path, tt.start, tt.end, tt.text)
			if err != nil {
				t.Fatal(err)
			}
			if id == base {
				t.Errorf("ID did not change: %s", id)
			}
		})
	}
}

func TestNewChunkIDKeepsFieldsApart(t *testing.T) {
	a, _ := NewChunkID("x1", 2, 30, "package x")
	b, _ := NewChunkID("x", 12, 30, "package x")
	if a == b {
		t.Errorf(`("x1", line 2) and ("x", line 12) share the ID %s`, a)
	}
}

func TestNewChunkIDRejectsInvalidInput(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		start, end int
	}{
		{"empty path", "", 1, 30},
		{"line zero", "sync/mutex.go", 0, 30},
		{"end before start", "sync/mutex.go", 30, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, err := NewChunkID(tt.path, tt.start, tt.end, "package sync")
			if !errors.Is(err, ErrInvalidChunkID) {
				t.Errorf("err = %v, want one that wraps ErrInvalidChunkID", err)
			}
			if !id.IsZero() {
				t.Errorf("id = %s, want the zero ChunkID", id)
			}
		})
	}
}

func TestZeroChunkID(t *testing.T) {
	var id ChunkID
	if !id.IsZero() {
		t.Error("the zero value is not IsZero")
	}
	if got := id.String(); got != "" {
		t.Errorf("String() = %q, want an empty string", got)
	}
}
