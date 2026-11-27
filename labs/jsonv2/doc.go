// Program jsonv2 compares encoding/json, v1, with encoding/json/v2 on the response of a
// search of Garimpo: what each one writes and accepts, and how fast it writes. In Go 1.27,
// v2 is on by default (the experiment jsonv2 is in the baseline, and GOEXPERIMENT=nojsonv2
// turns it off), and v1 is built on top of it, keeping its own behavior.
//
//	go test -v ./labs/jsonv2
//	go run ./labs/jsonv2
package main
