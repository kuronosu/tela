//go:build darwin

package cocoa

import (
	"fmt"
	"image"
	"slices"
	"sync/atomic"
	"time"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/keyboard"
	"github.com/kuronosu/tela/internal/platform/internal/loop"
)

// Window is a Cocoa window. All methods run on the main thread.
type Window struct {
	// WindowState holds the delivered metrics, closed and the frame
	// requests, and provides Metrics, State and RequestFrame.
	loop.WindowState

	b        *Backend
	ns       darwin.ID // TelaWindow (owned: +1)
	delegate darwin.ID // TelaWindowDelegate (owned: +1)
	view     darwin.ID // TelaView, owned by the window
	layer    darwin.ID // CALayer hosted by the view (owned: +1)
	opts     platform.WindowOptions
	parent   *Window

	visible   bool // the app wants it shown (not Hidden, SetVisible)
	shown     bool // ordered front
	presented bool // a Present gave it content
	minimized bool // in the Dock: no frames (ADR-185)
	focused   bool
	ctl       windowCtl // window control (windowctl_darwin.go)
	cursor    platform.Cursor
	gravity   darwin.ID   // the layer's contentsGravity, set at Present
	back      *image.RGBA // the frame as last presented (damage-only presents)

	// Input (input_darwin.go).
	held         keyboard.Held // keys whose press was delivered
	prevFlags    uint64        // modifierFlags of the last flagsChanged: or focus gain
	lastKey      keyStamp      // the last key event handled
	ptr          pointerState
	gesture      scrollGesture // the touchpad scroll gesture (input.go)
	tracking     darwin.ID     // the view's tracking area (the view owns it)
	cursorHidden bool          // this window's +[NSCursor hide] is in effect
	ime          imeWindow     // text input and IME (ime_darwin.go)
	dnd          windowDnD     // drag and drop (dnd_darwin.go)
	acc          windowAccess  // the accessibility bridge (access_darwin.go)
	// trans is a transparent window's input mask and click-through
	// (transparent_darwin.go, ADR-249).
	trans transparentState

	// Pacing (link_darwin.go).
	link        darwin.ID // CADisplayLink of the view (owned: +1)
	cvLink      uintptr   // CVDisplayLink (owned)
	linkID      uintptr   // key in registry.links: the CVDisplayLink context
	linkRunning bool
	linkSince   time.Duration // when the outstanding display signal was requested
	linkFired   atomic.Bool   // the CVDisplayLink ticked (set on CoreVideo's thread)
	ticks       atomic.Int64  // link ticks, for tests
	stalls      atomic.Int64  // signals the timer stood in for, for tests
}

var _ platform.Window = (*Window)(nil)

