//go:build linux

package wayland

import (
	"fmt"
	"image"
	"slices"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/loop"
	"github.com/kuronosu/tela/internal/platform/wayland/protocols"
)

// maxDamageRects bounds the damage_buffer requests of one Present; beyond
// it the damage is sent as its bounding box.
const maxDamageRects = 32

// frameInterval paces frame requests that cannot wait for a frame callback
// (see paceFrame), and defines "has not drawn recently": a request made more
// than one interval after the previous FrameEvent is delivered on the next
// loop iteration. 60 Hz, like the X11 and headless backends.
const frameInterval = time.Second / 60

// Window is a Wayland toplevel: wl_surface + xdg_surface + xdg_toplevel.
// Its methods must be called on the UI thread.
type Window struct {
	// WindowState holds the delivered metrics, closed, the frame requests
	// and the frame callback gate (VsyncRequested / VsyncDone), and
	// provides Metrics and RequestFrame.
	loop.WindowState

	b    *Backend
	opts platform.WindowOptions

	surface    *protocols.WlSurface
	xdgSurface *protocols.XdgSurface
	toplevel   *protocols.XdgToplevel
	decoration *protocols.ZxdgToplevelDecorationV1

	cursor platform.Cursor

	// Configure state. xdg_toplevel.configure is latched in pendingW/H and
	// pendingStates and applied by the xdg_surface.configure that ends
	// the sequence.
	pendingW, pendingH int32
	pendingStates      toplevelStates
	states             toplevelStates
	configured         bool // the first xdg_surface.configure arrived
	sizes              sizeChoice

	// Focus: the xdg_toplevel activated state and the number of seats
	// whose keyboard focus is on the window; focused is what the app was
	// told (see updateFocus).
	activated bool
	kbdFocus  int
	focused   bool

	// Scale: see scaleFor.
	preferredScale int32 // wl_surface.preferred_buffer_scale; 0 = not received
	outputs        []*output
	scale          int32

	// announced is the metrics of the latest queued ConfigureEvent (the
	// latest delivered one is Metrics).
	announced platform.Metrics

	// Pending acknowledgement: the serial of the latest configure not yet
	// acked, and the logical size the next buffer must have to ack it.
	// xdg_surface.ack_configure is sent right before the commit of the
	// first buffer of that size (the Present following the ConfigureEvent),
	// so the compositor never sees an ack followed by a buffer of the old
	// size.
	ackPending bool
	ackSerial  uint32
	ackSize    image.Point

	chain       swapchain
	bufferScale int32 // scale committed with set_buffer_scale (protocol default 1)
	mapped      bool  // a buffer was committed
}

var _ platform.Window = (*Window)(nil)

// NewWindow creates a toplevel. Its first event, a ConfigureEvent with the
// size the compositor granted, follows the compositor's initial configure;
// it is mapped (shown) by its first Present.
func (b *Backend) NewWindow(opts platform.WindowOptions) (platform.Window, error) {
	opts = opts.Resolve()
	if b.released {
		return nil, fmt.Errorf("wayland: NewWindow called after Run returned; the backend's connection is closed")
	}
	if err := b.conn.Err(); err != nil {
		return nil, err
	}
	w := &Window{b: b, opts: opts, bufferScale: 1}
	size := logicalPoint(opts.Size)
	w.sizes = sizeChoice{
		current:   size,
		stateless: size,
		min:       ceilPoint(opts.MinSize),
		fixed:     opts.FixedSize,
	}
	w.scale = 1
	if b.canScale() {
		w.scale = b.maxOutputScale()
	}

	w.surface = b.compositor.CreateSurface()
	w.surface.SetEnterHandler(w.onSurfaceEnter)
	w.surface.SetLeaveHandler(w.onSurfaceLeave)
	w.surface.SetPreferredBufferScaleHandler(func(e protocols.WlSurfacePreferredBufferScaleEvent) {
		w.preferredScale = max(1, e.Factor)
		w.updateScale()
	})
	w.xdgSurface = b.wm.GetXdgSurface(w.surface)
	w.xdgSurface.SetConfigureHandler(w.onConfigure)
	w.toplevel = w.xdgSurface.GetToplevel()
	w.toplevel.SetConfigureHandler(func(e protocols.XdgToplevelConfigureEvent) {
		w.pendingW, w.pendingH = e.Width, e.Height
		w.pendingStates = decodeStates(e.States)
	})
	w.toplevel.SetCloseHandler(func(protocols.XdgToplevelCloseEvent) {
		// A close before the first configure would be the window's first
		// event; the contract says that is a ConfigureEvent, so drop it.
		if w.configured {
			b.enqueue(w, platform.CloseRequestedEvent{})
		}
	})

	if opts.Title != "" {
		w.toplevel.SetTitle(opts.Title)
	}
	appID := opts.AppID
	if appID == "" {
		appID = b.appID
	}
	w.toplevel.SetAppId(appID)
	w.applySizeHints()
	if b.decoration != nil {
		// Ask for server-side decorations. The compositor may still pick
		// client-side; drawing them is a later task, so the window is
		// then undecorated (the configure event is ignored).
		w.decoration = b.decoration.GetToplevelDecoration(w.toplevel)
		w.decoration.SetMode(protocols.ZxdgToplevelDecorationV1ModeServerSide)
	}
	// The initial commit, without a buffer, asks for the first configure.
	w.surface.Commit()

	b.loop.Add(w)
	b.surfaces[w.surface] = w
	return w, nil
}

