package win32

import (
	"math"
	"syscall"
	"unsafe"
)

// The UIA methods whose arguments take more than one stack slot on 32-bit
// Windows (uia_windows.go): stdcall passes a double in two slots (low
// word first) and a UiaPoint or VARIANT (16 bytes) in four, and a
// callback must declare every slot, since the callee pops them.

func f64(lo, hi uintptr) float64 { return math.Float64frombits(uint64(hi)<<32 | uint64(uint32(lo))) }

// uiaArchCallbacks returns the vtable entries of
// IRawElementProviderFragmentRoot::ElementProviderFromPoint,
// IRangeValueProvider::SetValue, IScrollProvider::SetScrollPercent,
// ITextProvider::RangeFromPoint and ITextRangeProvider::FindAttribute.
func uiaArchCallbacks() (fromPoint, rangeSetValue, setScrollPercent, textFromPoint, findAttribute uintptr) {
	fromPoint = syscall.NewCallback(func(this, xlo, xhi, ylo, yhi, ret uintptr) uintptr {
		return uiaElementProviderFromPoint(this, f64(xlo, xhi), f64(ylo, yhi), ret)
	})
	rangeSetValue = syscall.NewCallback(func(this, lo, hi uintptr) uintptr {
		return uiaRangeValueSetValue(this, f64(lo, hi))
	})
	setScrollPercent = syscall.NewCallback(func(this, hlo, hhi, vlo, vhi uintptr) uintptr { return uiaSetScrollPercent(this) })
	textFromPoint = syscall.NewCallback(func(this, xlo, xhi, ylo, yhi, ret uintptr) uintptr {
		return uiaTextRangeFromPoint(this, f64(xlo, xhi), f64(ylo, yhi), ret)
	})
	findAttribute = syscall.NewCallback(func(this, attr, v0, v1, v2, v3, backward, ret uintptr) uintptr {
		return uiaFindAttribute(this, ret)
	})
	return
}

// uiaRaisePropertyChanged calls UiaRaiseAutomationPropertyChangedEvent,
// whose VARIANTs take four slots each.
func uiaRaisePropertyChanged(p uintptr, id int32, o, n *variant) {
	args := append([]uintptr{p, uintptr(id)}, variantArgs(o)...)
	procUiaRaiseAutomationPropertyChangedEvent.Call(append(args, variantArgs(n)...)...)
}

// variantArgs returns the slots of a VARIANT passed by value.
func variantArgs(v *variant) []uintptr {
	w := (*[4]uintptr)(unsafe.Pointer(v))
	return w[:]
}
