//go:build windows

package remote

import (
	"fmt"
	"os"
	"sync/atomic"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows: a named, pagefile-backed file mapping in the session's Local\
// namespace. The child creates it and keeps its handle and view while the
// host may still open it (until the host acknowledges a newer generation,
// see child.go); the host opens it by name. Windows' AF_UNIX cannot pass
// handles, and DuplicateHandle would need a handle to the host process.

var (
	kernel32             = windows.NewLazySystemDLL("kernel32.dll")
	procOpenFileMappingW = kernel32.NewProc("OpenFileMappingW")
)

var segmentSeq atomic.Uint32

func createSegment(size int) (seg *segment, f *os.File, name string, err error) {
	name = fmt.Sprintf(`Local\tela-remote-%d-%d-%08x`, os.Getpid(), segmentSeq.Add(1), randUint32())
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, nil, "", err
	}
	h, err := windows.CreateFileMapping(windows.InvalidHandle, nil, windows.PAGE_READWRITE, uint32(uint64(size)>>32), uint32(size), p)
	if err != nil {
		return nil, nil, "", fmt.Errorf("remote: CreateFileMapping: %w", err)
	}
	seg, err = mapView(h, windows.FILE_MAP_WRITE, size)
	if err != nil {
		return nil, nil, "", err
	}
	return seg, nil, name, nil
}

func openSegment(f *os.File, name string, size int) (*segment, error) {
	if f != nil {
		f.Close()
	}
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	r, _, e := procOpenFileMappingW.Call(windows.FILE_MAP_READ, 0, uintptr(unsafe.Pointer(p)))
	if r == 0 {
		return nil, fmt.Errorf("remote: OpenFileMapping %s: %w", name, e)
	}
	return mapView(windows.Handle(r), windows.FILE_MAP_READ, size)
}

// mapView maps size bytes of h; the segment owns h. MapViewOfFile fails
// when size exceeds the mapping, so a short mapping cannot be read past
// its end.
func mapView(h windows.Handle, access uint32, size int) (*segment, error) {
	addr, err := windows.MapViewOfFile(h, access, 0, 0, uintptr(size))
	if err != nil {
		windows.CloseHandle(h)
		return nil, fmt.Errorf("remote: MapViewOfFile: %w", err)
	}
	data := unsafe.Slice((*byte)(ptrFromUintptr(addr)), size)
	return &segment{data: data, release: func() {
		windows.UnmapViewOfFile(addr)
		windows.CloseHandle(h)
	}}, nil
}

// ptrFromUintptr converts an address returned by the OS (memory Go does
// not manage) without tripping vet's unsafeptr check, as the win32 backend
// does.
func ptrFromUintptr(p uintptr) unsafe.Pointer { return *(*unsafe.Pointer)(unsafe.Pointer(&p)) }
