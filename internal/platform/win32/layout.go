//go:build windows

package win32

import (
	"sync"
	utf16pkg "unicode/utf16"
	"unsafe"
)

// Questions to the active keyboard layout: its language, whether it has an
// AltGr key, and what a key types.

const (
	mapvkVKToVSCEx = 4 // MAPVK_VK_TO_VSC_EX
	// toUnicodeKeepState is the wFlags bit of ToUnicodeEx (Windows 10 1607)
	// that leaves the kernel's dead-key state alone. Without it, asking
	// what a dead key types would start a sequence that the next real key
	// press would complete.
	toUnicodeKeepState = 4
)

// currentLayout returns the keyboard layout (HKL) of the calling thread.
func currentLayout() uintptr {
	h, _, _ := procGetKeyboardLayout.Call(0)
	return h
}

// layoutLang returns the primary language id of a layout: the low 10 bits of
// the low word of the HKL.
func layoutLang(hkl uintptr) uint16 { return uint16(hkl) & 0x3FF }

func mapVirtualKey(vk, mapType uint32, hkl uintptr) uint32 {
	r, _, _ := procMapVirtualKeyExW.Call(uintptr(vk), uintptr(mapType), hkl)
	return uint32(r)
}

// toUnicode asks hkl what vk, at scan code sc, types with the given keyboard
// state. The kernel's dead-key state is left as it was: on a system that
// ignores toUnicodeKeepState, a dead key would change it, and a second call
// consumes the accent again.
func toUnicode(vk, sc uint32, state *[256]byte, hkl uintptr) (string, charKind) {
	var buf [8]uint16
	call := func() int32 {
		n, _, _ := procToUnicodeEx.Call(uintptr(vk), uintptr(sc), uintptr(unsafe.Pointer(&state[0])),
			uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), toUnicodeKeepState, hkl)
		return int32(n)
	}
	n := call()
	if keyTrace != nil {
		keyTrace("ToUnicodeEx(vk %#x, sc %#x, hkl %#x) = %d %q", vk, sc, hkl, n, utf16pkg.Decode(buf[:max(0, min(int(n), len(buf)))]))
	}
	switch {
	case n > 0:
		return string(utf16pkg.Decode(buf[:min(int(n), len(buf))])), charText
	case n < 0:
		call()
		return "", charDead
	}
	return "", charNone
}

// keyTrace, when a test sets it, receives a line for every key message and
// layout question, to find out what Windows did.
var keyTrace func(format string, args ...any)

// altGrCache remembers, per layout, whether it has AltGr. Windows gives no
// direct answer (it is the KLLF_ALTGR flag inside the layout DLL).
var altGrCache = struct {
	mu sync.Mutex
	m  map[uintptr]bool
}{m: map[uintptr]bool{}}

// layoutHasAltGr reports whether the layout has an AltGr key: a right Alt
// that Windows reports as Ctrl+Alt, and with which some keys type a
// character other than their plain one (@ on AltGr+Q or AltGr+2 in German
// and Spanish). A layout where Ctrl+Alt changes a key's character has it,
// which is how winit finds out too.
func layoutHasAltGr(hkl uintptr) bool {
	altGrCache.mu.Lock()
	defer altGrCache.mu.Unlock()
	if v, ok := altGrCache.m[hkl]; ok {
		return v
	}
	v := probeAltGr(hkl)
	altGrCache.m[hkl] = v
	return v
}

// forgetLayouts empties the AltGr cache (WM_INPUTLANGCHANGE: a layout may
// have been reinstalled).
func forgetLayouts() {
	altGrCache.mu.Lock()
	clear(altGrCache.m)
	altGrCache.mu.Unlock()
}

func probeAltGr(hkl uintptr) bool {
	var plain, ctrlAlt [256]byte
	for _, vk := range [...]int{vkControl, vkLControl, vkMenu, vkRMenu} {
		ctrlAlt[vk] = 0x80
	}
	try := func(vk uint32) bool {
		sc := mapVirtualKey(vk, 0, hkl) // MAPVK_VK_TO_VSC
		if sc == 0 {
			return false
		}
		a, ka := toUnicode(vk, sc, &plain, hkl)
		b, kb := toUnicode(vk, sc, &ctrlAlt, hkl)
		return ka == charText && kb == charText && a != b && stripControls(b) != ""
	}
	for vk := uint32('0'); vk <= '9'; vk++ {
		if try(vk) {
			return true
		}
	}
	for vk := uint32('A'); vk <= 'Z'; vk++ {
		if try(vk) {
			return true
		}
	}
	for vk := uint32(0xBA); vk <= 0xC0; vk++ {
		if try(vk) {
			return true
		}
	}
	for vk := uint32(0xDB); vk <= 0xDF; vk++ {
		if try(vk) {
			return true
		}
	}
	return try(vkOEM102)
}
