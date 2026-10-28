//go:build windows

package win32

import "unsafe"

// Repainting after the OS lost what it shows of a window.
//
// The window's pixels live in the retained copy (dib.go) and Present blits
// only the damage, so whatever Windows or DWM discards of the window is
// not restored until something paints it again. That happens when a
// session reconnects (RDP, fast user switching, the lock screen), the
// display mode or the DWM composition changes, the theme is reloaded and
// the machine resumes from sleep: none of them is an event of the platform
// contract, and a window that is not resized may get no WM_PAINT for the
// parts the surface lost. These handlers repaint the whole client area
// from the copy (InvalidateRect, answered by onPaint) and make the next
// Present a full one.
//
// DefWindowProc also paints the classic caption of a window that has
// WS_CAPTION but whose WM_NCCALCSIZE gave the whole window to the client
// (a frameless window): the non-client area is not where it draws, so its
// caption lands on the app's pixels and stays there until they are painted
// over. Where that happens depends on the desktop:
//
//   - Without composition (some remote sessions, safe mode, a DWM restart)
//     the window's drawing goes straight to the screen and DefWindowProc
//     paints on it for WM_NCPAINT, WM_NCACTIVATE, WM_SETTEXT, WM_SETICON and
//     the undocumented WM_NCUAHDRAWCAPTION and WM_NCUAHDRAWFRAME. A
//     frameless window then takes those messages, as Chromium's and
//     Electron's frameless windows do.
//   - With composition, DWM owns the frame and DefWindowProc paints nothing
//     visible, unless the window answers WM_NCPAINT itself: a window that
//     returned 0 for it got a classic inactive caption and frame over its
//     content when a modal dialog (a native file dialog) deactivated and
//     disabled it (seen on Windows 11 over RDP). The messages go to
//     DefWindowProc, as they did before the composition-off handling.
//
// Both ways the client area is painted again from the retained copy after
// every activation change, so nothing DefWindowProc drew stays.

// repaint repaints the whole client area from the retained copy. With full
// set, the next Present also draws everything.
func (w *Window) repaint(full bool) {
	if w.Closed() || w.creating || w.dib.pix == nil {
		return
	}
	if full {
		w.RequestFull()
	}
	procInvalidateRect.Call(w.hwnd, 0, 0)
}

// compositionEnabled (a variable for the tests) reports whether DWM composes
// the desktop. Since Windows 8 it always does, except when the session has
// no composing display (some remote sessions, safe mode, a DWM restart) and
// the window's drawing goes straight to the screen.
var compositionEnabled = dwmComposing

func dwmComposing() bool {
	if !available(procDwmIsCompositionEnabled) {
		return false
	}
	var on int32
	hr, _, _ := procDwmIsCompositionEnabled.Call(uintptr(unsafe.Pointer(&on)))
	return hr == 0 && on != 0
}

// frameChanged redoes what a frameless window asked DWM for when the
// composition or the theme changed (its corner preference), and the frame
// is laid out again.
func (w *Window) frameChanged() {
	if w.Closed() || w.creating {
		return
	}
	if w.opts.Frameless {
		w.roundCorners()
		setWindowPos(w.hwnd, 0, 0, 0, 0, swpNoMove|swpNoSize|swpNoZOrder|swpNoActivate|swpFrameChanged)
	}
	w.repaint(true)
}

// onPowerBroadcast repaints after the machine resumes (the display may
// have been reset).
func (w *Window) onPowerBroadcast(wparam uintptr) {
	if wparam == pbtAPMResumeAutomatic || wparam == pbtAPMResumeSuspend {
		w.repaint(true)
	}
}

// onNCActivate handles WM_NCACTIVATE. A frameless window gets DefWindowProc's
// activation state without the repaint of the non-client area (lParam -1,
// as documented) when composition is off, which would draw the classic
// caption over the client area; with composition DefWindowProc gets the
// message unchanged. Either way the client area is then repainted from the
// copy, activating or not: it costs a blit, covers whatever DefWindowProc
// painted, and brings back a surface that was lost while the window was in
// the background (a reconnected session activates its windows).
func (w *Window) onNCActivate(wparam, lparam uintptr) uintptr {
	if !w.opts.Frameless {
		return defWindowProc(w.hwnd, wmNCActivate, wparam, lparam)
	}
	if !compositionEnabled() {
		lparam = ^uintptr(0)
	}
	r := defWindowProc(w.hwnd, wmNCActivate, wparam, lparam)
	w.repaint(false)
	return r
}

// defNoFramePaint runs DefWindowProc for a message that makes it paint the
// caption (WM_SETTEXT, WM_SETICON) with the window temporarily not visible,
// when this is a frameless window and DWM does not compose (otherwise
// nothing is painted). Chromium does the same ("redraw lock").
func (w *Window) defNoFramePaint(msg uint32, wparam, lparam uintptr) uintptr {
	if !w.opts.Frameless || compositionEnabled() || !isWindowVisible(w.hwnd) {
		return defWindowProc(w.hwnd, uintptr(msg), wparam, lparam)
	}
	setWindowLong(w.hwnd, gwlStyle, getWindowLong(w.hwnd, gwlStyle)&^wsVisible)
	r := defWindowProc(w.hwnd, uintptr(msg), wparam, lparam)
	setWindowLong(w.hwnd, gwlStyle, getWindowLong(w.hwnd, gwlStyle)|wsVisible)
	return r
}
