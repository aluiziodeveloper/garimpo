package ollama

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aluiziodeveloper/garimpo/internal/app/ingestion"
	"github.com/aluiziodeveloper/garimpo/internal/app/ingestion/embeddertest"
	"github.com/aluiziodeveloper/garimpo/internal/domain/shared"
)

const fakeDim = 4

// fakeContext is the fake model's context, in bytes instead of tokens.
const fakeContext = 64

// fakeOllama answers POST /api/embed the way Ollama does, with a vector of fakeDim values
// taken from the SHA-256 of each input. Like Ollama, it cuts an input over fakeContext in
// silence, unless the request says "truncate": false; then it fails the request. It
// keeps the last request. Set reply to change the answer.
type fakeOllama struct {
	mu       sync.Mutex
	last     map[string]any
	arrivals []arrival
	reply    func(w http.ResponseWriter, r *http.Request, input []string)
}

// arrival is when a request reached the fake, on the clock of the test, and how many
// inputs it had.
type arrival struct {
	at     time.Time
	inputs int
}

func (f *fakeOllama) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/api/embed" {
		http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
		return
	}
	var req map[string]any
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"bad json"}`, http.StatusBadRequest)
		return
	}
	var input []string
	for _, x := range req["input"].([]any) {
		input = append(input, x.(string))
	}
	f.mu.Lock()
	f.last = req
	f.arrivals = append(f.arrivals, arrival{time.Now(), len(input)})
	reply := f.reply
	f.mu.Unlock()
	if reply != nil {
		reply(w, r, input)
		return
	}
	embeddings := make([][]float32, len(input))
	for i, text := range input {
		if len(text) > fakeContext {
			if req["truncate"] == false {
				http.Error(w, `{"error":"the input length exceeds the context length"}`, http.StatusBadRequest)
				return
			}
			text = text[:fakeContext]
		}
		sum := sha256.Sum256([]byte(text))
		for j := range fakeDim {
			embeddings[i] = append(embeddings[i], float32(sum[j])-127.5)
		}
	}
	_ = json.NewEncoder(w).Encode(map[string]any{"model": req["model"], "embeddings": embeddings})
}

// newEmbedder returns an Embedder that talks to f over the in-memory network of
// httptest.NewTestServer (Go 1.27): no port, no socket, and it works inside a synctest
// bubble. Its client sends every request to f, whatever the host, so the URL is the one
// of the dev container. The server shuts down when the test ends.
func newEmbedder(t *testing.T, f *fakeOllama) *Embedder {
	t.Helper()
	srv := httptest.NewTestServer(t, f)
	return &Embedder{
		URL: "http://ollama:11434", Model: "all-minilm:22m", Dim: fakeDim, Context: 512, NumThread: 4,
		Client: srv.Client(),
	}
}

func TestEmbedderMeetsTheContract(t *testing.T) {
	embeddertest.Contract(t, func(t *testing.T) ingestion.Embedder {
		return newEmbedder(t, new(fakeOllama))
	}, strings.Repeat("x", fakeContext+1))
}

func TestEmbedSpeaksOllamasAPI(t *testing.T) {
	var f fakeOllama
	if _, err := newEmbedder(t, &f).Embed(t.Context(), []string{"a", "b"}); err != nil {
		t.Fatal(err)
	}
	got, err := json.Marshal(f.last)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"input":["a","b"],"model":"all-minilm:22m","options":{"num_ctx":512,"num_thread":4},"truncate":false}`
	if string(got) != want {
		t.Errorf("request = %s, want %s", got, want)
	}
}

func TestAnOllamaErrorIsReturned(t *testing.T) {
	f := fakeOllama{reply: func(w http.ResponseWriter, _ *http.Request, _ []string) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":"model \"all-minilm:22m\" not found, try pulling it first"}`))
	}}
	_, err := newEmbedder(t, &f).Embed(t.Context(), []string{"a"})
	if err == nil || !strings.Contains(err.Error(), `404 Not Found: model "all-minilm:22m" not found`) {
		t.Fatalf("err = %v, want Ollama's status and message", err)
	}
}

func TestAVectorOfAnotherDimensionIsRejected(t *testing.T) {
	f := fakeOllama{reply: func(w http.ResponseWriter, _ *http.Request, _ []string) {
		_, _ = w.Write([]byte(`{"embeddings":[[1,2,3]]}`))
	}}
	_, err := newEmbedder(t, &f).Embed(t.Context(), []string{"a"})
	if !errors.Is(err, shared.ErrInvalidVector) {
		t.Fatalf("err = %v, want ErrInvalidVector (RN07)", err)
	}
}

func TestAMissingVectorIsAnError(t *testing.T) {
	f := fakeOllama{reply: func(w http.ResponseWriter, _ *http.Request, _ []string) {
		_, _ = w.Write([]byte(`{"embeddings":[[1,2,3,4]]}`))
	}}
	_, err := newEmbedder(t, &f).Embed(t.Context(), []string{"a", "b"})
	if err == nil || !strings.Contains(err.Error(), "1 vectors for 2 texts") {
		t.Fatalf("err = %v, want the count mismatch", err)
	}
}

// TestOllamaMeetsTheContract runs the contract against a real Ollama, when
// GARIMPO_OLLAMA_URL points to one (the dev container sets it; the CI does not).
func TestOllamaMeetsTheContract(t *testing.T) {
	url := os.Getenv("GARIMPO_OLLAMA_URL")
	if url == "" {
		t.Skip("GARIMPO_OLLAMA_URL is not set")
	}
	embeddertest.Contract(t, func(*testing.T) ingestion.Embedder {
		return &Embedder{URL: url, Model: "all-minilm:22m", Dim: 384, Context: 512, NumThread: 4}
	}, strings.Repeat("0x7f, ", 600)) // about 1,800 tokens
}
