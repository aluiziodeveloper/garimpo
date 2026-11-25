package main

import (
	"strings"
	"testing"
	"time"
)

func TestParseThreads(t *testing.T) {
	tests := []struct {
		name    string
		status  string
		want    int
		wantErr bool
	}{
		{"linux status", "Name:\tthreads\nState:\tS (sleeping)\nThreads:\t7\nVmRSS:\t2048 kB\n", 7, false},
		{"no Threads line", "Name:\tthreads\n", 0, true},
		{"not a number", "Threads:\tmany\n", 0, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseThreads(strings.NewReader(tt.status))
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %t", err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("parseThreads() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestWaitingOnAChannelTakesNoThread(t *testing.T) {
	s, err := measure(1000, "chan", 0, 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if s.Threads > 50 {
		t.Errorf("1000 goroutines waiting on a channel used %d threads, want at most 50", s.Threads)
	}
}

func TestLimitBoundsTheThreadsOfBlockingCalls(t *testing.T) {
	const limit = 20
	s, err := measure(200, "syscall", limit, 50*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if s.Threads > limit+50 {
		t.Errorf("limit %d: %d threads, want at most %d", limit, s.Threads, limit+50)
	}
}

func TestUnknownWait(t *testing.T) {
	if _, err := measure(1, "disk", 0, time.Millisecond); err == nil {
		t.Error(`measure with wait "disk": err = nil, want an error`)
	}
}
