//go:build !(telagpu && (linux || windows))

package gpu

// hasBackend reports whether this build has a device backend.
const hasBackend = false