// applySizeHints sends min/max size (double-buffered: applied by the next
// commit). FixedSize pins both to the current size.
func (w *Window) applySizeHints() {
	min := w.sizes.min
	var max image.Point
	if w.opts.FixedSize {
		min, max = w.sizes.stateless, w.sizes.stateless
	}
	w.toplevel.SetMinSize(int32(min.X), int32(min.Y))
	w.toplevel.SetMaxSize(int32(max.X), int32(max.Y))
}

// onConfigure ends a configure sequence: it applies the latched toplevel
// state and decides what the app must be told.
func (w *Window) onConfigure(e protocols.XdgSurfaceConfigureEvent) {
	if w.Closed() {
		return
	}
	st := w.pendingStates
	size := w.sizes.choose(w.pendingW, w.pendingH, st)
	w.states = st
	first := !w.configured
	w.configured = true

	target := platform.Metrics{Size: sizeOf(size), Scale: float64(w.scale)}
	switch {
	case first || target != w.announced:
		w.announce(target)
		w.ackPending, w.ackSerial, w.ackSize = true, e.Serial, size
	case w.ackPending:
		// An earlier configure is waiting for its frame and this one
		// asks for the same size: the frame acks the newer serial, which
		// supersedes the older one.
		w.ackSerial = e.Serial
	default:
		// Nothing for the app to redraw (activation, a no-op resize):
		// ack and commit now, so the compositor applies the new state
		// without waiting for the next frame.
		w.xdgSurface.AckConfigure(e.Serial)
		if w.mapped {
			w.surface.Commit()
		}
	}
	w.activated = st.activated
	w.updateFocus()
}

// updateFocus reports a focus change to the app. The window is focused
// while it is activated (xdg_toplevel) or has keyboard focus
// (wl_keyboard.enter), whichever comes first: compositors send the two in
// either order, and the FocusEvent must precede the first key. It is
// unfocused once both are gone; the keyboard's leave queues the synthetic
// releases of held keys before calling this, so they precede the
// FocusEvent (ADR-050). A window that stays activated while another
// surface (a launcher, a lock screen) takes the keyboard keeps focus but
// gets its releases at once.
func (w *Window) updateFocus() {
	if w.Closed() || !w.configured {
		return // the first event must be the ConfigureEvent; onConfigure calls again
	}
	f := w.activated || w.kbdFocus > 0
	if f == w.focused {
		return
	}
	w.focused = f
	w.b.enqueue(w, platform.FocusEvent{Focused: f})
}

// announce queues a ConfigureEvent with new metrics.
func (w *Window) announce(m platform.Metrics) {
	w.announced = m
	w.b.enqueue(w, platform.ConfigureEvent{Metrics: m})
}

func (w *Window) onSurfaceEnter(e protocols.WlSurfaceEnterEvent) {
	o := w.b.outputFor(e.Output)
	if o == nil || slices.Contains(w.outputs, o) {
		return
	}
	w.outputs = append(w.outputs, o)
	w.updateScale()
}

