//go:build !darwin

package detect

// macOSVersion is "" off macOS: the current design is picked.
func macOSVersion() string { return "" }