// NewWindow creates a window; it is shown at its first Present (or, for
// WindowOptions.Hidden, at the first Present after SetVisible(true)). Its
// first event is a ConfigureEvent with the content size AppKit granted, in
// points, and the backing scale factor.
func (b *Backend) NewWindow(opts platform.WindowOptions) (platform.Window, error) {
	opts = opts.Resolve()
	if b.closed {
		return nil, errClosed
	}
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)

	w := &Window{b: b, opts: opts, visible: !opts.Hidden, parent: b.parentOf(opts.Parent)}
	// A frameless or title bar overlay window is titled too, with a
	// full-size content view (windowctl_darwin.go): the system's shadow,
	// corners and resizing, and the overlay's traffic lights.
	style := uint(darwin.NSWindowStyleMaskTitled | darwin.NSWindowStyleMaskClosable |
		darwin.NSWindowStyleMaskMiniaturizable | darwin.NSWindowStyleMaskResizable)
	if opts.Frameless || opts.TitleBarOverlay {
		style |= darwin.NSWindowStyleMaskFullSizeContentView
	}
	if opts.FixedSize {
		style &^= darwin.NSWindowStyleMaskResizable
	}
	if opts.Transparent {
		style = transparentStyle(style) // transparent_darwin.go
	}
	rect := darwin.CGRect{Size: cgSize(opts.Size)}
	w.ns = darwin.Send(darwin.SendClass(app.window, sel.alloc), sel.initWithContentRect,
		rect, style, uint(darwin.NSBackingStoreBuffered), false)
	if w.ns == 0 {
		return nil, fmt.Errorf("cocoa: creating an NSWindow of %v points failed", opts.Size)
	}
	// The backend owns the window and releases it after Close.
	darwin.Send(w.ns, sel.setReleasedWhenClosed, false)
	darwin.Send(w.ns, sel.setTabbingMode, uintptr(darwin.NSWindowTabbingModeDisallowed))
	darwin.Send(w.ns, sel.setRestorable, false)
	w.setTitle(opts.Title)

	// The content view is a plain container holding TelaView (and, under
	// it, the backdrop's NSVisualEffectView when there is one), both kept
	// at its size.
	container := darwin.Send(darwin.SendClass(app.nsView, sel.alloc), sel.initWithFrame, rect)
	// Layer-backed, so that TelaView's layer is a sublayer of its own.
	darwin.Send(container, sel.wantsLayer, true)
	view := darwin.Send(darwin.SendClass(app.view, sel.alloc), sel.initWithFrame, rect)
	darwin.Send(view, wsel.setAutoresizingMask, uint(darwin.NSViewWidthSizable|darwin.NSViewHeightSizable))
	w.layer = darwin.Retain(darwin.SendClass(app.layer, sel.layer))
	// setLayer before setWantsLayer makes a layer-hosting view: AppKit
	// never draws into the layer, whose contents are Present's images.
	darwin.Send(view, sel.setLayer, w.layer)
	darwin.Send(view, sel.wantsLayer, true)
	w.gravity = darwin.CAGravityTopLeft
	darwin.Send(w.layer, sel.setContentsGravity, w.gravity)
	darwin.Send(container, wsel.addSubview, view)
	darwin.Release(view) // the container keeps it
	darwin.Send(w.ns, sel.setContentView, container)
	darwin.Release(container) // the window keeps it
	w.view, w.ctl.container = view, container
	darwin.Send(w.ns, sel.makeFirstResponder, view)
	w.addTrackingArea()
	w.registerDrop()
	w.delegate = darwin.Send(darwin.SendClass(app.windowDelegate, sel.alloc), sel.init)

	registry.Lock()
	registry.objects[w.delegate] = w
	registry.objects[w.view] = w
	registry.objects[w.ns] = w // the accessibility host (access_host_darwin.go)
	registry.Unlock()
	// The delegate comes after the setup, whose moves and resizes must
	// not queue events before the window's first ConfigureEvent.
	w.setupWindowCtl()
	darwin.Send(w.ns, sel.setDelegate, w.delegate)

	b.loop.Add(w)
	m := w.osMetrics()
	darwin.Send(w.layer, sel.setContentsScale, m.Scale)
	b.reconfigure(w, m)
	w.syncState()
	return w, nil
}

// parentOf returns the backend's window behind p (nil if p is nil or of
// another backend).
func (b *Backend) parentOf(p platform.Window) *Window {
	if w, ok := p.(*Window); ok && w.b == b && !w.Closed() {
		return w
	}
	return nil
}

func cgSize(s platform.Size) darwin.CGSize {
	return darwin.CGSize{Width: float64(s.W), Height: float64(s.H)}
}

// osMetrics returns the content view's size in points and the window's
// backing scale factor.
func (w *Window) osMetrics() platform.Metrics {
	bounds := darwin.SendRect(w.view, sel.bounds)
	scale := darwin.SendFloat(w.ns, sel.backingScale)
	if !(scale > 0) {
		scale = 1
	}
	return platform.Metrics{
		Size:  platform.Size{W: float32(bounds.Size.Width), H: float32(bounds.Size.Height)},
		Scale: scale,
	}
}

// updateHidden pauses frames while the app hides the window or it is in
// the Dock.
func (w *Window) updateHidden() { w.SetHidden(!w.visible || w.minimized) }

