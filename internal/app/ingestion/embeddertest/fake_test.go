package embeddertest_test

import (
	"strings"
	"testing"

	"github.com/aluiziodeveloper/garimpo/internal/app/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/app/ingestion/embeddertest"
)

func TestFakeMeetsTheContract(t *testing.T) {
	embeddertest.Contract(t, func(*testing.T) ingestion.Embedder {
		return &embeddertest.Fake{Context: 64}
	}, strings.Repeat("x", 65))
}

func TestFakeRecordsEachCall(t *testing.T) {
	var f embeddertest.Fake
	for _, texts := range [][]string{{"a", "b"}, {"c"}} {
		if _, err := f.Embed(t.Context(), texts); err != nil {
			t.Fatal(err)
		}
	}
	if calls := f.Calls(); len(calls) != 2 || len(calls[0]) != 2 || calls[1][0] != "c" {
		t.Fatalf("Calls() = %q, want [[a b] [c]]", calls)
	}
}
