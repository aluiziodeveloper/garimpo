package main

import (
	"bytes"
	"testing"
)

func TestRunWritesATrace(t *testing.T) {
	var buf bytes.Buffer
	n, err := run(&buf, 4, 40)
	if err != nil {
		t.Fatal(err)
	}
	if n != 40 {
		t.Errorf("run() hashed %d jobs, want 40", n)
	}
	// A trace file starts with a header such as "go 1.26 trace".
	if !bytes.HasPrefix(buf.Bytes(), []byte("go 1.")) {
		t.Errorf("trace starts with %q, want the header %q", buf.Bytes()[:min(buf.Len(), 12)], "go 1.")
	}
}

func TestRunNeedsWorkers(t *testing.T) {
	var buf bytes.Buffer
	if _, err := run(&buf, 0, 10); err == nil {
		t.Error("run with 0 workers: err = nil, want an error")
	}
}
