//go:build !windows

package wine

// Detected reports whether the process runs under Wine; it never does on
// a system other than Windows.
func Detected() bool { return false }