// show orders the window front and, at the first window, activates the
// app (an unbundled process launched from a terminal is not active). A
// child window is attached to its parent, a modal one disables its
// parent's buttons, and a Minimize made before the window ever showed is
// applied now. A window whose modal child is shown is ordered front
// behind it, and the child keeps the focus.
func (w *Window) show() {
	if w.modalChild() != nil {
		darwin.Send(w.ns, wsel.orderFront, darwin.ID(0))
	} else {
		darwin.Send(w.ns, sel.makeKeyAndOrderFront, darwin.ID(0))
	}
	w.shown = true
	if !w.b.activated {
		w.b.activated = true
		darwin.Send(app.ns, sel.activate, true)
	}
	w.attach()
	if w.opts.Modal {
		w.parent.updateModal()
	}
	if w.ctl.minimizeOnShow {
		w.ctl.minimizeOnShow = false
		darwin.Send(w.ns, wsel.miniaturize, darwin.ID(0))
	}
	w.syncState()
}

func (w *Window) setTitle(title string) {
	s := darwin.NSString(title)
	darwin.Send(w.ns, sel.setTitle, s)
	darwin.Release(s)
}

// SetTitle sets the window title.
func (w *Window) SetTitle(title string) {
	if w.Closed() {
		return
	}
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	w.setTitle(title)
}

// SetCursor sets the cursor shown over the content area, through the
// view's cursor rectangle (resetCursorRects), and at once while the
// pointer is in the view. CursorNone hides the cursor while the pointer is
// in the view (+[NSCursor hide], balanced by unhide when it leaves, the
// cursor changes or the window closes).
func (w *Window) SetCursor(c platform.Cursor) {
	if !c.Valid() {
		panic(fmt.Sprintf("cocoa: SetCursor(%d): invalid cursor; use one of the platform.Cursor constants", c))
	}
	if w.Closed() || c == w.cursor {
		return
	}
	w.cursor = c
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	darwin.Send(w.ns, sel.invalidateCursorRects, w.view)
	if w.ptr.inside && w.focused && c != platform.CursorNone {
		darwin.Send(w.nsCursor(), isel.set)
	}
	w.updateCursorHidden()
}

// nsCursor returns the NSCursor of the window's cursor.
func (w *Window) nsCursor() darwin.ID {
	name := cursorSelector(w.shownCursor())
	if name == "" {
		name = "arrowCursor"
	}
	s, ok := cursorSels[name]
	if !ok {
		s = darwin.Sel(name)
		cursorSels[name] = s
	}
	return darwin.SendClass(app.cursor, s)
}

var cursorSels = map[string]darwin.SEL{}

// SetSize asks AppKit to resize the content area to size (points, clamped
// to the size limits). The outcome arrives as a ConfigureEvent
// (windowDidResize:, sent during the call and delivered after the handler
// returns).
func (w *Window) SetSize(size platform.Size) {
	platform.CheckSize("SetSize size", size)
	if w.Closed() {
		return
	}
	w.opts.Size = w.opts.ClampSize(size)
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	w.resizeContent(w.opts.Size)
}

// SetMinSize and SetMaxSize change the content size limits AppKit applies
// to user resizes (zero is no limit; the minimum wins), and resize the
// window into the new limits when it is outside them.
func (w *Window) SetMinSize(size platform.Size) {
	platform.CheckSize("SetMinSize size", size)
	if !w.Closed() && size != w.opts.MinSize {
		w.opts.MinSize = size
		w.limitsChanged()
	}
}

func (w *Window) SetMaxSize(size platform.Size) {
	platform.CheckSize("SetMaxSize size", size)
	if !w.Closed() && size != w.opts.MaxSize {
		w.opts.MaxSize = size
		w.limitsChanged()
	}
}

func (w *Window) limitsChanged() {
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	w.applyLimits()
	if w.ctl.fullscreen || w.ctl.fsTransition {
		return
	}
	cur := w.osMetrics().Size
	if c := w.opts.ClampSize(cur); c != cur {
		w.resizeContent(c)
	}
}

// SetVisible hides the window (orderOut:) or shows it again; a window that
// never presented is shown at its first Present after SetVisible(true).
// No frames are delivered while it is hidden.
func (w *Window) SetVisible(visible bool) {
	if w.Closed() || w.visible == visible {
		return
	}
	w.visible = visible
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	switch {
	case !visible && w.shown:
		// Not shown before orderOut:, which may make the parent key at
		// once: the parent must not bring this child back (raiseModal).
		wasKey := darwin.SendBool(w.ns, wsel.isKeyWindow)
		w.shown = false
		p := w.parent
		modal := w.opts.Modal && p != nil && !p.Closed()
		if modal {
			p.updateModal()
		}
		w.detach()
		darwin.Send(w.ns, sel.orderOut, darwin.ID(0))
		if modal && wasKey && p.shown {
			darwin.Send(p.ns, wsel.makeKeyWindow)
		}
		w.syncState()
	case visible && w.presented && !w.shown:
		w.show()
	}
	w.updateHidden()
}

