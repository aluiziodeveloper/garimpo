# labs/jsonv2

`encoding/json/v2` against `encoding/json` (v1) on the response of a search of Garimpo
(module 16). In Go 1.27, v2 is part of the standard library and on by default: the
experiment `jsonv2` is in the baseline, and `GOEXPERIMENT=nojsonv2` turns it off (then
this lab does not build). With v2 on, v1 is built on top of it and keeps its own behavior.

```bash
docker compose exec dev go test -v ./labs/jsonv2
docker compose exec dev go run ./labs/jsonv2
```

What changes, each in a test:

| | v1 | v2 | Test |
|---|---|---|---|
| A nil slice | `null` | `[]` | `TestANilSliceIsAnEmptyArrayInV2` |
| `<`, `>` and `&` in a string | `\u003c`, `\u003e`, `\u0026`, for HTML | as they are; `jsontext.EscapeForHTML(true)` escapes them | `TestV1EscapesHTMLAndV2DoesNot` |
| Invalid UTF-8 | replaced by U+FFFD, no error | an error; `jsontext.AllowInvalidUTF8(true)` replaces it | `TestV2RefusesInvalidUTF8` |
| Two members with the same name | the last one wins | an error | `TestV2ReadsNamesStrictly` |
| `{"QUERY": ...}` into the field `query` | matched, in any case | ignored, as an unknown name | `TestV2ReadsNamesStrictly` |

Measured in the `bench` service: 8 virtual CPUs of an i7-13700H, Go 1.27.4, 2026-11-27:

```text
v1, 158 bytes: {"query":"less than","results":[{"citation":"sort/sort.go:1-3","score":0.5,"text":"func less(a, b int) bool { return a \u003c b \u0026\u0026 b \u003e 0 }"}]}
v2, 137 bytes: {"query":"less than","results":[{"citation":"sort/sort.go:1-3","score":0.5,"text":"func less(a, b int) bool { return a < b && b > 0 }"}]}
10 results of sort/sort.go, 7,000 bytes of code: v1 7976 bytes, v2 7875 bytes
v1, no results: {"query":"nothing","results":null}
v2, no results: {"query":"nothing","results":[]}
```

```bash
BENCH_CPUSET=2,4,6,8,10,12,14,16 docker compose run --rm bench sh -c 'go test -run "^$" -bench Encode -benchmem -count 10 ./labs/jsonv2 > /tmp/b.txt && go tool -modfile=tools/go.mod benchstat -col /enc /tmp/b.txt'
```

| | v1 | v2 |
|---|---|---|
| sec/op | 6.547µ ± 6% | 6.420µ ± 1% (p = 0.247, no difference) |
| B/op | 96 | 96 |
| allocs/op | 2 | 2 |

The response of 10 results costs the same in both: v1 runs on the code of v2. What v2 brings
to the server is what it writes and refuses, not speed.
