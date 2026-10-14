//go:build windows

package win32

import (
	"time"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

// osKeyEnv answers keyEnv from the OS state of the window's thread.
type osKeyEnv struct{ w *Window }

var _ keyEnv = osKeyEnv{}

func (e osKeyEnv) now() time.Duration            { return e.w.b.Now() }
func (e osKeyEnv) modifiers() platform.Modifiers { return e.w.modifiers() }
func (e osKeyEnv) altGrLayout() bool             { return layoutHasAltGr(currentLayout()) }
func (e osKeyEnv) lang() uint16                  { return layoutLang(currentLayout()) }

func (e osKeyEnv) scancodeOf(vk uint32) uint32 {
	return mapVirtualKey(vk, mapvkVKToVSCEx, currentLayout())
}

// char asks the layout for the character of a key with the Shift and
// CapsLock state of the thread's keyboard state, Ctrl and Alt removed (the
// contract ignores them for Key and Text) unless they are AltGr, which is
// Ctrl+Alt to the layout and selects the third level.
func (e osKeyEnv) char(vk, sc uint32) (string, charKind) {
	var state [256]byte
	procGetKeyboardState.Call(uintptr(unsafe.Pointer(&state[0])))
	hkl := currentLayout()
	altGr := state[vkRMenu]&0x80 != 0 && layoutHasAltGr(hkl)
	for _, vk := range [...]int{vkControl, vkLControl, vkRControl, vkMenu, vkLMenu, vkRMenu} {
		state[vk] &^= 0x80
	}
	if altGr {
		for _, vk := range [...]int{vkControl, vkLControl, vkMenu, vkRMenu} {
			state[vk] |= 0x80
		}
	}
	return toUnicode(vk, sc&0xFF, &state, hkl)
}

// peekKey and peekChar return the next key or character message of the
// window, with PeekMessageW and PM_NOREMOVE. The PM_QS_* flags that would keep
// it from dispatching the sent messages that are waiting do not do: they
// select messages that arrived since the last call to a message function, and
// a peek that ran just before, here or in the message loop, hides the
// characters of the next key from this one. A sent message that runs WndProc
// inside a peek only queues events (or, for WM_KILLFOCUS, releases the held
// keys), as at every other call that pumps messages.
func (e osKeyEnv) peekKey() (keyMsg, bool)  { return e.peek(wmKeyDown, wmSysKeyUp) }
func (e osKeyEnv) peekChar() (keyMsg, bool) { return e.peek(wmChar, wmSysDeadChar) }

func (e osKeyEnv) peek(first, last uintptr) (keyMsg, bool) {
	var m msg
	r, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&m)), e.w.hwnd, first, last,
		pmNoRemove)
	if r == 0 {
		return keyMsg{}, false
	}
	return keyMsg{message: m.message, wparam: m.wParam, lparam: m.lParam, time: m.time}, true
}

func (e osKeyEnv) messageTime() uint32 {
	t, _, _ := procGetMessageTime.Call()
	return uint32(t)
}

func (e osKeyEnv) keyDown(vk uint32) bool {
	r, _, _ := procGetAsyncKeyState.Call(uintptr(vk))
	return int16(r) < 0
}

// modifiers returns the shortcut keys held, for the events of this window.
// The window delivered the press of a left Ctrl that the user pressed; the
// fake one that comes with AltGr is never delivered.
func (w *Window) modifiers() platform.Modifiers {
	return keyboardModifiers(w.kbd.held.IsHeld(platform.CodeControlLeft))
}

// keyEvent queues a key event of the window. A window without the keyboard
// focus reports none: a message that PeekMessageW dispatches in the middle of
// a key's translation can take the focus away.
func (w *Window) keyEvent(e platform.KeyEvent) {
	if w.focused {
		w.b.enqueue(w, e)
	}
}
