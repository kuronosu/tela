//go:build windows

package win32

import (
	"sync"
	"syscall"

	"github.com/kuronosu/tela/internal/platform"
)

// registry maps window handles to the Go objects behind them. Windows calls
// WndProc on the thread that owns the window, and several Backends may live
// on several threads, so the maps are guarded.
var registry = struct {
	mu       sync.RWMutex
	windows  map[uintptr]*Window
	backends map[uintptr]*Backend // message-only windows
}{
	windows:  map[uintptr]*Window{},
	backends: map[uintptr]*Backend{},
}

func lookup(hwnd uintptr) (w *Window, b *Backend) {
	registry.mu.RLock()
	defer registry.mu.RUnlock()
	return registry.windows[hwnd], registry.backends[hwnd]
}

var (
	wndProcOnce sync.Once
	wndProcAddr uintptr
)

// wndProcPtr returns the stdcall function pointer of wndProc. The callback
// is created once: syscall.NewCallback allocates from a table of limited
// size that is never freed.
func wndProcPtr() uintptr {
	wndProcOnce.Do(func() { wndProcAddr = syscall.NewCallback(wndProc) })
	return wndProcAddr
}

// wndProc is the window procedure of every Tela window and of the
// message-only window. A message for a window that is not (yet, or any
// longer) registered, such as the ones sent while CreateWindowExW runs or
// DestroyWindow tears a window down, gets the default handling.
//
// A panic in the handler is recovered here, because it cannot unwind
// through the Windows frames above: the Backend records it, stops the loop,
// and Run raises it again.
func wndProc(hwnd, msg, wparam, lparam uintptr) (ret uintptr) {
	w, b := lookup(hwnd)
	if w != nil {
		b = w.b
	}
	if b == nil || b.failed {
		return defWindowProc(hwnd, msg, wparam, lparam)
	}
	defer func() {
		if r := recover(); r != nil {
			b.fail(r)
			ret = 0
		}
	}()
	if w == nil {
		return b.message(hwnd, uint32(msg), wparam, lparam)
	}
	ret = w.message(uint32(msg), wparam, lparam)
	// Run a pass for the events this message queued. Inside the modal loops
	// of DefWindowProc this is the only place that runs one; outside, it
	// saves a round trip through the message queue.
	if b.dirty {
		b.pump()
	}
	return ret
}

// message dispatches a window message. This switch is where new message
// families plug in (IME arrives with text input). A case handles the message
// and returns its result; the default is DefWindowProc.
func (w *Window) message(msg uint32, wparam, lparam uintptr) uintptr {
	if keyTrace != nil && msg >= wmKeyFirst && msg <= wmKeyLast {
		t, _, _ := procGetMessageTime.Call()
		keyTrace("msg %#x wparam %#x lparam %#x time %d", msg, wparam, lparam, int32(t))
	}
	switch msg {
	case wmClose:
		// The app decides: it answers with Window.Close.
		w.b.enqueue(w, platform.CloseRequestedEvent{})
		return 0

	case wmNCDestroy:
		w.destroyed()

	case wmSize:
		w.b.reconfigure(w, w.osMetrics())
		return 0

	case wmDPIChanged:
		w.onDPIChanged(wparam, lparam)
		return 0

	case wmGetMinMaxInfo:
		w.onGetMinMaxInfo(lparam)
		return 0

	case wmEraseBkgnd:
		// Present covers the whole client area; erasing would flash.
		return 1

	case wmPaint:
		w.onPaint()
		return 0

	case wmSetFocus:
		w.setFocused(true)
		return 0
	case wmKillFocus:
		w.setFocused(false)
		return 0

	case wmKeyDown, wmSysKeyDown:
		// The system keys are swallowed: a lone Alt or F10 would enter the
		// menu loop of a window that has no menu. Alt+F4 (close) and
		// Alt+Space (the window menu) are the exceptions. A window without
		// the keyboard focus (SetFocus(NULL) sends the keys to the active
		// window as system keys) reports nothing.
		if w.focused {
			if w.kbd.down(w.kenv, msg, wparam, lparam) {
				return defWindowProc(w.hwnd, uintptr(msg), wparam, lparam)
			}
		} else if isSystemShortcut(msg, uint32(wparam&0xFF), lparam) {
			return defWindowProc(w.hwnd, uintptr(msg), wparam, lparam)
		}
		return 0
	case wmKeyUp, wmSysKeyUp:
		if w.focused {
			w.kbd.up(w.kenv, msg, wparam, lparam)
		}
		return 0
	case wmChar, wmSysChar:
		if !w.kbd.char(w.kenv, wparam) && msg == wmChar && w.focused {
			break // not a key's text: an IME result or an injected message
		}
		return 0 // WM_SYSCHAR for DefWindowProc would beep (no menu mnemonic)
	case wmDeadChar, wmSysDeadChar:
		w.kbd.deadChar(w.kenv)
		return 0
	case wmSysCommand:
		if swallowSysCommand(wparam, lparam) {
			return 0
		}
	case wmExitMenuLoop, wmExitSizeMove:
		// The system's modal loops swallow key releases.
		w.kbd.releaseStale(w.kenv)
	case wmInputLangChange:
		forgetLayouts()
	case wmSettingChange:
		w.b.settingsChanged()

	case wmSetCursor:
		if loword(lparam) == htClient {
			w.applyCursor()
			return 1
		}

	case wmMouseMove:
		w.onMouseMove(lparam)
		return 0
	case wmMouseLeave:
		w.onMouseLeave()
		return 0
	case wmLButtonDown, wmLButtonUp, wmRButtonDown, wmRButtonUp, wmMButtonDown, wmMButtonUp:
		w.onButton(msg, wparam, lparam)
		return 0
	case wmXButtonDown, wmXButtonUp:
		w.onButton(msg, wparam, lparam)
		return 1 // TRUE: the message was handled, as WM_XBUTTON* requires
	case wmMouseWheel, wmMouseHWheel:
		w.onWheel(msg, wparam, lparam)
		return 0
	case wmCaptureChanged:
		w.onCaptureLost()
		return 0
	case wmCancelMode:
		w.onCaptureLost()
	}
	return defWindowProc(w.hwnd, uintptr(msg), wparam, lparam)
}
