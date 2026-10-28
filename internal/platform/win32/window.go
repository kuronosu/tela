//go:build windows

package win32

import (
	"fmt"
	"image"
	"slices"
	"unsafe"

	"golang.org/x/sys/windows"

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

	// Keyboard state: the translation of key messages into KeyEvents.
	kbd  *keyTranslator
	kenv keyEnv

	// Pointer state.
	ptr      pointerState
	emitFn   func(platform.PointerEvent) // w.pointerEvent, allocated once
	mods     platform.Modifiers          // keyboard modifiers of the message being handled
	tracking bool                        // TrackMouseEvent(TME_LEAVE) is armed
	cursor   platform.Cursor

	// IME state (ime_windows.go).
	ime imeWindow

	// Window control state (windowctl.go).
	creating    bool // inside CreateWindowExW: only the sizing messages are handled
	ready       bool // the first ConfigureEvent is queued: the state can be reported
	visible     bool // the app wants the window shown (not Hidden, SetVisible)
	everShown   bool
	ownerHidden bool        // Windows hid it with its minimized owner
	pending     showRequest // a state request made while it was not shown
	parent      *Window
	blockers    int             // visible modal children: disabled while > 0
	blocking    bool            // this modal window disables its parent
	fs          *fullscreenSave // non-nil while fullscreen
	pos         image.Point     // content origin of the normal placement
	onTop       bool
	regions     []platform.HitRegion
	icon        []image.Image // SetIcon's images, kept for DPI changes
	icons       [2]uintptr    // ICON_SMALL and ICON_BIG
	ncTracking  bool          // TrackMouseEvent(TME_LEAVE|TME_NONCLIENT) is armed
}

var _ platform.Window = (*Window)(nil)

