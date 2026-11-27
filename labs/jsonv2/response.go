package main

import (
	"bytes"
	jsonv1 "encoding/json"
	"encoding/json/v2"
	"errors"
	"fmt"
	"go/build"
	"io"
	"os"
	"path/filepath"
)

// Response is the body of a search, as the server of Garimpo writes it.
type Response struct {
	Query   string   `json:"query"`
	Results []Result `json:"results"`
}

// Result is one result of a search.
type Result struct {
	Citation string  `json:"citation"`
	Score    float32 `json:"score"`
	Text     string  `json:"text"`
}

// encodeV1 writes r the way the server writes it with v1: an Encoder, which ends the value
// with a newline.
func encodeV1(w io.Writer, r Response) error {
	return jsonv1.NewEncoder(w).Encode(r)
}

// encodeV2 writes r with v2, straight to w.
func encodeV2(w io.Writer, r Response) error {
	return json.MarshalWrite(w, r)
}

// sample is a response with one result of real Go code: comparisons and && in the text.
var sample = Response{Query: "less than", Results: []Result{{
	Citation: "sort/sort.go:1-3",
	Score:    0.5,
	Text:     "func less(a, b int) bool { return a < b && b > 0 }",
}}}

// corpusResponse returns a response of 10 results of real Go code: the first 7,000 bytes of
// sort/sort.go of the toolchain, in pieces of 700 bytes, the window of Garimpo.
func corpusResponse() (Response, error) {
	src, err := os.ReadFile(filepath.Join(build.Default.GOROOT, "src", "sort", "sort.go"))
	if err != nil {
		return Response{}, err
	}
	r := Response{Query: "sort a slice in place"}
	for i := range 10 {
		r.Results = append(r.Results, Result{Citation: "sort/sort.go:1-20", Score: 0.03, Text: string(src[i*700 : (i+1)*700])})
	}
	return r, nil
}

func main() {
	for _, e := range []struct {
		name   string
		encode func(io.Writer, Response) error
	}{{"v1", encodeV1}, {"v2", encodeV2}} {
		var b bytes.Buffer
		if err := e.encode(&b, sample); err != nil {
			fmt.Println(e.name, "failed:", err)
			continue
		}
		fmt.Printf("%s, %d bytes: %s\n", e.name, b.Len(), bytes.TrimSpace(b.Bytes()))
	}
	if r, err := corpusResponse(); err != nil {
		fmt.Println("no corpus:", err)
	} else {
		var b1, b2 bytes.Buffer
		if err := errors.Join(encodeV1(&b1, r), encodeV2(&b2, r)); err != nil {
			fmt.Println("failed:", err)
		}
		fmt.Printf("10 results of sort/sort.go, 7,000 bytes of code: v1 %d bytes, v2 %d bytes\n", b1.Len(), b2.Len())
	}
	var empty bytes.Buffer
	_ = encodeV1(&empty, Response{Query: "nothing"})
	fmt.Printf("v1, no results: %s\n", bytes.TrimSpace(empty.Bytes()))
	out, _ := json.Marshal(Response{Query: "nothing"})
	fmt.Printf("v2, no results: %s\n", out)
}
