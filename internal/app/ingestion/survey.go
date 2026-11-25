package ingestion

import (
	"fmt"
	"iter"

	domain "github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/seq"
)

// Survey is what a dry run of the ingestion found in the corpus: the documents, their
// bytes, the chunks they were cut into and the size of the largest chunk.
type Survey struct {
	Documents int
	Bytes     int
	Chunks    int
	Largest   int
}

// String formats the survey for a log line, with the average chunk size.
func (s Survey) String() string {
	average := 0
	if s.Chunks > 0 {
		average = s.Bytes / s.Chunks
	}
	return fmt.Sprintf("documents=%d bytes=%d chunks=%d average=%d largest=%d",
		s.Documents, s.Bytes, s.Chunks, average, s.Largest)
}

// SurveyCorpus cuts every document of docs with cutter and counts what it found, without
// embedding anything. It holds one document at a time. It stops at the first error of
// docs and returns it.
func SurveyCorpus(docs iter.Seq2[domain.Document, error], cutter domain.Cutter) (Survey, error) {
	var s Survey
	err := seq.Results[domain.Document](docs).
		Peek(func(d domain.Document) {
			s.Documents++
			s.Bytes += len(d.Text())
		}).
		FlatMap(cutter.Cut).
		Each(func(c domain.Chunk) {
			s.Chunks++
			s.Largest = max(s.Largest, len(c.Text()))
		})
	return s, err
}
