//go:build !(goexperiment.simd && amd64)

package vec

// Without GOEXPERIMENT=simd, or off amd64, there is only the scalar version.
func dot(a, b []float32) float32 {
	return dotScalar(a, b)
}
