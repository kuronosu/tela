package win32

import (
	"math"
	"syscall"
	"unsafe"
)

// The UIA methods whose arguments syscall.NewCallback cannot take on
// ARM64 (uia_windows.go). AAPCS64 passes doubles, and a structure of two
// doubles (UiaPoint, a homogeneous aggregate), in the floating-point
// registers D0-D7, numbered apart from the integer ones X0-X7, which a Go
// callback reads alone. A thunk in uiathunk_windows_arm64.s moves the
// integer arguments after the doubles up and the doubles' bits into the
// integer registers they would hold, and jumps to the Go callback.
// Structures larger than 16 bytes (VARIANT) are passed by reference.

var (
	// The callbacks the thunks jump to (read by the assembly only).
	//lint:ignore U1000 read by uiathunk_windows_*.s
	uiaFromPointTarget, uiaRangeSetValueTarget, uiaTextFromPointTarget uintptr
	// The thunks' addresses (DATA in uiathunk_windows_arm64.s).
	uiaFromPointThunkAddr, uiaRangeSetValueThunkAddr, uiaTextFromPointThunkAddr uintptr
)

// uiaArchCallbacks returns the vtable entries of
// IRawElementProviderFragmentRoot::ElementProviderFromPoint,
// IRangeValueProvider::SetValue, IScrollProvider::SetScrollPercent,
// ITextProvider::RangeFromPoint and ITextRangeProvider::FindAttribute.
func uiaArchCallbacks() (fromPoint, rangeSetValue, setScrollPercent, textFromPoint, findAttribute uintptr) {
	uiaFromPointTarget = syscall.NewCallback(func(this, x, y, ret uintptr) uintptr {
		return uiaElementProviderFromPoint(this, math.Float64frombits(uint64(x)), math.Float64frombits(uint64(y)), ret)
	})
	uiaTextFromPointTarget = syscall.NewCallback(func(this, x, y, ret uintptr) uintptr {
		return uiaTextRangeFromPoint(this, math.Float64frombits(uint64(x)), math.Float64frombits(uint64(y)), ret)
	})
	uiaRangeSetValueTarget = syscall.NewCallback(func(this, v uintptr) uintptr {
		return uiaRangeValueSetValue(this, math.Float64frombits(uint64(v)))
	})
	setScrollPercent = syscall.NewCallback(func(this uintptr) uintptr { return uiaSetScrollPercent(this) })
	findAttribute = syscall.NewCallback(func(this, attr, value, backward, ret uintptr) uintptr {
		return uiaFindAttribute(this, ret)
	})
	return uiaFromPointThunkAddr, uiaRangeSetValueThunkAddr, setScrollPercent, uiaTextFromPointThunkAddr, findAttribute
}

// uiaRaisePropertyChanged calls UiaRaiseAutomationPropertyChangedEvent,
// whose VARIANTs (24 bytes) go by reference on ARM64.
func uiaRaisePropertyChanged(p uintptr, id int32, o, n *variant) {
	procUiaRaiseAutomationPropertyChangedEvent.Call(p, uintptr(id), uintptr(unsafe.Pointer(o)), uintptr(unsafe.Pointer(n)))
}

// variantArgs returns the arguments of a VARIANT passed by value to a
// COM method (a pointer to it on ARM64). v must be on the heap.
func variantArgs(v *variant) []uintptr { return []uintptr{uintptr(unsafe.Pointer(v))} }
