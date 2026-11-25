package main

import (
	"testing"
	"time"
)

func TestTheSleeperWakesWhileTheLoopSpins(t *testing.T) {
	r, err := run("spin", 2_000_000_000, 10*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if r.Woke > 100*time.Millisecond {
		t.Errorf("asked to sleep 10ms beside a spinning loop, woke after %v", r.Woke)
	}
	if r.Loop > 150*time.Millisecond {
		t.Errorf("the loop ran %v after the sleeper asked it to stop", r.Loop)
	}
}

func TestCallLoopsStopToo(t *testing.T) {
	for _, loop := range []string{"leaf", "call"} {
		r, err := run(loop, 2_000_000_000, 10*time.Millisecond)
		if err != nil {
			t.Fatal(err)
		}
		if r.Woke > 100*time.Millisecond {
			t.Errorf("loop %s: woke after %v, want about 10ms", loop, r.Woke)
		}
	}
}

func TestUnknownLoop(t *testing.T) {
	if _, err := run("recursion", 1, time.Millisecond); err == nil {
		t.Error(`run with loop "recursion": err = nil, want an error`)
	}
}
