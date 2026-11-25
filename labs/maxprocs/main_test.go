package main

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestWorkersFollowsGOMAXPROCS(t *testing.T) {
	// A value no machine reports as NumCPU by accident, so sizing the pool
	// by runtime.NumCPU fails here.
	const procs = 3
	if runtime.NumCPU() == procs {
		t.Skipf("NumCPU is %d; the test needs a different value", procs)
	}
	prev := runtime.GOMAXPROCS(procs)
	t.Cleanup(func() { runtime.GOMAXPROCS(prev) })

	if got := workers(); got != procs {
		t.Fatalf("workers() = %d, want %d (GOMAXPROCS, not NumCPU = %d)", got, procs, runtime.NumCPU())
	}
}

func TestReadThrottled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cpu.stat")
	stat := "usage_usec 4100000\nnr_periods 41\nnr_throttled 39\nthrottled_usec 65505000\n"
	if err := os.WriteFile(path, []byte(stat), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := readThrottled(path)
	if err != nil {
		t.Fatal(err)
	}
	want := throttled{periods: 39, time: 65505 * time.Millisecond}
	if got != want {
		t.Fatalf("readThrottled() = %+v, want %+v", got, want)
	}
}

func TestReadThrottledMissingFile(t *testing.T) {
	if _, err := readThrottled(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("readThrottled() on a missing file: want an error")
	}
}

func TestRunRejectsUnknownPool(t *testing.T) {
	err := run(io.Discard, "threads", 1)
	if err == nil || !strings.Contains(err.Error(), `unknown pool "threads"`) {
		t.Fatalf("run() error = %v, want unknown pool", err)
	}
}

func TestReadQuota(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantCPUs    float64
		wantLimited bool
		wantErr     bool
	}{
		{name: "two CPUs", content: "200000 100000\n", wantCPUs: 2, wantLimited: true},
		{name: "fraction", content: "150000 100000\n", wantCPUs: 1.5, wantLimited: true},
		{name: "no quota", content: "max 100000\n", wantLimited: false},
		{name: "one field", content: "200000\n", wantErr: true},
		{name: "not a number", content: "lots 100000\n", wantErr: true},
		{name: "zero period", content: "200000 0\n", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "cpu.max")
			if err := os.WriteFile(path, []byte(tt.content), 0o600); err != nil {
				t.Fatal(err)
			}

			cpus, limited, err := readQuota(path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("readQuota(%q) = %v, %v, want an error", tt.content, cpus, limited)
				}
				return
			}
			if err != nil {
				t.Fatalf("readQuota(%q) unexpected error: %v", tt.content, err)
			}
			if cpus != tt.wantCPUs || limited != tt.wantLimited {
				t.Fatalf("readQuota(%q) = %v, %v, want %v, %v", tt.content, cpus, limited, tt.wantCPUs, tt.wantLimited)
			}
		})
	}
}

func TestReadQuotaMissingFile(t *testing.T) {
	if _, _, err := readQuota(filepath.Join(t.TempDir(), "absent")); err == nil {
		t.Fatal("readQuota() on a missing file: want an error")
	}
}