func (w *Window) onSurfaceLeave(e protocols.WlSurfaceLeaveEvent) {
	o := w.b.outputFor(e.Output)
	if i := slices.Index(w.outputs, o); o != nil && i >= 0 {
		w.outputs = slices.Delete(w.outputs, i, i+1)
		// Keep the scale when the surface left every output (it is
		// hidden or moving): rendering for no output helps nobody.
		if len(w.outputs) > 0 {
			w.updateScale()
		}
	}
}

// updateScale recomputes the buffer scale and reports a change to the app
// once the window is configured. The new scale is committed with
// set_buffer_scale together with the first buffer rendered for it.
func (w *Window) updateScale() {
	if w.Closed() {
		return
	}
	if !w.b.canScale() {
		return
	}
	s := scaleFor(w.preferredScale, w.outputs, w.scale)
	if s == w.scale {
		return
	}
	w.scale = s
	if !w.configured {
		return // the first ConfigureEvent will carry it
	}
	w.announce(platform.Metrics{Size: w.announced.Size, Scale: float64(s)})
}

// paceFrame is the loop's Pace hook: it handles a frame request that is
// not due at now (visible, not forced, no callback outstanding). With a
// buffer on screen it requests a frame callback with a commit that
// attaches no new buffer, so the FrameEvent follows the compositor's pacing
// (vsync) and stops while the window is hidden; the commit changes nothing
// the user can see (see "Frame pacing" in doc.go).
//
// Before the window shows a buffer, or while a configure waits for the
// buffer that acks it (a commit cannot apply the new state yet), it returns
// false: the loop falls back to interval pacing like the X11 backend, and
// the frame's due time becomes the poll timeout.
func (w *Window) paceFrame(time.Duration) bool {
	if w.mapped && !w.ackPending {
		w.requestCallback()
		w.surface.Commit()
		return true
	}
	return false
}

// requestCallback requests a wl_surface.frame callback, committed by the
// caller, unless one is outstanding. The callback is the loop's display
// signal: a requested frame waits for its done event, then is due.
func (w *Window) requestCallback() {
	if w.VsyncPending() {
		return
	}
	w.VsyncRequested()
	cb := w.surface.Frame()
	cb.SetDoneHandler(func(protocols.WlCallbackDoneEvent) {
		// The compositor destroys the callback after done.
		w.VsyncDone()
	})
}

// --- platform.Window ---
//
// Metrics and RequestFrame come from loop.WindowState. A requested frame is
// delivered at the next frame callback if one is outstanding (the
// compositor paces it to the display, and stops it while the window is
// hidden), on the next loop iteration if the window has not had a frame
// recently, otherwise at a frame callback requested for it (see paceFrame).

// Present shows fb; see platform.Window. The pixels are copied into a
// wl_shm buffer the compositor does not hold, the buffer is attached with
// its damage and a frame callback, and the surface is committed. The
// requests are flushed by the loop when the handler returns.
func (w *Window) Present(fb *image.RGBA, damage []image.Rectangle) error {
	if w.Closed() {
		return nil
	}
	m := w.Metrics()
	if m.Scale == 0 {
		panic("wayland: Present called before the window's first ConfigureEvent was delivered; present from the FrameEvent that follows it")
	}
	platform.CheckFramebuffer(fb, m)
	b := w.b
	if err := b.conn.Err(); err != nil {
		return err
	}
	size := fb.Rect.Max
	full := w.NeedsFull() || w.chain.prev == nil || w.chain.size != size
	rects := platform.ClipDamage(damage, fb.Rect, full)
	if len(rects) == 0 {
		return nil // everything was clipped away: nothing changed
	}
	if w.chain.alloc == nil {
		w.chain.alloc = func(size image.Point) (*shmBuffer, error) {
			buf, err := allocShm(b.shm, b.format, size)
			if err != nil {
				return nil, err
			}
			buf.wl.SetReleaseHandler(func(protocols.WlBufferReleaseEvent) { w.chain.released(buf) })
			return buf, nil
		}
		w.chain.copyRect = copyXRGB
		if b.format == protocols.WlShmFormatXbgr8888 {
			w.chain.copyRect = copyXBGR
		}
	}
	buf, err := w.chain.acquire(size)
	if err != nil {
		return err
	}
	w.chain.fill(buf, fb, rects)
	w.ClearNeedsFull()

	logical := logicalPoint(m.Size)
	if w.ackPending && logical == w.ackSize {
		w.xdgSurface.AckConfigure(w.ackSerial)
		w.ackPending = false
	}
	if scale := int32(m.Scale); scale != w.bufferScale {
		w.surface.SetBufferScale(scale) // scale > 1 implies version 3+ (canScale)
		w.bufferScale = scale
	}
	w.surface.Attach(buf.wl, 0, 0)
	w.damage(rects)
	w.requestCallback()
	w.surface.Commit()
	w.chain.submitted(buf, rects)
	w.mapped = true
	return nil
}

