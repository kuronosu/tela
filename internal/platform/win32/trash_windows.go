//go:build windows

package win32

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

// MoveToTrash with IFileOperation (see trash.go for the choice).
//
// IFileOperation works only in a single-threaded apartment, and
// MoveToTrash is called from any goroutine. So each call runs on a new
// goroutine locked to its OS thread, which joins an STA of its own, does
// the operation and leaves the apartment; the goroutine ends still locked,
// so the runtime ends the thread too and no COM state is left on a thread
// of the pool. The progress sink is a Go COM object (comserver_windows.go)
// that lives in that apartment, so the shell calls it on that thread,
// inside PerformOperations.

var (
	procCoInitializeEx = ole32.NewProc("CoInitializeEx")
	procCoUninitialize = ole32.NewProc("CoUninitialize")
)

var (
	clsidFileOperation           = guid{0x3AD05575, 0x8857, 0x4850, [8]byte{0x92, 0x77, 0x11, 0xB8, 0x5B, 0xDB, 0x8E, 0x09}}
	iidFileOperation             = guid{0x947AAB5F, 0x0A5C, 0x4C13, [8]byte{0xB4, 0xD6, 0x4B, 0xF7, 0x83, 0x6F, 0xC9, 0xF8}}
	iidFileOperationProgressSink = guid{0x04B0F1A7, 0x9490, 0x44BC, [8]byte{0x96, 0xE1, 0x42, 0x96, 0xA3, 0x12, 0x52, 0xE2}}
)

const (
	coinitApartmentThreaded = 0x2
	coinitDisableOLE1DDE    = 0x4

	// IFileOperation methods (vtable indices).
	vtFileOpSetOperationFlags       = 5
	vtFileOpDeleteItem              = 18
	vtFileOpPerformOperations       = 21
	vtFileOpGetAnyOperationsAborted = 22

	// tsfDeleteRecycleIfPossible is the TRANSFER_SOURCE_FLAGS bit that
	// PreDeleteItem gets for an item the shell will recycle.
	tsfDeleteRecycleIfPossible = 0x80
)

func (d desktop) MoveToTrash(path string) error {
	clean, _, err := platform.CheckTrash(path)
	if err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() {
		// Never unlocked: the thread ends with the goroutine.
		runtime.LockOSThread()
		done <- recycle(clean)
	}()
	return <-done
}

// recycle moves path to the Recycle Bin on the calling thread, which it
// joins to a single-threaded apartment for the duration.
func recycle(path string) error {
	r, _, _ := procCoInitializeEx.Call(0, coinitApartmentThreaded|coinitDisableOLE1DDE)
	if hr := int32(uint32(r)); hr < 0 {
		return hresultError("CoInitializeEx", hr)
	}
	defer procCoUninitialize.Call()

	var op comObject
	r, _, _ = procCoCreateInstance.Call(uintptr(unsafe.Pointer(&clsidFileOperation)), 0, clsctxInprocServer,
		uintptr(unsafe.Pointer(&iidFileOperation)), uintptr(unsafe.Pointer(&op)))
	if hr := int32(uint32(r)); hr < 0 {
		if hr == hrClassNotRegistered {
			return fmt.Errorf("win32: MoveToTrash: the shell has no IFileOperation: %w", platform.ErrUnsupported)
		}
		return hresultError("CoCreateInstance(FileOperation)", hr)
	}
	defer op.release()
	if hr := op.call(vtFileOpSetOperationFlags, trashFlags); hr < 0 {
		return hresultError("IFileOperation.SetOperationFlags", hr)
	}
	var item comObject
	r, _, _ = procSHCreateItemFromParsingName.Call(uintptr(unsafe.Pointer(utf16(path))), 0,
		uintptr(unsafe.Pointer(&iidShellItem)), uintptr(unsafe.Pointer(&item)))
	if hr := int32(uint32(r)); hr < 0 {
		return trashHRError(path, hr)
	}
	defer item.release()

	initTrashVtable()
	sink := &trashSink{}
	srv := newComServer(&trashSinkVtbl[0], []guid{iidFileOperationProgressSink}, sink, nil)
	defer srv.release()
	if hr := op.call(vtFileOpDeleteItem, uintptr(item), srv.ptr()); hr < 0 {
		return trashHRError(path, hr)
	}
	perform := op.call(vtFileOpPerformOperations)
	var aborted int32
	op.call(vtFileOpGetAnyOperationsAborted, uintptr(unsafe.Pointer(&aborted)))
	return trashOutcome(path, perform, aborted != 0, sink.refused, sink.result)
}

