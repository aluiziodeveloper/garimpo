package ollama

import (
	"errors"
	"strings"
	"testing"

	"github.com/aluiziodeveloper/garimpo/internal/app/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/app/ingestion/embeddertest"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// The splitter cuts only at line breaks, so a single line over the context still fails,
// and the contract holds.
func TestSplitterMeetsTheContract(t *testing.T) {
	embeddertest.Contract(t, func(_ *testing.T) ingestion.Embedder {
		return &Splitter{Next: &embeddertest.Fake{Context: 64}}
	}, strings.Repeat("x", 65))
}

func TestALongTextIsEmbeddedInHalves(t *testing.T) {
	f := embeddertest.Fake{Context: 12}
	s := Splitter{Next: &f}
	text := "line one\nline two\n" // 18 bytes: over the context, two lines of 9
	vs, err := s.Embed(t.Context(), []string{"short", text})
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 2 {
		t.Fatalf("%d vectors, want 2", len(vs))
	}
	halves, _ := f.Embed(t.Context(), []string{"line one\n", "line two\n"})
	a, b := halves[0].Values(), halves[1].Values()
	mean := make([]float32, len(a))
	for i := range a {
		mean[i] = 9*a[i] + 9*b[i] // each half weighs its bytes
	}
	want, err := shared.NewVector(mean, len(mean))
	if err != nil {
		t.Fatal(err)
	}
	if d := vs[1].Dot(want); d < 1-1e-5 {
		t.Errorf("dot(vector, mean of the halves) = %v, want 1", d)
	}
	if one, _ := f.Embed(t.Context(), []string{"short"}); vs[0].Dot(one[0]) < 1-1e-5 {
		t.Error("the short text did not keep its own vector")
	}
}

func TestAHalfOverTheContextIsCutAgain(t *testing.T) {
	f := embeddertest.Fake{Context: 6}
	s := Splitter{Next: &f}
	if _, err := s.Embed(t.Context(), []string{"a\nb\nc\nd\ne\nf\ng\nh\n"}); err != nil {
		t.Fatalf("16 bytes in lines of 2 with a context of 6: %v", err)
	}
}

func TestALineOverTheContextStillFails(t *testing.T) {
	s := Splitter{Next: &embeddertest.Fake{Context: 8}}
	_, err := s.Embed(t.Context(), []string{"ok\n", "a single line of 25 bytes\n"})
	if !errors.Is(err, ingestion.ErrTextTooLong) {
		t.Fatalf("err = %v, want ErrTextTooLong", err)
	}
}
