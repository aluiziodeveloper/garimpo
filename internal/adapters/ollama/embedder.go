// Package ollama is the adapter to a local Ollama server. It translates Garimpo's ports
// into Ollama's HTTP API and back; nothing of that API leaves the package (anticorruption
// layer, ADR 0002).
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

// Embedder is the ingestion.Embedder backed by Ollama's POST /api/embed.
type Embedder struct {
	URL       string       // the server, such as http://ollama:11434
	Model     string       // the embedding model, such as all-minilm:22m
	Dim       int          // the model's dimension: every vector must have it (RN07)
	NumThread int          // sent as options.num_thread; 0 leaves it to Ollama (D31)
	Client    *http.Client // nil means http.DefaultClient
}

// embedRequest and embedResponse are Ollama's words. They stay in this file.
type embedRequest struct {
	Model   string        `json:"model"`
	Input   []string      `json:"input"`
	Options *embedOptions `json:"options,omitempty"`
}

type embedOptions struct {
	NumThread int `json:"num_thread,omitempty"`
}

type embedResponse struct {
	Embeddings [][]float32 `json:"embeddings"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// Embed asks Ollama for the vectors of texts, in one request.
func (e *Embedder) Embed(ctx context.Context, texts []string) ([]shared.Vector, error) {
	if len(texts) == 0 {
		return nil, nil
	}
	req := embedRequest{Model: e.Model, Input: texts}
	if e.NumThread > 0 {
		req.Options = &embedOptions{NumThread: e.NumThread}
	}
	var resp embedResponse
	if err := e.post(ctx, "/api/embed", req, &resp); err != nil {
		return nil, fmt.Errorf("ollama embed: %w", err)
	}
	if len(resp.Embeddings) != len(texts) {
		return nil, fmt.Errorf("ollama embed: %d vectors for %d texts", len(resp.Embeddings), len(texts))
	}
	vs := make([]shared.Vector, len(texts))
	for i, values := range resp.Embeddings {
		v, err := shared.NewVector(values, e.Dim)
		if err != nil {
			return nil, fmt.Errorf("ollama embed: text %d: %w", i, err)
		}
		vs[i] = v
	}
	return vs, nil
}

// post sends in as JSON to path and decodes the answer into out. A status other than 200
// is an error with Ollama's message.
func (e *Embedder) post(ctx context.Context, path string, in, out any) error {
	body, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, e.URL+path, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	client := e.Client
	if client == nil {
		client = http.DefaultClient
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		var oe errorResponse
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		if json.Unmarshal(raw, &oe) != nil || oe.Error == "" {
			oe.Error = string(raw)
		}
		return fmt.Errorf("%s: %s", resp.Status, oe.Error)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
