//go:build windows

package win32

import (
	"fmt"
	"image"
	"unsafe"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/loop"
)

// wsOverlappedWindow is WS_OVERLAPPEDWINDOW: a normal top-level window with
// the system's title bar, border, buttons and menu.
const wsOverlappedWindow = wsOverlapped | wsCaption | wsSysMenu | wsThickFrame | wsMinimizeBox | wsMaximizeBox

// Window is a Win32 top-level window. All methods run on the UI thread.
type Window struct {
	// WindowState holds the delivered metrics, closed and the frame
	// requests, and provides Metrics and RequestFrame.
	loop.WindowState

	b       *Backend
	hwnd    uintptr
	opts    platform.WindowOptions
	style   uint32
	exStyle uint32
	dpi     uint32
	client  image.Point // physical client size at the latest WM_SIZE

	shown   bool // ShowWindow was called (at the first Present)
	dib     dib  // retained content, answers WM_PAINT
	bmi     bitmapInfo
	focused bool

	// Pointer state.
	ptr      pointerState
	emitFn   func(platform.PointerEvent) // w.pointerEvent, allocated once
	mods     platform.Modifiers          // keyboard modifiers of the message being handled
	tracking bool                        // TrackMouseEvent(TME_LEAVE) is armed
	cursor   platform.Cursor
}

var _ platform.Window = (*Window)(nil)

// NewWindow creates a hidden window; it is shown at its first Present. Its
// first event is a ConfigureEvent with the client size Windows granted.
func (b *Backend) NewWindow(opts platform.WindowOptions) (platform.Window, error) {
	opts = opts.Resolve()
	if b.closed {
		return nil, errClosed
	}
	// The window is created on the primary monitor with the default
	// position; its DPI is re-read once it exists.
	dpi := primaryDPI()
	client := clampPhysical(platform.PhysicalSize(opts.Size, scaleFromDPI(dpi)))
	style := uint32(wsOverlappedWindow | wsClipSiblings | wsClipChildren)
	if opts.FixedSize {
		style &^= wsThickFrame | wsMaximizeBox
	}
	const exStyle = 0
	outer := adjustWindowRect(rect{right: int32(client.X), bottom: int32(client.Y)}, style, exStyle, dpi)

	hwnd, _, e := procCreateWindowExW.Call(exStyle, uintptr(unsafe.Pointer(classNamePtr)), uintptr(unsafe.Pointer(utf16(opts.Title))),
		uintptr(style), cwUseDefault, cwUseDefault, uintptr(outer.width()), uintptr(outer.height()), 0, 0, b.hinst, 0)
	if hwnd == 0 {
		return nil, winError("CreateWindowExW", e)
	}
	w := &Window{b: b, hwnd: hwnd, opts: opts, style: style, exStyle: exStyle, dpi: dpi}
	w.emitFn = w.pointerEvent
	registry.mu.Lock()
	registry.windows[hwnd] = w
	registry.mu.Unlock()
	b.loop.Add(w)

	if d := windowDPI(hwnd); d != dpi {
		// Windows put the window on a monitor with another DPI: keep the
		// logical size the app asked for.
		w.dpi = d
		w.resizeClient(clampPhysical(platform.PhysicalSize(opts.Size, scaleFromDPI(d))))
	}
	b.reconfigure(w, w.osMetrics())
	return w, nil
}

// osMetrics reads the client size and returns the window metrics at the
// current DPI. A minimized window has an empty client area, which the
// platform contract takes as "not visible".
func (w *Window) osMetrics() platform.Metrics {
	w.client = image.Point{}
	if !isIconic(w.hwnd) {
		r := getClientRect(w.hwnd)
		w.client = image.Pt(int(r.width()), int(r.height()))
	}
	return metricsFor(w.client, w.dpi)
}

// outerSize returns the outer window size for a physical client size.
func (w *Window) outerSize(client image.Point) rect {
	return adjustWindowRect(rect{right: int32(client.X), bottom: int32(client.Y)}, w.style, w.exStyle, w.dpi)
}

