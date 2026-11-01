//go:build windows

package win32

import (
	"runtime"
	"slices"
	"sync"
	"sync/atomic"
	"syscall"
	"unsafe"
)

// COM objects implemented in Go: the IDropTarget of every window, and the
// IDropSource and IDataObject of a drag (dnd_windows.go). The client side,
// calling the OS's objects, is com_windows.go.
//
//   - Memory. The pointer COM holds is the address of a comHeader, whose
//     only field is the address of the interface's vtable: a package-level
//     array, which never moves and is never freed. The header is a Go
//     allocation pinned with runtime.Pinner for as long as the object
//     lives, so the garbage collector neither moves nor frees it while COM
//     holds its address (a uintptr it cannot see); comServers maps that
//     address to the Go object.
//   - Methods. The vtable entries are syscall.NewCallback functions,
//     created once and shared by every object of an interface (the
//     callback table holds about 2000 and is never freed). Each looks its
//     object up by the this pointer, so an object COM still calls after
//     its last Release (a bug elsewhere) finds nothing and fails cleanly.
//   - Reference counting. AddRef and Release are atomic. An object starts
//     with one reference, its creator's; when the count reaches zero it
//     leaves the map, is unpinned, and its final hook releases what it
//     holds.
//   - Threads. The objects live in the UI thread's single-threaded
//     apartment (OleInitialize), so COM calls them on that thread only;
//     another process's calls are marshalled to it through its message
//     loop (Run's, or a modal loop's: DoDragDrop, a dialog).

// comHeader is the memory a COM interface pointer points to.
type comHeader struct {
	vtbl uintptr
}

// comServer is a COM object implemented in Go.
type comServer struct {
	hdr   *comHeader
	pin   runtime.Pinner
	refs  atomic.Int32
	iids  []guid // the interfaces it answers besides IUnknown
	impl  any    // *dropTargetServer, *dropSourceServer or *dataObjectServer
	final func() // runs when the last reference goes, or nil
}

var comServers = struct {
	sync.Mutex
	m map[uintptr]*comServer
}{m: map[uintptr]*comServer{}}

// HRESULTs the servers return (winerror.h).
const (
	eNotImpl               = 0x80004001
	eNoInterface           = 0x80004002
	ePointer               = 0x80004003
	eOutOfMemory           = 0x8007000E
	eInvalidArg            = 0x80070057
	oleEAdviseNotSupported = 0x80040003
	dvEFormatEtc           = 0x80040064
	dvETymed               = 0x80040069
	dvEDVAspect            = 0x8004006B
)

// iidUnknown is IID_IUnknown.
var iidUnknown = guid{0x00000000, 0x0000, 0x0000, [8]byte{0xC0, 0, 0, 0, 0, 0, 0, 0x46}}

// newComServer creates a COM object with one reference (the caller's)
// whose vtable is the array starting at vtbl.
func newComServer(vtbl *uintptr, iids []guid, impl any, final func()) *comServer {
	initComVtables()
	s := &comServer{hdr: &comHeader{vtbl: uintptr(unsafe.Pointer(vtbl))}, iids: iids, impl: impl, final: final}
	s.pin.Pin(s.hdr)
	s.refs.Store(1)
	comServers.Lock()
	comServers.m[s.ptr()] = s
	comServers.Unlock()
	return s
}

// ptr returns the interface pointer COM sees.
func (s *comServer) ptr() uintptr { return uintptr(unsafe.Pointer(s.hdr)) }

func (s *comServer) addRef() int32 { return s.refs.Add(1) }

// release drops a reference; the last one destroys the object.
func (s *comServer) release() int32 {
	n := s.refs.Add(-1)
	if n == 0 {
		comServers.Lock()
		delete(comServers.m, s.ptr())
		comServers.Unlock()
		s.pin.Unpin()
		if s.final != nil {
			s.final()
		}
	}
	return n
}

// comServerOf returns the object behind an interface pointer, or nil.
func comServerOf(this uintptr) *comServer {
	comServers.Lock()
	defer comServers.Unlock()
	return comServers.m[this]
}

// The vtables. Each starts with IUnknown's three methods.
var (
	vtblOnce       sync.Once
	dropTargetVtbl [7]uintptr  // IDropTarget
	dropSourceVtbl [5]uintptr  // IDropSource
	dataObjectVtbl [12]uintptr // IDataObject
)

// initComVtables creates the callbacks, once per process.
func initComVtables() {
	vtblOnce.Do(func() {
		qi := syscall.NewCallback(comQueryInterface)
		addRef := syscall.NewCallback(comAddRef)
		release := syscall.NewCallback(comRelease)
		// The callbacks whose arguments differ between 32 and 64-bit
		// Windows (a POINTL by value) are in dndcall*_windows.go.
		dropTargetVtbl = [7]uintptr{qi, addRef, release,
			syscall.NewCallback(dtDragEnter),
			syscall.NewCallback(dtDragOver),
			syscall.NewCallback(dtDragLeave),
			syscall.NewCallback(dtDrop),
		}
		dropSourceVtbl = [5]uintptr{qi, addRef, release,
			syscall.NewCallback(dsQueryContinueDrag),
			syscall.NewCallback(dsGiveFeedback),
		}
		dataObjectVtbl = [12]uintptr{qi, addRef, release,
			syscall.NewCallback(doGetData),
			syscall.NewCallback(doGetDataHere),
			syscall.NewCallback(doQueryGetData),
			syscall.NewCallback(doGetCanonicalFormatEtc),
			syscall.NewCallback(doSetData),
			syscall.NewCallback(doEnumFormatEtc),
			syscall.NewCallback(doDAdvise),
			syscall.NewCallback(doDUnadvise),
			syscall.NewCallback(doEnumDAdvise),
		}
	})
}

// IUnknown::QueryInterface.
func comQueryInterface(this, riid, ppv uintptr) uintptr {
	if ppv == 0 {
		return ePointer
	}
	*(*uintptr)(ptrFromUintptr(ppv)) = 0
	s := comServerOf(this)
	if s == nil || riid == 0 {
		return eNoInterface
	}
	iid := *(*guid)(ptrFromUintptr(riid))
	if iid != iidUnknown && !slices.Contains(s.iids, iid) {
		return eNoInterface
	}
	s.addRef()
	*(*uintptr)(ptrFromUintptr(ppv)) = this
	return sOK
}

// IUnknown::AddRef.
func comAddRef(this uintptr) uintptr {
	if s := comServerOf(this); s != nil {
		return uintptr(s.addRef())
	}
	return 0
}

// IUnknown::Release.
func comRelease(this uintptr) uintptr {
	if s := comServerOf(this); s != nil {
		return uintptr(max(s.release(), 0))
	}
	return 0
}
