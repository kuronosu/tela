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

// comServer is a COM object implemented in Go. It has one interface
// pointer per vtable: the drag and drop objects one, a UI Automation
// element one per provider interface (uia_windows.go). Each header answers
// QueryInterface for its own IIDs; IUnknown is the first header (COM's
// identity rule).
type comServer struct {
	hdrs  []comHeader // pinned; COM's pointers are their addresses
	pin   runtime.Pinner
	refs  atomic.Int32
	iids  [][]guid // per header: the interfaces it answers besides IUnknown
	impl  any      // *dropTargetServer, *dropSourceServer, *dataObjectServer, *uiaNode...
	final func()   // runs when the last reference goes, or nil
}

// comIface is one interface of a comServer: its vtable and IIDs.
type comIface struct {
	vtbl *uintptr
	iids []guid
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
	return newComServerOf([]comIface{{vtbl, iids}}, impl, final)
}

// newComServerOf creates a COM object with one reference whose interfaces
// are ifaces (a nil vtbl leaves a header out of QueryInterface).
func newComServerOf(ifaces []comIface, impl any, final func()) *comServer {
	s := &comServer{hdrs: make([]comHeader, len(ifaces)), iids: make([][]guid, len(ifaces)), impl: impl, final: final}
	for i, f := range ifaces {
		if f.vtbl != nil {
			s.hdrs[i].vtbl = uintptr(unsafe.Pointer(f.vtbl))
		}
		s.iids[i] = f.iids
	}
	s.pin.Pin(&s.hdrs[0])
	s.refs.Store(1)
	comServers.Lock()
	for i := range s.hdrs {
		comServers.m[s.iface(i)] = s
	}
	comServers.Unlock()
	return s
}

// ptr returns the interface pointer COM sees (the first interface's).
func (s *comServer) ptr() uintptr { return s.iface(0) }

// iface returns the pointer of interface i.
func (s *comServer) iface(i int) uintptr { return uintptr(unsafe.Pointer(&s.hdrs[i])) }

func (s *comServer) addRef() int32 { return s.refs.Add(1) }

// release drops a reference; the last one destroys the object.
func (s *comServer) release() int32 {
	n := s.refs.Add(-1)
	if n == 0 {
		comServers.Lock()
		for i := range s.hdrs {
			delete(comServers.m, s.iface(i))
		}
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
	p := uintptr(0)
	if iid == iidUnknown {
		p = s.ptr()
	} else {
		for i, ids := range s.iids {
			if s.hdrs[i].vtbl != 0 && slices.Contains(ids, iid) {
				p = s.iface(i)
				break
			}
		}
	}
	if p == 0 {
		return eNoInterface
	}
	s.addRef()
	*(*uintptr)(ptrFromUintptr(ppv)) = p
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
