package ingestion

import "github.com/aluiziodeveloper/garimpo/internal/domain/shared"

// Event is something that happened to an ingestion run, in the past tense. The run
// records one for each command it accepts, in order, and hands them out with TakeEvents.
// Module 10 writes them to the journal, and a resumed run replays them.
//
// The method is unexported: only this package declares events.
type Event interface {
	ingestionEvent()
}

// RunStarted records that the run left pending and began reading the corpus.
type RunStarted struct{ Run string }

// RunPaused records that the run stopped by request.
type RunPaused struct{ Run string }

// RunResumed records that a paused run continued.
type RunResumed struct{ Run string }

// RunCompleted records that the run handled the whole corpus; the counts are its totals.
type RunCompleted struct {
	Run                         string
	Documents, Embedded, Failed int
}

// RunFailed records that the run stopped because of an error.
type RunFailed struct{ Run, Reason string }

// DocumentDiscovered records that the run read a document of the corpus.
type DocumentDiscovered struct{ Run, Path string }

// ChunkEmbedded records that a chunk got its vector. A resumed run skips it (RN02).
type ChunkEmbedded struct {
	Run   string
	Chunk shared.ChunkID
}

// ChunkFailed records that the embedder gave up on a chunk.
type ChunkFailed struct {
	Run    string
	Chunk  shared.ChunkID
	Reason string
}

func (RunStarted) ingestionEvent()         {}
func (RunPaused) ingestionEvent()          {}
func (RunResumed) ingestionEvent()         {}
func (RunCompleted) ingestionEvent()       {}
func (RunFailed) ingestionEvent()          {}
func (DocumentDiscovered) ingestionEvent() {}
func (ChunkEmbedded) ingestionEvent()      {}
func (ChunkFailed) ingestionEvent()        {}
