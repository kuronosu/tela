//go:build !(telagpu && (linux || windows))

package gpu

// Open opens the GPU device. This build has none: it returns
// ErrUnavailable, and the caller renders with the CPU rasterizer.
func Open(Config) (Device, error) { return nil, ErrUnavailable }
