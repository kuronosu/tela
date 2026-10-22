//go:build linux || darwin || windows

package remote

// supported reports whether this system has a shared memory
// implementation (shm_unix.go, shm_windows.go).
const supported = true
