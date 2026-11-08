//go:build windows

package text

import (
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// mapFile maps the first size bytes of f read-only. The view keeps the
// file mapping object (and the file) alive after their handles are
// closed, so only the view's address is kept.
func mapFile(f *os.File, size int) ([]byte, error) {
	h, err := windows.CreateFileMapping(windows.Handle(f.Fd()), nil, windows.PAGE_READONLY, 0, 0, nil)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(h)
	addr, err := windows.MapViewOfFile(h, windows.FILE_MAP_READ, 0, 0, uintptr(size))
	if err != nil {
		return nil, err
	}
	return unsafe.Slice((*byte)(ptrFromUintptr(addr)), size), nil
}

// ptrFromUintptr converts an address returned by the OS (memory Go does
// not manage) without tripping vet's unsafeptr check, as the win32 backend
// does.
func ptrFromUintptr(p uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&p)) }

// unmapFile undoes mapFile. Only tests call it, on mappings nothing else
// references (readFontFile never unmaps).
func unmapFile(b []byte) error {
	return windows.UnmapViewOfFile(uintptr(unsafe.Pointer(unsafe.SliceData(b))))
}