// trashSink is the IFileOperationProgressSink of a MoveToTrash. Its
// methods run on the operation's thread, inside PerformOperations.
type trashSink struct {
	refused bool  // PreDeleteItem stopped the item: it would not be recycled
	result  int32 // PostDeleteItem's HRESULT
}

var (
	trashVtblOnce sync.Once
	trashSinkVtbl [19]uintptr // IFileOperationProgressSink
)

// initTrashVtable creates the sink's callbacks, once per process. Every
// method but PreDeleteItem and PostDeleteItem only returns S_OK; each has
// its own arity, which matters for stdcall on 386.
func initTrashVtable() {
	trashVtblOnce.Do(func() {
		ok1 := syscall.NewCallback(func(this uintptr) uintptr { return sOK })
		ok2 := syscall.NewCallback(func(this, a uintptr) uintptr { return sOK })
		ok3 := syscall.NewCallback(func(this, a, b uintptr) uintptr { return sOK })
		ok4 := syscall.NewCallback(func(this, a, b, c uintptr) uintptr { return sOK })
		ok5 := syscall.NewCallback(func(this, a, b, c, d uintptr) uintptr { return sOK })
		ok6 := syscall.NewCallback(func(this, a, b, c, d, e uintptr) uintptr { return sOK })
		ok7 := syscall.NewCallback(func(this, a, b, c, d, e, f uintptr) uintptr { return sOK })
		ok8 := syscall.NewCallback(func(this, a, b, c, d, e, f, g uintptr) uintptr { return sOK })
		trashSinkVtbl = [19]uintptr{
			syscall.NewCallback(comQueryInterface),
			syscall.NewCallback(comAddRef),
			syscall.NewCallback(comRelease),
			ok1,                                      // StartOperations()
			ok2,                                      // FinishOperations(hr)
			ok4,                                      // PreRenameItem(flags, item, name)
			ok6,                                      // PostRenameItem(flags, item, name, hr, new)
			ok5,                                      // PreMoveItem(flags, item, folder, name)
			ok7,                                      // PostMoveItem(flags, item, folder, name, hr, new)
			ok5,                                      // PreCopyItem(flags, item, folder, name)
			ok7,                                      // PostCopyItem(flags, item, folder, name, hr, new)
			syscall.NewCallback(trashPreDeleteItem),  // PreDeleteItem(flags, item)
			syscall.NewCallback(trashPostDeleteItem), // PostDeleteItem(flags, item, hr, new)
			ok4,                                      // PreNewItem(flags, folder, name)
			ok8,                                      // PostNewItem(flags, folder, name, template, attrs, hr, new)
			ok3,                                      // UpdateProgress(total, done)
			ok1,                                      // ResetTimer()
			ok1,                                      // PauseTimer()
			ok1,                                      // ResumeTimer()
		}
	})
}

// trashSinkOf returns the sink behind an interface pointer, or nil.
func trashSinkOf(this uintptr) *trashSink {
	if s := comServerOf(this); s != nil {
		sink, _ := s.impl.(*trashSink)
		return sink
	}
	return nil
}

// IFileOperationProgressSink::PreDeleteItem: an item without
// TSF_DELETE_RECYCLE_IF_POSSIBLE would be deleted for good, so the
// operation stops before it.
func trashPreDeleteItem(this, flags, item uintptr) uintptr {
	if flags&tsfDeleteRecycleIfPossible == 0 {
		if s := trashSinkOf(this); s != nil {
			s.refused = true
		}
		return eAbort
	}
	return sOK
}

// IFileOperationProgressSink::PostDeleteItem: the item's outcome.
func trashPostDeleteItem(this, flags, item, hr, created uintptr) uintptr {
	if s := trashSinkOf(this); s != nil {
		s.result = int32(uint32(hr))
	}
	return sOK
}
