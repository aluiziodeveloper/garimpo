package ingestion

import (
	"fmt"
	"sync"
)

// Progress counts what an ingestion run has done so far. The run's workers update it from
// many goroutines, and anyone may read a Snapshot while the run goes on, to log it or to
// show it. The zero value is ready to use; a Progress must not be copied after first use.
//
// One mutex guards the four counts. Four atomics would make each count exact and the
// snapshot a mix of moments: a reader between two loads sees more chunks done than cut
// (lesson 05-08).
type Progress struct {
	mu sync.Mutex
	c  Counts
}

// DocumentRead counts one document of the corpus read from disk.
func (p *Progress) DocumentRead() {
	p.mu.Lock()
	p.c.Documents++
	p.mu.Unlock()
}

// ChunksCut counts n chunks cut from a document.
func (p *Progress) ChunksCut(n int) {
	p.mu.Lock()
	p.c.Chunks += int64(n)
	p.mu.Unlock()
}

// ChunkEmbedded counts one chunk turned into a vector.
func (p *Progress) ChunkEmbedded() {
	p.mu.Lock()
	p.c.Embedded++
	p.mu.Unlock()
}

// ChunkFailed counts one chunk the embedder gave up on.
func (p *Progress) ChunkFailed() {
	p.mu.Lock()
	p.c.Failed++
	p.mu.Unlock()
}

// Snapshot returns the counts at one moment of the run: a copy taken under the lock.
func (p *Progress) Snapshot() Counts {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.c
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
