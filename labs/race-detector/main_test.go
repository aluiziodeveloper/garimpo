package main

import (
	"errors"
	"testing"
)

// The tests run only the cases with synchronization: the CI runs them with -race, and the
// race detector would fail a test that runs "sleep" or "map".

func TestChanHandsTheMessageOver(t *testing.T) {
	got, err := run("chan")
	if err != nil {
		t.Fatal(err)
	}
	if got != "hello" {
		t.Errorf(`run("chan") = %q, want "hello"`, got)
	}
}

func TestOnceLoadsOnce(t *testing.T) {
	for range 20 {
		got, err := run("once")
		if err != nil {
			t.Fatal(err)
		}
		if got != "load ran 1 times" {
			t.Fatalf(`run("once") = %q, want "load ran 1 times"`, got)
		}
	}
}

func TestUnknownCase(t *testing.T) {
	if _, err := run("magic"); !errors.Is(err, errCase) {
		t.Errorf("err = %v, want errCase", err)
	}
}
