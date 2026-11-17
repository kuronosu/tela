//go:build !windows

package update

// inPackage is false: MSIX packages exist only on Windows.
func inPackage() bool { return false }
