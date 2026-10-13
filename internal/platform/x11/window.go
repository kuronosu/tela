//go:build linux

package x11

import (
	"fmt"
	"image"
	"os"
	"strings"

	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/loop"
)

// maxWindowSide is the largest X11 window side (a CARD16 that the server
// also treats as INT16 in coordinates).
const maxWindowSide = 32767

// windowEvents is the core event mask of every window. KeymapState makes
// the server send KeymapNotify (the keys down) after every FocusIn.
const windowEvents = xproto.EventMaskExposure |
	xproto.EventMaskStructureNotify |
	xproto.EventMaskFocusChange |
	xproto.EventMaskKeyPress |
	xproto.EventMaskKeyRelease |
	xproto.EventMaskKeymapState

// corePointerEvents are the core pointer events a window selects when the
// server has no XInput2 2.1 (otherwise it selects the XI2 ones: a client
// gets either for a window, and XI2 brings smooth scrolling).
const corePointerEvents = xproto.EventMaskButtonPress |
	xproto.EventMaskButtonRelease |
	xproto.EventMaskPointerMotion |
	xproto.EventMaskEnterWindow |
	xproto.EventMaskLeaveWindow

// Window is an X11 top-level window. All methods run on the UI thread.
type Window struct {
	// WindowState holds the delivered metrics, closed, the frame requests
	// and the iconified veto (SetHidden), and provides Metrics and
	// RequestFrame.
	loop.WindowState

	b    *Backend
	id   xproto.Window
	gc   xproto.Gcontext
	opts platform.WindowOptions

	mapped bool // MapWindow was sent (at the first Present)

	osSize image.Point // latest physical size from the server

	// Retained copy of the window content in the server's pixel layout
	// (BGRX, stride 4*pixSize.X), used to answer Expose.
	pix     []byte
	pixSize image.Point
	expose  image.Rectangle // Expose rectangles accumulated until Count == 0

	// Pointer state.
	inside       bool
	leavePending bool // the pointer left while a button was held (see LeaveNotify)
	focused      bool
	extra        platform.Buttons // held buttons the core state mask lacks (back, forward)
	lastPos      platform.Point
	cursor       platform.Cursor
}

var _ platform.Window = (*Window)(nil)

// NewWindow creates an unmapped window; it is mapped at its first Present.
// Its first event is a ConfigureEvent with the requested size (an unmapped
// window has the size it was created with); the window manager's decision
// arrives as a later ConfigureEvent once the window is mapped.
func (b *Backend) NewWindow(opts platform.WindowOptions) (platform.Window, error) {
	opts = opts.Resolve()
	if b.closed {
		return nil, errClosed
	}
	size := clampPhysical(platform.PhysicalSize(opts.Size, b.scale))

	id, err := xproto.NewWindowId(b.conn)
	if err != nil {
		return nil, fmt.Errorf("x11: allocating a window id: %w", err)
	}
	gc, err := xproto.NewGcontextId(b.conn)
	if err != nil {
		return nil, fmt.Errorf("x11: allocating a graphics context id: %w", err)
	}
	// Background None: the server never clears the window, so exposed or
	// newly grown areas keep stale pixels until Expose is answered from the
	// retained copy, instead of flashing a background color. NorthWest bit
	// gravity keeps the content in place on resize instead of discarding it.
	const mask = xproto.CwBackPixmap | xproto.CwBitGravity | xproto.CwEventMask | xproto.CwCursor
	events := uint32(windowEvents)
	if !b.xi.ok {
		events |= corePointerEvents
	}
	values := []uint32{xproto.BackPixmapNone, xproto.GravityNorthWest, events, uint32(b.cursorFor(platform.CursorDefault))}
	err = xproto.CreateWindowChecked(b.conn, b.depth, id, b.root, 0, 0, uint16(size.X), uint16(size.Y), 0,
		xproto.WindowClassInputOutput, b.visual, mask, values).Check()
	if err != nil {
		return nil, fmt.Errorf("x11: CreateWindow: %w", err)
	}
	// GraphicsExposures off: PutImage never generates them, but the
	// default GC would make any later CopyArea send NoExpose events.
	xproto.CreateGC(b.conn, gc, xproto.Drawable(id), xproto.GcGraphicsExposures, []uint32{0})
	if b.xi.ok {
		b.sendRequest(xiSelectEventsRequest(b.xi.opcode, id, xiAllMasterDevices, xiWindowEvents))
	}

	w := &Window{b: b, id: id, gc: gc, opts: opts, osSize: size}
	a := b.atoms
	b.setProperty(id, a.wmProtocols, xproto.AtomAtom, 32, card32s(uint32(a.wmDeleteWindow), uint32(a.netWMPing)))
	b.setProperty(id, xproto.AtomWmClass, xproto.AtomString, 8, wmClass(opts.AppID, platform.DefaultAppID()))
	b.setProperty(id, xproto.AtomWmHints, xproto.AtomWmHints, 32, wmHints())
	b.setProperty(id, a.netWMWindowType, xproto.AtomAtom, 32, card32s(uint32(a.netWMTypeNormal)))
	// _NET_WM_PING requires _NET_WM_PID and WM_CLIENT_MACHINE (EWMH).
	b.setProperty(id, a.netWMPID, xproto.AtomCardinal, 32, card32s(uint32(os.Getpid())))
	if host, err := os.Hostname(); err == nil {
		b.setProperty(id, xproto.AtomWmClientMachine, xproto.AtomString, 8, []byte(host))
	}
	w.setTitle(opts.Title)
	w.updateNormalHints()

	b.windows[id] = w
	b.loop.Add(w)
	b.queueConfigure(w)
	return w, nil
}

