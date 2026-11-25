package main

import (
	"errors"
	"testing"
)

func TestAtomicCountIsExact(t *testing.T) {
	got, err := count("atomic", 8, 10000)
	if err != nil {
		t.Fatal(err)
	}
	if got != 80000 {
		t.Errorf("count(atomic, 8, 10000) = %d, want 80000", got)
	}
}

// One goroutine owns the plain counter, and wg.Wait orders its writes before the read:
// there is no race, so this test is safe under -race.
func TestOneWorkerNeedsNoAtomic(t *testing.T) {
	got, err := count("plain", 1, 10000)
	if err != nil {
		t.Fatal(err)
	}
	if got != 10000 {
		t.Errorf("count(plain, 1, 10000) = %d, want 10000", got)
	}
}

func TestUnknownCounter(t *testing.T) {
	if _, err := count("magic", 1, 1); !errors.Is(err, errCounter) {
		t.Errorf("err = %v, want errCounter", err)
	}
}
