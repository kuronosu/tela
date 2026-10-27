//go:build windows

package win32

import (
	"errors"
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// A minimal COM client for the common item dialogs: interfaces are raw
// pointers to objects whose first word points to their vtable, methods are
// called by vtable index with syscall.SyscallN (stdcall on 386, the one
// calling convention elsewhere), and every interface obtained is released
// by the code that obtained it.

// comObject is a COM interface pointer.
type comObject uintptr

// IUnknown methods (every interface starts with them).
const (
	vtQueryInterface = 0
	vtAddRef         = 1
	vtRelease        = 2
)

// call calls method index i of the object with the given arguments and
// returns its HRESULT. Like LazyProc.Call it is marked uintptrescapes: a
// pointer converted to uintptr in the call expression stays on the heap
// and alive until the call returns.
//
//go:uintptrescapes
func (o comObject) call(i int, args ...uintptr) int32 {
	vtbl := *(*unsafe.Pointer)(ptrFromUintptr(uintptr(o)))
	fn := *(*uintptr)(unsafe.Add(vtbl, uintptr(i)*unsafe.Sizeof(uintptr(0))))
	all := make([]uintptr, 0, len(args)+1)
	all = append(all, uintptr(o))
	all = append(all, args...)
	r, _, _ := syscall.SyscallN(fn, all...)
	return int32(uint32(r))
}

// release releases the interface (nothing for a nil pointer).
func (o comObject) release() {
	if o != 0 {
		o.call(vtRelease)
	}
}

// guid literals of the classes and interfaces used.
var (
	clsidFileOpenDialog = guid{0xDC1C5A9C, 0xE88A, 0x4DDE, [8]byte{0xA5, 0xA1, 0x60, 0xF8, 0x2A, 0x20, 0xAE, 0xF7}}
	clsidFileSaveDialog = guid{0xC0B4E2F3, 0xBA21, 0x4773, [8]byte{0x8D, 0xBA, 0x33, 0x5E, 0xC9, 0x46, 0xEB, 0x8B}}
	iidFileOpenDialog   = guid{0xD57C7288, 0xD4AD, 0x4768, [8]byte{0xBE, 0x02, 0x9D, 0x96, 0x95, 0x32, 0xD9, 0x60}}
	iidFileSaveDialog   = guid{0x84BCCD23, 0x5FDE, 0x4CDB, [8]byte{0xAE, 0xA4, 0xAF, 0x64, 0xB8, 0x3D, 0x78, 0xAB}}
	iidShellItem        = guid{0x43826D1E, 0xE718, 0x42EE, [8]byte{0xBC, 0x55, 0xA1, 0xE2, 0x61, 0xC3, 0x7B, 0xFE}}
)

const (
	clsctxInprocServer   = 0x1
	coinitApartment      = 0x2 // COINIT_APARTMENTTHREADED
	coinitDisableOLE1DDE = 0x4
)

// errMTA is returned when the UI thread already joined the multithreaded
// COM apartment, where the common item dialogs do not work.
var errMTA = errors.New("win32: the UI thread is in COM's multithreaded apartment; the file dialogs need a single-threaded one (do not call CoInitializeEx with COINIT_MULTITHREADED on it)")

// hresultError describes a failed COM call.
func hresultError(what string, hr int32) error {
	if code, ok := hresultFacilityWin32(hr); ok {
		return fmt.Errorf("win32: %s: %w", what, syscall.Errno(code))
	}
	return fmt.Errorf("win32: %s: %s", what, hresultString(hr))
}

// initCOM joins the UI thread to a single-threaded COM apartment (the
// dialogs and ShellExecuteW, which may load shell extensions, need it),
// once; shutdown balances it. The flags are those rfd and the
// ShellExecuteW documentation use.
func (d *desktopState) initCOM() error {
	if d.comDone {
		return d.comErr
	}
	d.comDone = true
	r, _, _ := procCoInitializeEx.Call(0, coinitApartment|coinitDisableOLE1DDE)
	switch hr := int32(uint32(r)); hr {
	case sOK, sFalse:
		d.comInit = true // S_FALSE: already initialized, still counted
	case hrChangedMode:
		d.comErr = errMTA
	default:
		d.comErr = hresultError("CoInitializeEx", hr)
	}
	return d.comErr
}

// closeCOM undoes initCOM.
func (d *desktopState) closeCOM() {
	if d.comInit {
		procCoUninitialize.Call()
	}
	d.comInit, d.comDone, d.comErr = false, false, nil
}

// coTaskString returns the UTF-16 string at p, allocated by COM, and frees
// it.
func coTaskString(p uintptr) string {
	if p == 0 {
		return ""
	}
	s := windows.UTF16PtrToString((*uint16)(ptrFromUintptr(p)))
	procCoTaskMemFree.Call(p)
	return s
}
