//go:build !(telagpu && (linux || windows))

package main

// gpuStat is empty without the GPU backend (gpu_test.go).
type gpuStat struct{}