// Close closes the window and its child windows (first, in creation
// order). Queued events for them are dropped; their objects are released
// at the next pass.
func (w *Window) Close() {
	if w.Closed() {
		return
	}
	for _, c := range slices.Clone(w.b.loop.Windows()) {
		if c.parent == w {
			c.Close()
		}
	}
	w.b.loop.Remove(w) // marks it closed and drops its queued events
	w.updateCursorHidden()
	imePool := darwin.PoolPush()
	w.closeIME()
	darwin.PoolPop(imePool)
	w.releaseLink()
	w.releaseTransparent()
	w.closeDnD()
	accPool := darwin.PoolPush()
	w.closeAccess()
	darwin.PoolPop(accPool)
	registry.Lock()
	delete(registry.objects, w.delegate)
	delete(registry.objects, w.view)
	delete(registry.objects, w.ns)
	registry.Unlock()
	pool := darwin.PoolPush()
	wasKey := darwin.SendBool(w.ns, wsel.isKeyWindow)
	darwin.Send(w.ns, sel.setDelegate, darwin.ID(0))
	w.detach()
	darwin.Send(w.ns, sel.orderOut, darwin.ID(0))
	darwin.Send(w.ns, sel.close)
	if p := w.parent; w.opts.Modal && p != nil && !p.Closed() {
		// The parent has the input again, and the focus the child had.
		p.updateModal()
		if wasKey && p.shown {
			darwin.Send(p.ns, wsel.makeKeyWindow)
		}
	}
	darwin.PoolPop(pool)
	w.b.releaseLater(w.layer, w.delegate, w.ns, w.ctl.effect)
	w.ctl.effect = 0
	w.back = nil
}

// setFocused reports a change of the key window status. A window that
// loses it first releases its held keys (synthetic releases, in press
// order) and ends its IME session; one that gains it starts from the
// modifiers held now, whose release then gives no event (no press was
// delivered), and starts the session of its text field after the
// FocusEvent (the order of the headless backend).
func (w *Window) setFocused(focused bool) {
	if w.focused == focused {
		return
	}
	if focused {
		w.prevFlags = darwin.SendUint(darwin.ID(app.event), isel.modifierFlags)
		w.focused = true
		w.b.enqueue(w, platform.FocusEvent{Focused: true})
		w.syncIME()
		return
	}
	for _, e := range w.held.ReleaseAll(w.b.Now()) {
		w.b.enqueue(w, e)
	}
	w.focused = false
	w.syncIME()
	w.b.enqueue(w, platform.FocusEvent{Focused: false})
}

// --- AppKit callbacks ---

