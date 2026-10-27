//go:build windows

package win32

import (
	"sync"
	"syscall"

	"golang.org/x/sys/windows"

	"github.com/kuronosu/tela/internal/platform"
)

// registry maps window handles to the Go objects behind them. Windows calls
// WndProc on the thread that owns the window, and several Backends may live
// on several threads, so the maps are guarded.
var registry = struct {
	mu       sync.RWMutex
	windows  map[uintptr]*Window
	backends map[uintptr]*Backend // message-only windows
	creating map[uint32]*Window   // per thread: the window inside CreateWindowExW
}{
	windows:  map[uintptr]*Window{},
	backends: map[uintptr]*Backend{},
	creating: map[uint32]*Window{},
}

func lookup(hwnd uintptr) (w *Window, b *Backend) {
	registry.mu.RLock()
	w, b = registry.windows[hwnd], registry.backends[hwnd]
	creating := w == nil && b == nil && len(registry.creating) > 0
	registry.mu.RUnlock()
	if creating {
		w = adopt(hwnd)
	}
	return w, b
}

// adopt registers a window that NewWindow is creating, at the first message
// CreateWindowExW sends it (WM_GETMINMAXINFO), so that the sizing messages
// sent during creation are answered for it (a frameless window's
// WM_NCCALCSIZE, the size limits). Only Tela windows use this window
// procedure, and a thread creates one at a time.
func adopt(hwnd uintptr) *Window {
	tid := windows.GetCurrentThreadId()
	registry.mu.Lock()
	defer registry.mu.Unlock()
	w := registry.creating[tid]
	if w == nil {
		return nil
	}
	delete(registry.creating, tid)
	w.hwnd = hwnd
	registry.windows[hwnd] = w
	return w
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

// message dispatches a window message. A case handles the message and
// returns its result; the default is DefWindowProc.
func (w *Window) message(msg uint32, wparam, lparam uintptr) uintptr {
	if keyTrace != nil && (msg >= wmKeyFirst && msg <= wmKeyLast || isIMEMessage(msg)) {
		t, _, _ := procGetMessageTime.Call()
		keyTrace("msg %#x wparam %#x lparam %#x time %d", msg, wparam, lparam, int32(t))
	}
	if w.creating {
		// Inside CreateWindowExW: the window is not in the loop yet.
		switch msg {
		case wmGetMinMaxInfo:
			w.onGetMinMaxInfo(lparam)
			return 0
		case wmNCCalcSize:
			if w.opts.Frameless {
				w.onNCCalcSize(lparam)
				return 0
			}
		}
		return defWindowProc(w.hwnd, uintptr(msg), wparam, lparam)
	}
	switch msg {
	case wmClose:
		// The app decides: it answers with Window.Close.
		w.b.enqueue(w, platform.CloseRequestedEvent{})
		return 0

	case wmNCDestroy:
		w.destroyed()

	case wmSize:
		// The state first (a WindowStateEvent before the ConfigureEvent of
		// the same change).
		m := w.osMetrics()
		w.updateState()
		w.b.reconfigure(w, m)
		return 0
	case wmMove:
		w.updateState()
		return 0
	case wmShowWindow:
		w.onShowWindow(lparam)

	case wmNCCalcSize:
		if w.opts.Frameless {
			w.onNCCalcSize(lparam)
			return 0
		}
	case wmNCHitTest:
		return w.onNCHitTest(lparam)
	case wmNCMouseMove:
		w.onNCMouseMove(wparam, lparam)
		// DefWindowProc too: Windows 11 opens the snap layouts flyout from
		// the hover of an HTMAXBUTTON.
	case wmNCMouseLeave:
		w.onMouseLeave(true)
	case wmNCLButtonDown, wmNCLButtonUp, wmNCLButtonDblClk, wmNCRButtonDown, wmNCRButtonUp, wmNCRButtonDblClk,
		wmNCMButtonDown, wmNCMButtonUp, wmNCMButtonDblClk:
		if w.onNCButton(msg, wparam, lparam) {
			return 0
		}
	case wmNCXButtonDown, wmNCXButtonUp, wmNCXButtonDblClk:
		if w.onNCButton(msg, wparam, lparam) {
			return 1 // TRUE, as WM_NCXBUTTON* requires
		}

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
		// The IMEs of the new language may read other forms.
		w.ime.placed, w.ime.dirty = false, true
		w.b.dirty = true

	case wmIMESetContext, wmIMEStartComposition, wmIMEComposition, wmIMEEndComposition, wmIMEChar, wmIMERequest:
		if r, ok := w.imeMessage(msg, wparam, lparam); ok {
			return r
		}
	case wmSettingChange:
		w.b.settingsChanged()

	case wmSetCursor:
		// The app's cursor over its content, including its own title bar
		// regions; Windows' cursors over the frame (resize arrows).
		if code := uintptr(loword(lparam)); code == htClient || (len(w.regions) > 0 && isAppHit(code) && w.cursorOverAppRegion()) {
			w.applyCursor()
			return 1
		}

	case wmMouseMove:
		w.onMouseMove(lparam)
		return 0
	case wmMouseLeave:
		w.onMouseLeave(false)
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
