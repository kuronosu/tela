//go:build windows

package win32

import (
	"image"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
)

// TitleBarOverlay windows (ADR-236): the window calls. The pure half
// (WM_NCCALCSIZE math, the buttons' rectangle, the glyph color) is
// overlay.go, which explains the approach.

// dwmwaCaptionButtonBounds is DWMWA_CAPTION_BUTTON_BOUNDS: the caption
// buttons' RECT, relative to the window rectangle (DwmGetWindowAttribute).
const dwmwaCaptionButtonBounds = 5

// overlayAvailable reports whether a TitleBarOverlay window keeps DWM's
// caption buttons on this system (overlaySupported).
func overlayAvailable() bool {
	return overlaySupported(windowsBuild(), compositionEnabled(),
		available(procDwmDefWindowProc) && available(procDwmGetWindowAttribute))
}

// dwmDefWindowProc calls DwmDefWindowProc: DWM's handling of the caption
// buttons it draws over an extended frame (hit test, hover, press). ok
// reports that DWM handled the message, with r its result.
func dwmDefWindowProc(hwnd uintptr, msg uint32, wparam, lparam uintptr) (r uintptr, ok bool) {
	handled, _, _ := procDwmDefWindowProc.Call(hwnd, uintptr(msg), wparam, lparam, uintptr(unsafe.Pointer(&r)))
	return r, handled != 0
}

// overlayMessage runs DwmDefWindowProc first, as DWM requires of a window
// that keeps its caption buttons over the client area, and reports whether
// the message is done. Only the button messages that DWM claims (presses,
// releases and double clicks over its buttons) end there: the app must not
// get them, and DefWindowProc would track the buttons itself. Everything
// else continues as usual: WM_NCHITTEST asks DWM in onNCHitTest (also for
// pointerOverApp), and WM_NCMOUSEMOVE and WM_NCMOUSELEAVE, which DWM needs
// for the hover highlight, still update the pointer (a move over DWM's
// buttons is a leave of the app's content) and reach DefWindowProc (the
// snap layouts flyout of the maximize button).
func (w *Window) overlayMessage(msg uint32, wparam, lparam uintptr) (uintptr, bool) {
	if msg == wmNCHitTest {
		return 0, false
	}
	r, ok := dwmDefWindowProc(w.hwnd, msg, wparam, lparam)
	if !ok {
		return 0, false
	}
	switch msg {
	case wmNCLButtonDown, wmNCLButtonUp, wmNCLButtonDblClk, wmNCRButtonDown, wmNCRButtonUp, wmNCRButtonDblClk,
		wmNCMButtonDown, wmNCMButtonUp, wmNCMButtonDblClk, wmNCXButtonDown, wmNCXButtonUp, wmNCXButtonDblClk:
		return r, true
	}
	return 0, false
}

// overlayHitTest asks DWM whether a WM_NCHITTEST point is over one of its
// caption buttons: HTMINBUTTON, HTMAXBUTTON (Windows 11 shows the snap
// layouts flyout from it) or HTCLOSE. DWM leaves other points with a zero
// result (HTNOWHERE).
func (w *Window) overlayHitTest(lparam uintptr) (uintptr, bool) {
	if !w.overlay {
		return 0, false
	}
	code, ok := dwmDefWindowProc(w.hwnd, wmNCHitTest, 0, lparam)
	return code, ok && code != 0
}

// osButtonAt reports whether a point in physical client coordinates is over
// the OS's caption buttons of a TitleBarOverlay window: what is there is
// the OS's, not the app's (no pointer events, the system's cursor).
func (w *Window) osButtonAt(x, y int) bool {
	return w.overlay && image.Pt(x, y).In(w.ctlPx)
}

// overlayControls returns WindowState.TitleBarControls (updateState): empty
// for other windows and in fullscreen (no caption, so no buttons), the last
// rectangle while minimized (the bounds of an iconic window mean nothing),
// else the buttons' rectangle DWM reports. Reading it also asks for the
// check at the next Presents (recheckControls).
func (w *Window) overlayControls(iconic bool) platform.Rect {
	switch {
	case !w.overlay:
		return platform.Rect{}
	case iconic:
		return w.controls
	case w.fs != nil:
		w.controls, w.ctlPx = platform.Rect{}, image.Rectangle{}
	default:
		w.controls, w.ctlPx = w.readControls()
	}
	w.recheck = recheckFrames
	return w.controls
}

// readControls reads DWMWA_CAPTION_BUTTON_BOUNDS and converts it to the
// client area (controlsRect). Both are empty when DWM does not answer.
func (w *Window) readControls() (platform.Rect, image.Rectangle) {
	var b rect
	if hr, _, _ := procDwmGetWindowAttribute.Call(w.hwnd, dwmwaCaptionButtonBounds, uintptr(unsafe.Pointer(&b)), unsafe.Sizeof(b)); hr != 0 {
		return platform.Rect{}, image.Rectangle{}
	}
	off := clientOrigin(w.hwnd).Sub(getWindowRect(w.hwnd).image().Min)
	return controlsRect(b.image(), off, w.client, w.scale())
}

// recheckFrames is how many Presents after a reported state read the
// buttons' rectangle again (recheckControls).
const recheckFrames = 3

// recheckControls reads the buttons' rectangle again at the first Presents
// after the state was reported, and reports the state again if it moved:
// DWM may lay its buttons out for a new size, DPI or theme after the WM_SIZE
// (maximize, restore) or WM_DPICHANGED that reported the state, and a window
// that was never shown may have no buttons to report yet (the first Present
// shows it). The app presents the frames of that change soon after, so a
// stale rectangle lasts a frame or two.
func (w *Window) recheckControls() {
	if w.recheck == 0 {
		return
	}
	w.recheck--
	if w.fs != nil || isIconic(w.hwnd) {
		return
	}
	if r, _ := w.readControls(); r != w.controls {
		n := w.recheck
		w.updateState()
		// Keep counting down: a moving rectangle may take another frame.
		w.recheck = n
	}
}

// overlayMargins returns the frame extended into the client area of a
// TitleBarOverlay window without a client-area backdrop: the height of the
// caption a decorated window has at the current DPI (the top of
// AdjustWindowRectExForDpi without a menu bar), so DWM draws its buttons
// whole, maximized or not, and nothing else. The pixels there stay the app's
// (the retained copy is opaque).
func (w *Window) overlayMargins() margins {
	style, exStyle := w.style, w.exStyle
	if w.fs != nil {
		style, exStyle = w.fs.style, w.fs.exStyle
	}
	top := -adjustWindowRectMenu(rect{}, style, exStyle, w.dpi, false).top
	return margins{top: max(top, 1)}
}

// overlayDPIChanged extends the frame again for the new DPI's caption height
// and reports the buttons where DWM puts them at that DPI.
func (w *Window) overlayDPIChanged() {
	if !w.overlay {
		return
	}
	w.extendFrame(w.glass)
	w.updateState()
}
