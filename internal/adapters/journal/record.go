package journal

import (
	"encoding/json"
	"fmt"

	domain "github.com/aluiziodeveloper/garimpo/internal/domain/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// record is an event as the file stores it: the type by name and the fields of every
// event type, the ones it does not have left out. The domain events know nothing of JSON;
// this is the only place that does.
type record struct {
	Type      string `json:"type"`
	Run       string `json:"run"`
	Path      string `json:"path,omitempty"`
	Chunk     string `json:"chunk,omitempty"`
	Reason    string `json:"reason,omitempty"`
	Documents int    `json:"documents,omitempty"`
	Embedded  int    `json:"embedded,omitempty"`
	Failed    int    `json:"failed,omitempty"`
}

// encode returns the JSON of e.
func encode(e domain.Event) ([]byte, error) {
	var r record
	switch e := e.(type) {
	case domain.RunStarted:
		r = record{Type: "RunStarted", Run: e.Run}
	case domain.RunPaused:
		r = record{Type: "RunPaused", Run: e.Run}
	case domain.RunResumed:
		r = record{Type: "RunResumed", Run: e.Run}
	case domain.RunCompleted:
		r = record{Type: "RunCompleted", Run: e.Run, Documents: e.Documents, Embedded: e.Embedded, Failed: e.Failed}
	case domain.RunFailed:
		r = record{Type: "RunFailed", Run: e.Run, Reason: e.Reason}
	case domain.DocumentDiscovered:
		r = record{Type: "DocumentDiscovered", Run: e.Run, Path: e.Path}
	case domain.ChunkEmbedded:
		r = record{Type: "ChunkEmbedded", Run: e.Run, Chunk: e.Chunk.String()}
	case domain.ChunkFailed:
		r = record{Type: "ChunkFailed", Run: e.Run, Chunk: e.Chunk.String(), Reason: e.Reason}
	default:
		return nil, fmt.Errorf("journal: no record for event %T", e)
	}
	return json.Marshal(r)
}

// decode returns the event whose JSON is data.
func decode(data []byte) (domain.Event, error) {
	var r record
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	switch r.Type {
	case "RunStarted":
		return domain.RunStarted{Run: r.Run}, nil
	case "RunPaused":
		return domain.RunPaused{Run: r.Run}, nil
	case "RunResumed":
		return domain.RunResumed{Run: r.Run}, nil
	case "RunCompleted":
		return domain.RunCompleted{Run: r.Run, Documents: r.Documents, Embedded: r.Embedded, Failed: r.Failed}, nil
	case "RunFailed":
		return domain.RunFailed{Run: r.Run, Reason: r.Reason}, nil
	case "DocumentDiscovered":
		return domain.DocumentDiscovered{Run: r.Run, Path: r.Path}, nil
	case "ChunkEmbedded":
		id, err := shared.ParseChunkID(r.Chunk)
		return domain.ChunkEmbedded{Run: r.Run, Chunk: id}, err
	case "ChunkFailed":
		id, err := shared.ParseChunkID(r.Chunk)
		return domain.ChunkFailed{Run: r.Run, Chunk: id, Reason: r.Reason}, err
	default:
		return nil, fmt.Errorf("unknown event type %q", r.Type)
	}
}
