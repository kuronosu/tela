//go:build windows

package i18n

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var procGetUserDefaultLocaleName = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetUserDefaultLocaleName")

// platformLocale returns the user's default locale name ("es-MX"), from
// GetUserDefaultLocaleName.
func platformLocale() string {
	var buf [85]uint16 // LOCALE_NAME_MAX_LENGTH
	n, _, _ := procGetUserDefaultLocaleName.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
	if n <= 1 {
		return ""
	}
	return windows.UTF16ToString(buf[:n-1])
}