// clampPhysical keeps a physical size inside what X11 accepts.
func clampPhysical(p image.Point) image.Point {
	p.X = min(max(p.X, 1), maxWindowSide)
	p.Y = min(max(p.Y, 1), maxWindowSide)
	return p
}

// osMetrics returns the metrics for the latest server size at the current
// scale.
func (w *Window) osMetrics() platform.Metrics {
	return platform.Metrics{Size: platform.LogicalSize(w.osSize, w.b.scale), Scale: w.b.scale}
}

// Metrics and RequestFrame come from loop.WindowState. Frames requested
// while the window is iconified (SetHidden) stay pending until it is mapped
// again, like frame callbacks of a hidden Wayland surface.

// SetTitle sets WM_NAME and _NET_WM_NAME.
func (w *Window) SetTitle(title string) {
	if !w.Closed() {
		w.setTitle(title)
	}
}

func (w *Window) setTitle(title string) {
	title = strings.ToValidUTF8(title, "�")
	w.b.setProperty(w.id, w.b.atoms.netWMName, w.b.atoms.utf8String, 8, []byte(title))
	w.b.setProperty(w.id, xproto.AtomWmName, xproto.AtomString, 8, latin1(title))
}

// SetCursor sets the cursor shown over the window.
func (w *Window) SetCursor(c platform.Cursor) {
	if !c.Valid() {
		panic(fmt.Sprintf("x11: SetCursor(%d): invalid cursor; use one of the platform.Cursor constants", c))
	}
	if w.Closed() || c == w.cursor {
		return
	}
	w.cursor = c
	xproto.ChangeWindowAttributes(w.b.conn, w.id, xproto.CwCursor, []uint32{uint32(w.b.cursorFor(c))})
}

// SetSize asks the window manager to resize the window to size (logical
// pixels, clamped to MinSize). The outcome arrives as a ConfigureEvent.
func (w *Window) SetSize(size platform.Size) {
	platform.CheckSize("SetSize size", size)
	if w.Closed() {
		return
	}
	w.opts.Size = w.opts.ClampSize(size)
	p := clampPhysical(platform.PhysicalSize(w.opts.Size, w.b.scale))
	if w.opts.FixedSize {
		// The hints must allow the new size before the request, or the
		// window manager rejects it.
		w.updateNormalHints()
	}
	xproto.ConfigureWindow(w.b.conn, w.id, xproto.ConfigWindowWidth|xproto.ConfigWindowHeight, []uint32{uint32(p.X), uint32(p.Y)})
}

// updateNormalHints writes WM_NORMAL_HINTS from the options at the current
// scale.
func (w *Window) updateNormalHints() {
	size := clampPhysical(platform.PhysicalSize(w.opts.Size, w.b.scale))
	var minSize image.Point
	if w.opts.MinSize != (platform.Size{}) {
		minSize = platform.PhysicalSize(w.opts.MinSize, w.b.scale)
	}
	w.b.setProperty(w.id, xproto.AtomWmNormalHints, xproto.AtomWmSizeHints, 32, sizeHints(size, minSize, w.opts.FixedSize))
}

// Close destroys the window. Queued events for it are dropped.
func (w *Window) Close() {
	if w.Closed() {
		return
	}
	b := w.b
	b.loop.Remove(w) // marks it closed and drops its queued events
	b.kbd.dropWindow(w)
	delete(b.windows, w.id)
	w.destroy()
}

// destroy frees the window's server resources.
func (w *Window) destroy() {
	w.pix = nil
	xproto.FreeGC(w.b.conn, w.gc)
	xproto.DestroyWindow(w.b.conn, w.id)
}

// Present stores the damaged pixels of fb in the retained copy and sends
// them to the server; see platform.Window.
func (w *Window) Present(fb *image.RGBA, damage []image.Rectangle) error {
	if w.Closed() {
		return nil
	}
	if w.Metrics().Scale == 0 {
		panic("x11: Present called before the window's first ConfigureEvent was delivered; present from the FrameEvent that follows it")
	}
	platform.CheckFramebuffer(fb, w.Metrics())
	if w.b.lost.Load() {
		return errConnLost
	}
	size := fb.Rect.Size()
	full := w.NeedsFull() || w.pixSize != size
	rects := platform.ClipDamage(damage, fb.Rect, full)
	if len(rects) == 0 {
		return nil // every damage rectangle was clipped away: nothing changed
	}
	if full {
		w.ClearNeedsFull()
		if w.pixSize != size {
			w.pix = make([]byte, 4*size.X*size.Y)
			w.pixSize = size
		}
	}
	stride := 4 * size.X
	for _, r := range rects {
		platform.CopyRGBAToBGRA(w.pix, stride, fb, r)
	}
	if !w.mapped {
		// An unmapped window has no contents, so drawing now would be
		// lost. The Expose that follows the map paints the retained copy.
		w.mapped = true
		xproto.MapWindow(w.b.conn, w.id)
		return nil
	}
	for _, r := range rects {
		w.putImage(r)
	}
	if w.b.lost.Load() {
		return errConnLost
	}
	return nil
}