// windowDelegateMethods are the methods of TelaWindowDelegate. Each finds
// its window by the delegate, queues what changed and runs a delivery pass
// (a no-op when the change happened inside the handler: the pass that is
// running delivers it in its follow-up).
func windowDelegateMethods() []darwin.Method {
	on := func(name string, f func(w *Window)) darwin.Method {
		return darwin.Method{Sel: darwin.Sel(name), Fn: func(self darwin.ID, _ darwin.SEL, _ darwin.ID) {
			if w := windowFor(self); w != nil && !w.Closed() {
				w.b.guard(func() {
					f(w)
					w.b.pump()
				})
			}
		}}
	}
	return []darwin.Method{
		// The close button asks; closing is the app's decision. A window
		// with a modal child brings the child forward instead (its
		// buttons are disabled, but Command-W may still ask).
		{Sel: darwin.Sel("windowShouldClose:"), Fn: func(self darwin.ID, _ darwin.SEL, _ darwin.ID) bool {
			if w := windowFor(self); w != nil && !w.Closed() {
				w.b.guard(func() {
					if !w.raiseModal() {
						w.b.enqueue(w, platform.CloseRequestedEvent{})
					}
					w.b.pump()
				})
			}
			return false
		}},
		// Every zoom (ours, the green button, the Window menu) is
		// announced here before the frame changes: the zoom state is
		// tracked from it, not read from isZoomed (windowctl_darwin.go).
		// The NSRect toFrame argument is not read: it arrives in
		// floating-point registers (arm64) or on the stack (amd64) after
		// the window, and is left alone there.
		{Sel: darwin.Sel("windowShouldZoom:toFrame:"), Fn: func(self darwin.ID, _ darwin.SEL, _ darwin.ID) bool {
			if w := windowFor(self); w != nil && !w.Closed() {
				w.b.guard(w.zoomStarting)
			}
			return true
		}},
		// A live resize delivers its ConfigureEvent and forced FrameEvent
		// here, so the window is repainted before AppKit continues. The
		// state (a zoom ends in a resize) goes first.
		on("windowDidResize:", func(w *Window) {
			w.syncState()
			w.b.reconfigure(w, w.osMetrics())
			w.passIfFaint() // a transparent window under a resting cursor
		}),
		on("windowDidMove:", func(w *Window) {
			w.syncState()
			w.passIfFaint()
		}),
		on("windowDidChangeBackingProperties:", func(w *Window) {
			m := w.osMetrics()
			darwin.Send(w.layer, sel.setContentsScale, m.Scale)
			w.b.reconfigure(w, m)
		}),
		// A window with a shown modal child does not keep the focus: the
		// child takes it (a click on the parent, Command-`, Mission
		// Control), and the parent never reports it.
		on("windowDidBecomeKey:", func(w *Window) {
			if !w.raiseModal() {
				w.setFocused(true)
			}
		}),
		on("windowDidResignKey:", func(w *Window) { w.setFocused(false) }),
		on("windowDidChangeScreen:", func(w *Window) { w.followScreen() }),
		on("windowDidMiniaturize:", (*Window).syncState),
		on("windowDidDeminiaturize:", (*Window).syncState),
		on("windowWillEnterFullScreen:", func(w *Window) { w.fullscreenWill(true) }),
		on("windowWillExitFullScreen:", func(w *Window) { w.fullscreenWill(false) }),
		on("windowDidEnterFullScreen:", func(w *Window) { w.fullscreenDid(true, false) }),
		on("windowDidExitFullScreen:", func(w *Window) { w.fullscreenDid(false, false) }),
		on("windowDidFailToEnterFullScreen:", func(w *Window) { w.fullscreenDid(false, true) }),
		on("windowDidFailToExitFullScreen:", func(w *Window) { w.fullscreenDid(true, true) }),
	}
}

// viewMethods are the methods of TelaView, the content view.
func viewMethods() []darwin.Method {
	yes := func(darwin.ID, darwin.SEL) bool { return true }
	return []darwin.Method{
		// Flipped: y grows downward, like Tela's coordinates.
		{Sel: darwin.Sel("isFlipped"), Fn: yes},
		// Opaque unless a backdrop or the desktop (a transparent
		// window) shows through the frame's alpha.
		{Sel: darwin.Sel("isOpaque"), Fn: func(self darwin.ID, _ darwin.SEL) bool {
			w := windowFor(self)
			return w == nil || w.ctl.effect == 0 && !w.opts.Transparent
		}},
		// A press never moves the window by itself (a transparent title
		// bar would, over a view that is not opaque): caption regions
		// start the move (windowctl_darwin.go).
		{Sel: darwin.Sel("mouseDownCanMoveWindow"), Fn: func(darwin.ID, darwin.SEL) bool { return false }},
		{Sel: darwin.Sel("acceptsFirstResponder"), Fn: yes},
		// The NSView display link's callback (link_darwin.go).
		{Sel: darwin.Sel("telaDisplayLinkTick:"), Fn: func(self darwin.ID, _ darwin.SEL, _ darwin.ID) {
			if w := windowFor(self); w != nil && !w.Closed() {
				w.b.guard(w.linkTick)
			}
		}},
		{Sel: darwin.Sel("resetCursorRects"), Fn: func(self darwin.ID, _ darwin.SEL) {
			if w := windowFor(self); w != nil && !w.Closed() {
				w.b.guard(func() {
					darwin.Send(self, sel.addCursorRect, darwin.SendRect(self, sel.bounds), w.nsCursor())
				})
			}
		}},
	}
}
