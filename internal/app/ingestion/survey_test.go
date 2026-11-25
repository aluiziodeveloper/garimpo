package ingestion

import (
	"errors"
	"iter"
	"testing"

	domain "github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
)

func documents(t *testing.T, err error, texts ...string) iter.Seq2[domain.Document, error] {
	t.Helper()
	var docs []domain.Document
	for i, text := range texts {
		d, derr := domain.NewDocument(string(rune('a'+i))+".go", text)
		if derr != nil {
			t.Fatal(derr)
		}
		docs = append(docs, d)
	}
	return func(yield func(domain.Document, error) bool) {
		for _, d := range docs {
			if !yield(d, nil) {
				return
			}
		}
		if err != nil {
			yield(domain.Document{}, err)
		}
	}
}

func TestSurveyCorpusCountsDocumentsAndChunks(t *testing.T) {
	cutter, err := domain.NewCutter(10)
	if err != nil {
		t.Fatal(err)
	}
	docs := documents(t, nil, "aaaa\nbbbb\ncccc\n", "", "dd\n")
	got, err := SurveyCorpus(docs, cutter)
	if err != nil {
		t.Fatal(err)
	}
	want := Survey{Documents: 3, Bytes: 18, Chunks: 3, Largest: 10}
	if got != want {
		t.Errorf("SurveyCorpus() = %+v, want %+v", got, want)
	}
}

func TestSurveyCorpusReturnsTheWalkError(t *testing.T) {
	broken := errors.New("unreadable file")
	_, err := SurveyCorpus(documents(t, broken, "a\n"), domain.Cutter{})
	if !errors.Is(err, broken) {
		t.Errorf("SurveyCorpus() error = %v, want %v", err, broken)
	}
}

func TestSurveyString(t *testing.T) {
	s := Survey{Documents: 2, Bytes: 43, Chunks: 2, Largest: 29}
	if got, want := s.String(), "documents=2 bytes=43 chunks=2 average=21 largest=29"; got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}
	if got := (Survey{}).String(); got != "documents=0 bytes=0 chunks=0 average=0 largest=0" {
		t.Errorf("zero Survey String() = %q", got)
	}
}
