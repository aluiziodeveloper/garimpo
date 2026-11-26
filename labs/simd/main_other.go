//go:build !(goexperiment.simd && amd64)

package main

import "fmt"

func main() {
	fmt.Println("this lab needs GOEXPERIMENT=simd on amd64: GOEXPERIMENT=simd go test -bench . ./labs/simd")
}
