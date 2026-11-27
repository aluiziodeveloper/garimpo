package main

import (
	"bytes"
	jsonv1 "encoding/json"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"strings"
	"testing"
)

func v1(t *testing.T, r Response) string {
	t.Helper()
	var b bytes.Buffer
	if err := encodeV1(&b, r); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSuffix(b.String(), "\n")
}

func v2(t *testing.T, r Response) string {
	t.Helper()
	var b bytes.Buffer
	if err := encodeV2(&b, r); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// A search with no results: v1 writes a nil slice as null, v2 as an empty array, which a
// client can loop over without a check.
func TestANilSliceIsAnEmptyArrayInV2(t *testing.T) {
	r := Response{Query: "nothing"}
	if got, want := v1(t, r), `{"query":"nothing","results":null}`; got != want {
		t.Errorf("v1 = %s, want %s", got, want)
	}
	if got, want := v2(t, r), `{"query":"nothing","results":[]}`; got != want {
		t.Errorf("v2 = %s, want %s", got, want)
	}
}

// v1 escapes <, > and & for HTML, by default; v2 writes them as they are. Go code is full
// of them: each one costs 6 bytes in v1, against 1.
func TestV1EscapesHTMLAndV2DoesNot(t *testing.T) {
	text := "a < b && b > 0"
	r := Response{Results: []Result{{Text: text}}}
	if got := v1(t, r); !strings.Contains(got, `a \u003c b \u0026\u0026 b \u003e 0`) {
		t.Errorf("v1 = %s, want <, & and > escaped", got)
	}
	if got := v2(t, r); !strings.Contains(got, `"`+text+`"`) {
		t.Errorf("v2 = %s, want the text as it is", got)
	}
	escaped, err := json.Marshal(r, jsontext.EscapeForHTML(true))
	if err != nil || !strings.Contains(string(escaped), `\u003c`) {
		t.Errorf("v2 with EscapeForHTML: %s, err %v; want < escaped", escaped, err)
	}
}

// v1 replaces invalid UTF-8 with U+FFFD and says nothing; v2 refuses it, unless told to
// allow it.
func TestV2RefusesInvalidUTF8(t *testing.T) {
	r := Response{Query: "a\xffb"}
	if got := v1(t, r); !strings.Contains(got, "\"a\uFFFDb\"") {
		t.Errorf("v1 = %s, want the byte replaced by U+FFFD", got)
	}
	if out, err := json.Marshal(r); err == nil {
		t.Errorf("v2 = %s, err nil; want an error", out)
	} else {
		t.Logf("v2: %v", err)
	}
	if out, err := json.Marshal(r, jsontext.AllowInvalidUTF8(true)); err != nil || !strings.Contains(string(out), "\uFFFD") {
		t.Errorf("v2 with AllowInvalidUTF8 = %s, err %v; want U+FFFD", out, err)
	}
}

// Reading a request: v1 takes the last of two equal names, and matches names in any case;
// v2 refuses the duplicate, and matches the name exactly.
func TestV2ReadsNamesStrictly(t *testing.T) {
	var a, b Response
	dup := []byte(`{"query":"first","query":"second"}`)
	if err := jsonv1.Unmarshal(dup, &a); err != nil || a.Query != "second" {
		t.Errorf("v1 of a duplicate name: query %q, err %v; want second", a.Query, err)
	}
	if err := json.Unmarshal(dup, &b); err == nil {
		t.Errorf("v2 of a duplicate name: query %q, err nil; want an error", b.Query)
	} else {
		t.Logf("v2: %v", err)
	}
	upper := []byte(`{"QUERY":"loud"}`)
	a, b = Response{}, Response{}
	if err := jsonv1.Unmarshal(upper, &a); err != nil || a.Query != "loud" {
		t.Errorf("v1 of QUERY: query %q, err %v; want loud", a.Query, err)
	}
	if err := json.Unmarshal(upper, &b); err != nil || b.Query != "" {
		t.Errorf("v2 of QUERY: query %q, err %v; want it ignored, as an unknown name", b.Query, err)
	}
}
