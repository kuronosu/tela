//go:build windows

package wine

import "golang.org/x/sys/windows"

// Detected reports whether the process runs under Wine: Wine's ntdll.dll
// exports wine_get_version, which Windows' does not.
func Detected() bool {
	return windows.NewLazySystemDLL("ntdll.dll").NewProc("wine_get_version").Find() == nil
}
