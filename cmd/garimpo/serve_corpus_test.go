package main

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"os"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestServeOnTheCorpus measures garimpo serve on the corpus, with the real model: 100
// requests at once, all equal and then all different, with -collapse and without. It logs
// the time of the 100, the p50 and p99 of each request, the statuses and the calls to the
// model, counted by a proxy in front of Ollama. Without GARIMPO_VECTORS (a file of garimpo
// ingest -vectors), GARIMPO_CORPUS and GARIMPO_OLLAMA_URL, it skips. In the bench service:
//
//	BENCH_CPUSET=2,4,6,8,10,12,14,16 docker compose run --rm bench sh -c 'GARIMPO_VECTORS=/src/.bs/corpus-chunks.vec GARIMPO_CORPUS=/usr/local/go/src go test -count=1 -run TestServeOnTheCorpus -v ./cmd/garimpo'
func TestServeOnTheCorpus(t *testing.T) {
	vectors, dir, model := os.Getenv("GARIMPO_VECTORS"), os.Getenv("GARIMPO_CORPUS"), os.Getenv("GARIMPO_OLLAMA_URL")
	if vectors == "" || dir == "" || model == "" {
		t.Skip("set GARIMPO_VECTORS, GARIMPO_CORPUS and GARIMPO_OLLAMA_URL")
	}
	target, err := url.Parse(model)
	if err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int64
	proxy := httputil.NewSingleHostReverseProxy(target)
	counted := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		proxy.ServeHTTP(w, r)
	}))
	defer counted.Close()

	// 100 equal questions, and 100 different ones: 10 subjects with 10 endings each.
	equal := slices.Repeat([]string{"sort+a+slice+in+place"}, 100)
	var different []string
	for _, q := range []string{
		"sort+a+slice", "parse+a+duration", "context+deadline", "read+a+file", "channel+send",
		"http+server", "json+encode", "mutex+lock", "string+builder", "time+format",
	} {
		for _, w := range []string{"fast", "safe", "in+place", "with+errors", "by+key", "of+bytes", "twice", "again", "now", "first"} {
			different = append(different, q+"+"+w)
		}
	}
	// One query with each k from 1 to 50: 50 searches, one text for the model.
	var ks []string
	for k := range 50 {
		ks = append(ks, fmt.Sprintf("sort+a+slice+in+place&k=%d", k+1))
	}
	for _, collapse := range []bool{false, true} {
		var out syncBuffer
		addr, stop, done := started(t, &out, "-dim", "384", "-corpus", dir, "-vectors", vectors,
			"-ollama", counted.URL, fmt.Sprintf("-collapse=%t", collapse))
		if code, _ := get(t, addr, "warm+up"); code != http.StatusOK { // the model loaded, the connections open
			t.Fatalf("warm-up: status %d", code)
		}
		for _, run := range []struct {
			name    string
			queries []string
		}{{"100 equal", equal}, {"100 different", different}, {"1 query, 50 ks", ks}} {
			before := calls.Load()
			took, each, statuses := burst(t, addr, run.queries)
			t.Logf("collapse=%-5t %-14s all in %v; each p50 %v, p99 %v; %v; %d calls to the model",
				collapse, run.name, took.Round(time.Millisecond), each[len(each)/2].Round(time.Millisecond),
				each[len(each)*99/100].Round(time.Millisecond), statuses, calls.Load()-before)
		}
		stop()
		if err := <-done; err != nil {
			t.Fatal(err)
		}
	}
}

// burst sends the queries at once, and returns how long they all took, the time of each,
// sorted, and how many got each status.
func burst(t *testing.T, addr string, queries []string) (time.Duration, []time.Duration, map[int]int) {
	t.Helper()
	each := make([]time.Duration, len(queries))
	codes := make([]int, len(queries))
	var wg sync.WaitGroup
	start := time.Now()
	for i, q := range queries {
		wg.Go(func() {
			s := time.Now()
			resp, err := http.Get("http://" + addr + "/search?q=" + q)
			if err != nil {
				t.Error(err)
				return
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
			each[i], codes[i] = time.Since(s), resp.StatusCode
		})
	}
	wg.Wait()
	took := time.Since(start)
	slices.Sort(each)
	statuses := map[int]int{}
	for _, c := range codes {
		statuses[c]++
	}
	return took, each, statuses
}
