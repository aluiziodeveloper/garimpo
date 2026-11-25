package main

import (
	"testing"
	"time"
)

func TestCPUWorkersHashUntilTheDeadline(t *testing.T) {
	start := time.Now()
	n, err := run(4, "cpu", 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if n == 0 {
		t.Error("4 CPU workers for 50ms computed 0 hashes")
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("run took %v, want about 50ms", d)
	}
}

func TestWaitWorkersComputeNothing(t *testing.T) {
	n, err := run(4, "wait", 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("wait workers computed %d hashes, want 0", n)
	}
}

func TestUnknownKind(t *testing.T) {
	if _, err := run(1, "gpu", time.Millisecond); err == nil {
		t.Error(`run with kind "gpu": err = nil, want an error`)
	}
}
