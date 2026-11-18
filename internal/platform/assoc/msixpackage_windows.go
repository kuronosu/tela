//go:build windows

package assoc

import (
	"sync"
	"unsafe"

	syswin "golang.org/x/sys/windows"
)

var procGetCurrentPackageFullName = syswin.NewLazySystemDLL("kernel32.dll").NewProc("GetCurrentPackageFullName")

// InPackage reports whether the process runs from an MSIX package (it has
// a package identity): GetCurrentPackageFullName with a zero-length
// buffer, read by packageResult. The function is missing before Windows
// 8, which has no packages. The answer does not change while the process
// runs, so it is asked once. The win32 backend (no toast activator
// registration inside a package) and package update (an MSIX install
// updates through its manager) both read it.
var InPackage = sync.OnceValue(func() bool {
	if procGetCurrentPackageFullName.Find() != nil {
		return false
	}
	var n uint32
	r, _, _ := procGetCurrentPackageFullName.Call(uintptr(unsafe.Pointer(&n)), 0)
	return packageResult(r)
})
