package main

import (
	"runtime"
	"strings"
	"testing"
	"time"
	"unique"
	"unsafe"
	"weak"
)

// Equal strings, each in its own allocation, get the same Handle; different ones do not.
func TestEqualValuesGetTheSameHandle(t *testing.T) {
	a := "internal/adapters/flatindex/published.go"
	b := strings.Clone(a)
	if unsafe.StringData(a) == unsafe.StringData(b) {
		t.Fatal("strings.Clone returned the same bytes: the test proves nothing")
	}
	if unique.Make(a) != unique.Make(b) {
		t.Errorf("Make(%q) and Make of its clone differ, want the same Handle", a)
	}
	if unique.Make(a) == unique.Make(a+"x") {
		t.Errorf("Make(%q) and Make(%q) are the same Handle, want different", a, a+"x")
	}
	if got := unique.Make(b).Value(); got != a {
		t.Errorf("Value() = %q, want %q", got, a)
	}
}

// A weak pointer does not keep its value alive: after the last ordinary pointer goes and
// the GC runs, Value returns nil.
func TestAWeakPointerDoesNotKeepItsValue(t *testing.T) {
	w := func() weak.Pointer[text] {
		v := &text{s: strings.Repeat("x", 4096)}
		w := weak.Make(v)
		runtime.GC()
		if w.Value() != v {
			t.Fatal("the value died while v still held it")
		}
		return w // v, the last ordinary pointer, goes with this call
	}()
	runtime.GC()
	if w.Value() != nil {
		t.Error("the weak pointer still reaches its value after the GC, with nothing else holding it")
	}
}

// The cache keeps the texts the program still holds, and AddCleanup takes the others out.
func TestTheCacheForgetsWhatNobodyHolds(t *testing.T) {
	const texts, keep = 100, 10
	c := newCache()
	held := make([]*text, 0, keep)
	for k := range texts {
		v := &text{s: strings.Repeat("y", 1024)}
		c.Put(k, v)
		if k < keep {
			held = append(held, v)
		}
	}
	if _, ok := settle(c, keep, 5*time.Second); !ok {
		t.Fatalf("the cache has %d entries, want %d", c.Len(), keep)
	}
	for k := range texts {
		got, ok := c.Get(k)
		if k < keep && (!ok || got != held[k]) {
			t.Errorf("Get(%d) = %p, %v; want the text still held", k, got, ok)
		}
		if k >= keep && ok {
			t.Errorf("Get(%d) found a text nobody holds", k)
		}
	}
	runtime.KeepAlive(held)
}

// A cleanup of an old text does not take out the entry of the text put after it.
func TestACleanupLeavesANewerEntry(t *testing.T) {
	c := newCache()
	c.Put(1, &text{s: strings.Repeat("old", 100)})
	newer := &text{s: strings.Repeat("new", 100)}
	c.Put(1, newer)
	for range 10 {
		runtime.GC()
		time.Sleep(time.Millisecond)
	}
	if got, ok := c.Get(1); !ok || got != newer {
		t.Errorf("Get(1) = %p, %v; want the newer text", got, ok)
	}
	runtime.KeepAlive(newer)
}
