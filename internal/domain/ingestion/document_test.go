package ingestion

import (
	"errors"
	"testing"
)

func TestNewDocument(t *testing.T) {
	d, err := NewDocument("strings/builder.go", "package strings\n")
	if err != nil {
		t.Fatal(err)
	}
	if d.Path() != "strings/builder.go" || d.Text() != "package strings\n" {
		t.Errorf("NewDocument() = %q, %q", d.Path(), d.Text())
	}
}

func TestNewDocumentRejects(t *testing.T) {
	tests := []struct {
		name, path, text string
	}{
		{name: "empty path", path: "", text: "package x\n"},
		{name: "absolute path", path: "/usr/local/go/src/x.go", text: "package x\n"},
		{name: "invalid UTF-8", path: "x.go", text: "package x\n\xff\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewDocument(tt.path, tt.text)
			if !errors.Is(err, ErrInvalidDocument) {
				t.Errorf("NewDocument(%q, %q) error = %v, want ErrInvalidDocument", tt.path, tt.text, err)
			}
		})
	}
}
