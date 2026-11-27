// Package httpapi serves the searches of Garimpo over HTTP: the adapter that turns a request
// into a call to the use case Search, with a deadline, and its results into JSON.
//
// The JSON is encoding/json/v2's (module 16): the texts, Go code, go out as they are, with
// no escape for HTML; a list with no results is [], never null; and a text that is not valid
// UTF-8 fails the response, where v1 would replace its bytes with U+FFFD and answer 200.
package httpapi

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/aluiziodeveloper/garimpo/internal/app/search"
	"github.com/aluiziodeveloper/garimpo/internal/domain/retrieval"
)

// DefaultK is how many results a request gets when it does not say.
const DefaultK = 10

// Searcher is what the handler serves: the use case Search (search.Searcher).
type Searcher interface {
	Search(ctx context.Context, q retrieval.Query, k int) ([]search.Result, error)
}

// Response is the body of a search that worked.
type Response struct {
	Query   string   `json:"query"`
	Results []Result `json:"results"`
}

// Result is one result of a search, as JSON.
type Result struct {
	Citation string  `json:"citation"`
	Score    float32 `json:"score"`
	Text     string  `json:"text"`
}

// Error is the body of a request that failed.
type Error struct {
	Error string `json:"error"`
}

// New returns the handler of GET /search?q=<query>&k=<results>. Every search has timeout
// to answer, counted from when the request arrives: past it, the search is canceled and the
// response is 504 Gateway Timeout. A client that leaves cancels its search, and gets no
// response. A query or a k the use case refuses is 400 Bad Request; any other failure,
// including a response that cannot be JSON, 500. log, when not nil, gets the failures of the
// server.
func New(s Searcher, timeout time.Duration, log *slog.Logger) http.Handler {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	mux := http.NewServeMux()
	mux.Handle("GET /search", &handler{s: s, timeout: timeout, log: log})
	return mux
}

type handler struct {
	s       Searcher
	timeout time.Duration
	log     *slog.Logger
}

func (h *handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	q, err := retrieval.NewQuery(r.URL.Query().Get("q"))
	if err != nil {
		h.write(w, http.StatusBadRequest, Error{err.Error()})
		return
	}
	k := DefaultK
	if s := r.URL.Query().Get("k"); s != "" {
		if k, err = strconv.Atoi(s); err != nil {
			h.write(w, http.StatusBadRequest, Error{fmt.Sprintf("k %q is not a number", s)})
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), h.timeout)
	defer cancel()
	results, err := h.s.Search(ctx, q, k)
	switch {
	case err == nil:
		body := Response{Query: q.String(), Results: make([]Result, len(results))}
		for i, res := range results {
			body.Results[i] = Result{Citation: res.Citation.String(), Score: res.Score, Text: res.Text}
		}
		h.write(w, http.StatusOK, body)
	case r.Context().Err() != nil:
		// The client left: nobody reads a response. The search saw the cancel too.
	case errors.Is(err, context.DeadlineExceeded):
		h.write(w, http.StatusGatewayTimeout, Error{fmt.Sprintf("the search took more than %v", h.timeout)})
	case errors.Is(err, retrieval.ErrEmptyQuery), errors.Is(err, search.ErrInvalidK):
		h.write(w, http.StatusBadRequest, Error{err.Error()})
	default:
		h.log.Error("search failed", "query", q.String(), "k", k, "err", err)
		h.write(w, http.StatusInternalServerError, failed)
	}
}

// failed is the body of a 500: the cause goes to the log, not to the client.
var failed = Error{"the search failed"}

// write sends body as JSON with the status code. The body is encoded before the status is
// sent: a body that cannot be JSON, such as a text that is not valid UTF-8, turns into a
// 500, where encoding straight to w would have sent the 200 and half the body.
func (h *handler) write(w http.ResponseWriter, code int, body any) {
	out, err := json.Marshal(body)
	if err != nil {
		h.log.Error("encoding the response", "status", code, "err", err)
		code = http.StatusInternalServerError
		out, _ = json.Marshal(failed) // a struct of one ASCII string: it cannot fail
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_, _ = w.Write(out) // the status is out: a failed write has nobody to tell
}
