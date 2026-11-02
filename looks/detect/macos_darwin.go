package detect

import "golang.org/x/sys/unix"

// macOSVersion is the version of the running macOS, from the sysctl
// kern.osproductversion (no cgo), or "" if it cannot be read.
func macOSVersion() string {
	v, err := unix.Sysctl("kern.osproductversion")
	if err != nil {
		return ""
	}
	return v
}
