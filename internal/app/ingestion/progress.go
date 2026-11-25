package ingestion

import (
	"fmt"
	"sync/atomic"
)

// Progress counts what an ingestion run has done so far. The run's workers update it from
// many goroutines, and anyone may read a Snapshot while the run goes on, to log it or to
// show it. The zero value is ready to use; a Progress must not be copied after first use.
type Progress struct {
	documents atomic.Int64
	chunks    atomic.Int64
	embedded  atomic.Int64
	failed    atomic.Int64
}

// DocumentRead counts one document of the corpus read from disk.
func (p *Progress) DocumentRead() { p.documents.Add(1) }

// ChunksCut counts n chunks cut from a document.
func (p *Progress) ChunksCut(n int) { p.chunks.Add(int64(n)) }

// ChunkEmbedded counts one chunk turned into a vector.
func (p *Progress) ChunkEmbedded() { p.embedded.Add(1) }

// ChunkFailed counts one chunk the embedder gave up on.
func (p *Progress) ChunkFailed() { p.failed.Add(1) }

// Snapshot returns the counts so far.
func (p *Progress) Snapshot() Counts {
	return Counts{
		Documents: p.documents.Load(),
		Chunks:    p.chunks.Load(),
		Embedded:  p.embedded.Load(),
		Failed:    p.failed.Load(),
	}
}

// Counts is the progress of a run, as read by Snapshot.
type Counts struct {
	Documents int64
	Chunks    int64
	Embedded  int64
	Failed    int64
}

// Pending is how many chunks were cut and are neither embedded nor failed yet.
func (c Counts) Pending() int64 { return c.Chunks - c.Embedded - c.Failed }

func (c Counts) String() string {
	return fmt.Sprintf("documents=%d chunks=%d embedded=%d failed=%d pending=%d",
		c.Documents, c.Chunks, c.Embedded, c.Failed, c.Pending())
}