// damage sends the damaged rectangles: damage_buffer in buffer (physical)
// pixels on wl_compositor 4+, else wl_surface.damage in surface (logical)
// pixels, rounded outward.
func (w *Window) damage(rects []image.Rectangle) {
	if len(rects) > maxDamageRects {
		u := rects[0]
		for _, r := range rects[1:] {
			u = u.Union(r)
		}
		rects = []image.Rectangle{u}
	}
	if w.surface.Version() >= 4 {
		for _, r := range rects {
			w.surface.DamageBuffer(int32(r.Min.X), int32(r.Min.Y), int32(r.Dx()), int32(r.Dy()))
		}
		return
	}
	s := max(1, w.bufferScale)
	for _, r := range rects {
		x0, y0 := int32(r.Min.X)/s, int32(r.Min.Y)/s
		x1, y1 := (int32(r.Max.X)+s-1)/s, (int32(r.Max.Y)+s-1)/s
		w.surface.Damage(x0, y0, x1-x0, y1-y0)
	}
}

// SetTitle sets the window title.
func (w *Window) SetTitle(title string) {
	if w.Closed() {
		return
	}
	w.toplevel.SetTitle(title)
}

// SetCursor sets the cursor shown over the window. It takes effect at once
// if a pointer is over the window, otherwise at the next pointer enter.
func (w *Window) SetCursor(c platform.Cursor) {
	if !c.Valid() {
		panic(fmt.Sprintf("wayland: SetCursor(%d): invalid cursor; use one of the platform.Cursor constants", c))
	}
	if w.Closed() || c == w.cursor {
		return
	}
	w.cursor = c
	for _, s := range w.b.seats {
		if s.ptr.focus == w {
			s.applyCursor(c)
		}
	}
}

// SetSize asks for a new logical size. Wayland has no request for it: a
// floating window simply picks its size, so the backend adopts it (raised
// to MinSize) and reports it with a ConfigureEvent. A maximized, fullscreen
// or tiled window keeps the size the compositor imposed, and the request is
// ignored (like winit's request_surface_size).
func (w *Window) SetSize(size platform.Size) {
	platform.CheckSize("SetSize size", size)
	if w.Closed() {
		return
	}
	p := raise(logicalPoint(size), w.sizes.min)
	if p.X == 0 || p.Y == 0 {
		return
	}
	if w.states.constrained() {
		return
	}
	w.sizes.stateless = p
	if w.opts.FixedSize {
		w.applySizeHints() // committed with the next buffer
	}
	if !w.configured {
		// Not configured yet: the size answers a 0x0 initial configure.
		w.sizes.current = p
		return
	}
	w.sizes.current = p
	if w.ackPending {
		// The pending configure is acked by the frame of the new size;
		// a floating window may answer a configure with any size.
		w.ackSize = p
	}
	if m := (platform.Metrics{Size: sizeOf(p), Scale: w.announced.Scale}); m != w.announced {
		w.announce(m)
	}
}

// Close destroys the window. Queued events for it are dropped.
func (w *Window) Close() {
	if w.Closed() {
		return
	}
	b := w.b
	b.loop.Remove(w) // marks it closed and drops its queued events
	for _, s := range b.seats {
		if s.ptr.focus == w {
			s.ptr = pointerState{mods: s.ptr.mods}
		}
		if s.kbd != nil {
			s.kbd.dropWindow(w)
		}
	}
	delete(b.surfaces, w.surface)

	// Destruction order matters: the decoration before its toplevel
	// (zxdg_toplevel_decoration_v1.orphaned), the toplevel before its
	// xdg_surface, the xdg_surface before its wl_surface. Buffers last:
	// destroying a wl_buffer the compositor still holds is allowed once
	// nothing shows it.
	if w.decoration != nil {
		w.decoration.Destroy()
	}
	w.toplevel.Destroy()
	w.xdgSurface.Destroy()
	w.surface.Destroy()
	w.chain.destroy()
}
