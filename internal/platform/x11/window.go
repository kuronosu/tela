//go:build linux

package x11

import (
	"fmt"
	"image"
	"os"
	"strings"
	"time"

	"github.com/jezek/xgb/xproto"

	"github.com/kuronosu/tela/internal/platform"
)

// maxWindowSide is the largest X11 window side (a CARD16 that the server
// also treats as INT16 in coordinates).
const maxWindowSide = 32767

// windowEvents is the event mask of every window.
const windowEvents = xproto.EventMaskExposure |
	xproto.EventMaskStructureNotify |
	xproto.EventMaskButtonPress |
	xproto.EventMaskButtonRelease |
	xproto.EventMaskPointerMotion |
	xproto.EventMaskEnterWindow |
	xproto.EventMaskLeaveWindow |
	xproto.EventMaskFocusChange

// Window is an X11 top-level window. All methods run on the UI thread.
type Window struct {
	b    *Backend
	id   xproto.Window
	gc   xproto.Gcontext
	opts platform.WindowOptions

	closed bool
	mapped bool // MapWindow was sent (at the first Present)
	iconic bool // unmapped by the window manager after being mapped

	osSize           image.Point      // latest physical size from the server
	metrics          platform.Metrics // delivered by the latest ConfigureEvent
	configurePending bool             // a coalesced ConfigureEvent is queued

	frameRequested bool
	forcedFrame    bool // a ConfigureEvent was delivered; frame due now
	hasFramed      bool
	lastFrame      time.Duration

	// Retained copy of the window content in the server's pixel layout
	// (BGRX, stride 4*pixSize.X), used to answer Expose.
	needsFull bool
	pix       []byte
	pixSize   image.Point
	expose    image.Rectangle // Expose rectangles accumulated until Count == 0

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
	values := []uint32{xproto.BackPixmapNone, xproto.GravityNorthWest, windowEvents, uint32(b.cursorFor(platform.CursorDefault))}
	err = xproto.CreateWindowChecked(b.conn, b.depth, id, b.root, 0, 0, uint16(size.X), uint16(size.Y), 0,
		xproto.WindowClassInputOutput, b.visual, mask, values).Check()
	if err != nil {
		return nil, fmt.Errorf("x11: CreateWindow: %w", err)
	}
	// GraphicsExposures off: PutImage never generates them, but the
	// default GC would make any later CopyArea send NoExpose events.
	xproto.CreateGC(b.conn, gc, xproto.Drawable(id), xproto.GcGraphicsExposures, []uint32{0})

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
	b.order = append(b.order, w)
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

// visible reports whether frames can be delivered: the window has
// delivered metrics with a non-empty size and is not iconified. Frames
// requested while it is iconified stay pending until it is mapped again,
// like frame callbacks of a hidden Wayland surface.
func (w *Window) visible() bool {
	if w.closed || w.iconic || w.metrics.Scale == 0 {
		return false
	}
	p := w.metrics.Physical()
	return p.X > 0 && p.Y > 0
}

// frameDue reports whether a FrameEvent should be delivered at now.
func (w *Window) frameDue(now time.Duration) bool {
	if !w.visible() {
		return false
	}
	if w.forcedFrame {
		return true
	}
	return w.frameRequested && (!w.hasFramed || now >= w.lastFrame+FrameInterval)
}

// Metrics returns the metrics of the latest delivered ConfigureEvent.
func (w *Window) Metrics() platform.Metrics { return w.metrics }

// RequestFrame requests a FrameEvent; see platform.Window.
func (w *Window) RequestFrame() {
	if !w.closed {
		w.frameRequested = true
	}
}

// SetTitle sets WM_NAME and _NET_WM_NAME.
func (w *Window) SetTitle(title string) {
	if !w.closed {
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
	if w.closed || c == w.cursor {
		return
	}
	w.cursor = c
	xproto.ChangeWindowAttributes(w.b.conn, w.id, xproto.CwCursor, []uint32{uint32(w.b.cursorFor(c))})
}

// SetSize asks the window manager to resize the window to size (logical
// pixels, clamped to MinSize). The outcome arrives as a ConfigureEvent.
func (w *Window) SetSize(size platform.Size) {
	platform.CheckSize("SetSize size", size)
	if w.closed {
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
	if w.closed {
		return
	}
	b := w.b
	w.destroy()
	delete(b.windows, w.id)
	for i, x := range b.order {
		if x == w {
			b.order = append(b.order[:i], b.order[i+1:]...)
			break
		}
	}
	q := b.queue[:0]
	for _, it := range b.queue {
		if it.w != w {
			q = append(q, it)
		}
	}
	clear(b.queue[len(q):])
	b.queue = q
}

// destroy frees the window's server resources.
func (w *Window) destroy() {
	if w.closed {
		return
	}
	w.closed = true
	w.frameRequested, w.forcedFrame = false, false
	w.pix = nil
	xproto.FreeGC(w.b.conn, w.gc)
	xproto.DestroyWindow(w.b.conn, w.id)
}

// Present stores the damaged pixels of fb in the retained copy and sends
// them to the server; see platform.Window.
func (w *Window) Present(fb *image.RGBA, damage []image.Rectangle) error {
	if w.closed {
		return nil
	}
	if w.metrics.Scale == 0 {
		panic("x11: Present called before the window's first ConfigureEvent was delivered; present from the FrameEvent that follows it")
	}
	platform.CheckFramebuffer(fb, w.metrics)
	if w.b.lost.Load() {
		return errConnLost
	}
	size := fb.Rect.Size()
	full := w.needsFull || w.pixSize != size
	rects := platform.ClipDamage(damage, fb.Rect, full)
	if len(rects) == 0 {
		return nil // every damage rectangle was clipped away: nothing changed
	}
	if full {
		w.needsFull = false
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
