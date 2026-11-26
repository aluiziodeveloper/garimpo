//go:build goexperiment.simd && amd64

package main

import "fmt"

func main() {
	fmt.Println("the dot product of 384 dimensions, in every version: go test -bench . ./labs/simd")
}
