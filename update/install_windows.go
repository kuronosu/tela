package update

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var procGetCurrentPackageFullName = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetCurrentPackageFullName")

// inPackage reports whether the process runs from an MSIX package: with a
// zero-length buffer GetCurrentPackageFullName answers
// ERROR_INSUFFICIENT_BUFFER for a packaged process and
// APPMODEL_ERROR_NO_PACKAGE (15700) otherwise. The function is missing
// before Windows 8, which has no packages.
func inPackage() bool {
	if procGetCurrentPackageFullName.Find() != nil {
		return false
	}
	var n uint32
	r, _, _ := procGetCurrentPackageFullName.Call(uintptr(unsafe.Pointer(&n)), 0)
	return r == uintptr(windows.ERROR_INSUFFICIENT_BUFFER)
}
