package win32

import (
	"math"
	"syscall"
	"unsafe"
)

// The UIA methods whose arguments syscall.NewCallback cannot take on
// x64 (uia_windows.go):
//
//   - doubles: the x64 convention passes them in XMM registers, which a
//     Go callback does not read (compileCallback refuses float arguments).
//     A thunk in uiathunk_windows_amd64.s copies XMM1 and XMM2 (the
//     second and third arguments' registers) into RDX and R8 and jumps to
//     the Go callback, which then reads the doubles' bits as integers.
//   - structures larger than 8 bytes (UiaPoint, VARIANT): passed by
//     reference, so the callback takes a pointer.

var (
	// The callbacks the thunks jump to (read by the assembly only).
	//lint:ignore U1000 read by uiathunk_windows_*.s
	uiaFromPointTarget, uiaRangeSetValueTarget uintptr
	// The thunks' addresses (DATA in uiathunk_windows_amd64.s).
	uiaFromPointThunkAddr, uiaRangeSetValueThunkAddr uintptr
)

// uiaArchCallbacks returns the vtable entries of
// IRawElementProviderFragmentRoot::ElementProviderFromPoint,
// IRangeValueProvider::SetValue, IScrollProvider::SetScrollPercent,
// ITextProvider::RangeFromPoint and ITextRangeProvider::FindAttribute.
func uiaArchCallbacks() (fromPoint, rangeSetValue, setScrollPercent, textFromPoint, findAttribute uintptr) {
	uiaFromPointTarget = syscall.NewCallback(func(this, x, y, ret uintptr) uintptr {
		return uiaElementProviderFromPoint(this, math.Float64frombits(uint64(x)), math.Float64frombits(uint64(y)), ret)
	})
	uiaRangeSetValueTarget = syscall.NewCallback(func(this, v uintptr) uintptr {
		return uiaRangeValueSetValue(this, math.Float64frombits(uint64(v)))
	})
	setScrollPercent = syscall.NewCallback(func(this uintptr) uintptr { return uiaSetScrollPercent(this) })
	textFromPoint = syscall.NewCallback(func(this, pt, ret uintptr) uintptr {
		p := (*[2]float64)(ptrFromUintptr(pt))
		return uiaTextRangeFromPoint(this, p[0], p[1], ret)
	})
	findAttribute = syscall.NewCallback(func(this, attr, value, backward, ret uintptr) uintptr {
		return uiaFindAttribute(this, ret)
	})
	return uiaFromPointThunkAddr, uiaRangeSetValueThunkAddr, setScrollPercent, textFromPoint, findAttribute
}

// uiaRaisePropertyChanged calls UiaRaiseAutomationPropertyChangedEvent,
// whose VARIANTs go by reference on x64.
func uiaRaisePropertyChanged(p uintptr, id int32, o, n *variant) {
	procUiaRaiseAutomationPropertyChangedEvent.Call(p, uintptr(id), uintptr(unsafe.Pointer(o)), uintptr(unsafe.Pointer(n)))
}

// variantArgs returns the arguments of a VARIANT passed by value to a
// COM method (a pointer to it on x64). v must be on the heap.
func variantArgs(v *variant) []uintptr { return []uintptr{uintptr(unsafe.Pointer(v))} }