// NewWindow creates a hidden window; it is shown at its first Present (or,
// for WindowOptions.Hidden, at the first Present after SetVisible(true)).
// Its first event is a ConfigureEvent with the client size Windows granted,
// followed by a WindowStateEvent.
func (b *Backend) NewWindow(opts platform.WindowOptions) (platform.Window, error) {
	opts = opts.Resolve()
	if b.closed {
		return nil, errClosed
	}
	w := &Window{b: b, opts: opts, parent: b.parentOf(opts.Parent), visible: !opts.Hidden}
	w.style = uint32(wsOverlappedWindow | wsClipSiblings | wsClipChildren)
	if opts.FixedSize {
		w.style &^= wsThickFrame | wsMaximizeBox
	}
	if opts.Maximized {
		// Created maximized (and hidden): the first ConfigureEvent has
		// the maximized size, and the normal placement keeps opts.Size.
		w.style |= wsMaximize
	}
	if opts.AlwaysOnTop {
		w.exStyle |= wsExTopmost
	}
	x, y := w.initialPlacement()
	outer := w.outerSize(clampPhysical(platform.PhysicalSize(opts.Size, w.scale())))
	var owner uintptr
	if w.parent != nil {
		owner = w.parent.hwnd
	}

	// The messages sent during CreateWindowExW (WM_GETMINMAXINFO,
	// WM_NCCALCSIZE) already need the window: wndProc adopts it at its
	// first message.
	tid := windows.GetCurrentThreadId()
	registry.mu.Lock()
	registry.creating[tid] = w
	registry.mu.Unlock()
	w.creating = true
	hwnd, _, e := procCreateWindowExW.Call(uintptr(w.exStyle), uintptr(unsafe.Pointer(classNamePtr)), uintptr(unsafe.Pointer(utf16(opts.Title))),
		uintptr(w.style), x, y, uintptr(outer.width()), uintptr(outer.height()), owner, 0, b.hinst, 0)
	w.creating = false
	registry.mu.Lock()
	delete(registry.creating, tid)
	if hwnd == 0 {
		if w.hwnd != 0 {
			delete(registry.windows, w.hwnd)
		}
		registry.mu.Unlock()
		return nil, winError("CreateWindowExW", e)
	}
	w.hwnd = hwnd
	registry.windows[hwnd] = w
	registry.mu.Unlock()

	w.emitFn = w.pointerEvent
	w.kenv = osKeyEnv{w}
	w.kbd = newKeyTranslator(w.keyEvent)
	b.loop.Add(w)
	w.applyTheme(b.settings.DarkMode)
	// Windows give a new window the thread's default input context; the IME
	// stays off until the app reports a text field (SetTextInput).
	procImmAssociateContextEx.Call(hwnd, 0, 0)

	if d := windowDPI(hwnd); d != w.dpi {
		// Windows put the window on a monitor with another DPI: keep the
		// logical size the app asked for.
		w.dpi = d
		if !isZoomed(hwnd) {
			w.resizeClient(clampPhysical(platform.PhysicalSize(opts.Size, scaleFromDPI(d))))
		}
	}
	w.initControl()
	b.reconfigure(w, w.osMetrics())
	w.ready = true
	w.updateState()
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

// outerSize returns the outer window size for a physical client size: the
// same size for a frameless window, whose whole window is client area.
func (w *Window) outerSize(client image.Point) rect {
	r := rect{right: int32(client.X), bottom: int32(client.Y)}
	if w.opts.Frameless {
		if snapHas("ncstrip") {
			r.bottom++
		}
		return r
	}
	return adjustWindowRect(r, w.style, w.exStyle, w.dpi)
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
// clamped to the size limits). A maximized, minimized or fullscreen window
// keeps its state and size; size becomes the size it returns to. The
// outcome arrives as a ConfigureEvent.
func (w *Window) SetSize(size platform.Size) {
	platform.CheckSize("SetSize size", size)
	if w.Closed() {
		return
	}
	w.opts.Size = w.opts.ClampSize(size)
	client := clampPhysical(platform.PhysicalSize(w.opts.Size, scaleFromDPI(w.dpi)))
	if w.fs == nil && !isIconic(w.hwnd) && !isZoomed(w.hwnd) {
		w.resizeClient(client)
		return
	}
	style, exStyle := w.style, w.exStyle
	if w.fs != nil {
		style, exStyle = w.fs.style, w.fs.exStyle
	}
	r := rect{right: int32(client.X), bottom: int32(client.Y)}
	if !w.opts.Frameless {
		r = adjustWindowRect(r, style, exStyle, w.dpi)
	}
	w.setNormalRect(func(n image.Rectangle) image.Rectangle {
		return image.Rectangle{Min: n.Min, Max: n.Min.Add(image.Pt(int(r.width()), int(r.height())))}
	})
}

// Close destroys the window. Queued events for it are dropped. Its child
// windows are closed first, in creation order (Windows would destroy them
// with their owner anyway), and a modal window re-enables its parent
// before it goes, so that Windows activates the parent.
func (w *Window) Close() {
	if w.Closed() {
		return
	}
	for _, c := range slices.Clone(w.b.loop.Windows()) {
		if c.parent == w {
			c.Close()
		}
	}
	w.setBlocking(false)
	w.closeIME()
	w.b.loop.Remove(w) // marks it closed and drops its queued events
	w.releaseCapture()
	w.unregister()
	procDestroyWindow.Call(w.hwnd)
	w.dib.release()
	destroyIcons(w.icons)
	w.icons = [2]uintptr{}
}

// destroyed handles WM_NCDESTROY for a window that Windows destroyed on its
// own (the session is ending): the window leaves the loop like after Close.
func (w *Window) destroyed() {
	if w.Closed() {
		return
	}
	w.setBlocking(false)
	w.b.loop.Remove(w)
	w.ime = imeWindow{}
	w.unregister()
	w.dib.release()
	destroyIcons(w.icons)
	w.icons = [2]uintptr{}
}

func (w *Window) unregister() {
	registry.mu.Lock()
	delete(registry.windows, w.hwnd)
	registry.mu.Unlock()
}

// setFocused reports a change of the keyboard focus. Losing it releases the
// keys that are held first (synthetic releases, ADR-050); gaining it presses
// nothing, so the keys already down are ignored until their release. The IME
// session ends before the FocusEvent of a loss and starts after the one of a
// gain (as in the headless backend).
func (w *Window) setFocused(focused bool) {
	if w.focused != focused {
		if !focused {
			w.kbd.focusLost(w.kenv) // while keyEvent still delivers
		}
		w.focused = focused
		if !focused {
			w.syncIME()
		}
		w.b.enqueue(w, platform.FocusEvent{Focused: focused})
		if focused {
			w.syncIME()
		}
	}
}

// onDPIChanged handles WM_DPICHANGED: the window moved to a monitor with
// another DPI, or the user changed the scale. Windows suggests the new outer
// rectangle (same logical size, positioned so that the window stays under
// the pointer); applying it produces the WM_SIZE that carries the new
// metrics.
func (w *Window) onDPIChanged(wparam, lparam uintptr) {
	w.dpi = uint32(loword(wparam))
	// A fullscreen window keeps covering its monitor.
	if r := (*rect)(ptrFromUintptr(lparam)); r != nil && w.fs == nil {
		setWindowPos(w.hwnd, r.left, r.top, r.width(), r.height(), swpNoZOrder|swpNoActivate)
	}
	// A suggested rectangle of the same pixel size sends no WM_SIZE, but
	// the scale changed.
	w.b.reconfigure(w, w.osMetrics())
	// The caret's physical position changed with the scale.
	w.ime.dirty = true
	// The icons have other sizes at the new DPI.
	if len(w.icon) > 0 {
		w.applyIcon()
	}
}

// onGetMinMaxInfo applies WindowOptions.MinSize and MaxSize (logical sizes
// that follow the current DPI) as the outer track sizes; the minimum wins
// where the maximum is below it. Windows clamps the maximized size to the
// maximum track size too. A fullscreen window ignores the limits.
func (w *Window) onGetMinMaxInfo(lparam uintptr) {
	minSize, maxSize := w.opts.MinSize, w.opts.MaxSize
	if w.fs != nil || (minSize == (platform.Size{}) && maxSize == (platform.Size{})) {
		return
	}
	mm := (*minMaxInfo)(ptrFromUintptr(lparam))
	scale := w.scale()
	if minSize != (platform.Size{}) {
		outer := w.outerSize(clampPhysical(platform.PhysicalSize(minSize, scale)))
		mm.minTrackSize = point{outer.width(), outer.height()}
	}
	if maxSize.W > 0 || maxSize.H > 0 {
		outer := w.outerSize(clampPhysical(platform.PhysicalSize(maxSize, scale)))
		if maxSize.W > 0 {
			mm.maxTrackSize.x = max(outer.width(), mm.minTrackSize.x)
		}
		if maxSize.H > 0 {
			mm.maxTrackSize.y = max(outer.height(), mm.minTrackSize.y)
		}
	}
}
