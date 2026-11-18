//go:build !windows

package assoc

// InPackage is false: MSIX packages exist only on Windows.
func InPackage() bool { return false }