// resizeClient resizes the window so that its client area is client physical
// pixels, keeping its position.
func (w *Window) resizeClient(client image.Point) {
	outer := w.outerSize(client)
	setWindowPos(w.hwnd, 0, 0, outer.width(), outer.height(), swpNoMove|swpNoZOrder|swpNoActivate)
}

// SetTitle sets the window title.
func (w *Window) SetTitle(title string) {
	if !w.Closed() {
		procSetWindowTextW.Call(w.hwnd, uintptr(unsafe.Pointer(utf16(title))))
	}
}

// SetCursor sets the cursor shown over the client area.
func (w *Window) SetCursor(c platform.Cursor) {
	if !c.Valid() {
		panic(fmt.Sprintf("win32: SetCursor(%d): invalid cursor; use one of the platform.Cursor constants", c))
	}
	if w.Closed() || c == w.cursor {
		return
	}
	w.cursor = c
	// Windows asks for the cursor (WM_SETCURSOR) only when the mouse moves.
	if w.ptr.inside {
		w.applyCursor()
	}
}

// applyCursor makes the current cursor the visible one.
func (w *Window) applyCursor() {
	procSetCursor.Call(w.b.cursorHandle(w.cursor))
}

// SetSize asks Windows to resize the client area to size (logical pixels,
// raised to MinSize). A maximized or minimized window keeps its state and
// size. The outcome arrives as a ConfigureEvent.
func (w *Window) SetSize(size platform.Size) {
	platform.CheckSize("SetSize size", size)
	if w.Closed() {
		return
	}
	w.opts.Size = w.opts.ClampSize(size)
	if isIconic(w.hwnd) || isZoomed(w.hwnd) {
		return
	}
	w.resizeClient(clampPhysical(platform.PhysicalSize(w.opts.Size, scaleFromDPI(w.dpi))))
}

// Close destroys the window. Queued events for it are dropped.
func (w *Window) Close() {
	if w.Closed() {
		return
	}
	w.b.loop.Remove(w) // marks it closed and drops its queued events
	w.releaseCapture()
	w.unregister()
	procDestroyWindow.Call(w.hwnd)
	w.dib.release()
}

// destroyed handles WM_NCDESTROY for a window that Windows destroyed on its
// own (the session is ending): the window leaves the loop like after Close.
func (w *Window) destroyed() {
	if w.Closed() {
		return
	}
	w.b.loop.Remove(w)
	w.unregister()
	w.dib.release()
}

func (w *Window) unregister() {
	registry.mu.Lock()
	delete(registry.windows, w.hwnd)
	registry.mu.Unlock()
}

// setFocused reports a change of the keyboard focus.
func (w *Window) setFocused(focused bool) {
	if w.focused != focused {
		w.focused = focused
		w.b.enqueue(w, platform.FocusEvent{Focused: focused})
	}
}

// onDPIChanged handles WM_DPICHANGED: the window moved to a monitor with
// another DPI, or the user changed the scale. Windows suggests the new outer
// rectangle (same logical size, positioned so that the window stays under
// the pointer); applying it produces the WM_SIZE that carries the new
// metrics.
func (w *Window) onDPIChanged(wparam, lparam uintptr) {
	w.dpi = uint32(loword(wparam))
	if r := (*rect)(ptrFromUintptr(lparam)); r != nil {
		setWindowPos(w.hwnd, r.left, r.top, r.width(), r.height(), swpNoZOrder|swpNoActivate)
	}
	// A suggested rectangle of the same pixel size sends no WM_SIZE, but
	// the scale changed.
	w.b.reconfigure(w, w.osMetrics())
}

// onGetMinMaxInfo applies WindowOptions.MinSize, a logical size that follows
// the current DPI.
func (w *Window) onGetMinMaxInfo(lparam uintptr) {
	if w.opts.MinSize == (platform.Size{}) {
		return
	}
	mm := (*minMaxInfo)(ptrFromUintptr(lparam))
	outer := w.outerSize(clampPhysical(platform.PhysicalSize(w.opts.MinSize, scaleFromDPI(w.dpi))))
	mm.minTrackSize = point{outer.width(), outer.height()}
}
