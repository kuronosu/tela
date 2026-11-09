//go:build darwin

package cocoa

import (
	"image"
	"math"
	"time"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// Window control (phase 15, ADR-224): AppKit's requests and the state its
// window delegate reports, frameless windows, custom title bars, child and
// modal windows, and the vibrancy backdrop. See "Window control" in the
// package documentation; the pure rules are in windowctl.go.
//
// Quirks handled, mostly as winit (window_delegate.rs) and Electron
// (native_window_mac.mm) do:
//
//   - zoom: toggles, so SetMaximized sends it only when the zoom differs,
//     once per request (a zoom that AppKit ignored must not be sent again
//     and toggle back). A non-resizable (FixedSize) window does not zoom:
//     AppKit disables its zoom button and isZoomed is meaningless there.
//   - isZoomed only compares the frame with the standard frame, which the
//     size limits clamp: a window resized to its maximum size is "zoomed"
//     though nobody zoomed it. The zoom is tracked instead: a zoom (ours,
//     the green button, the Window menu, a caption double click) is
//     announced by windowShouldZoom:toFrame:, goes the way AppKit's
//     isZoomed says, and ends when isZoomed reaches the other side; any
//     other resize keeps the zoom only while isZoomed still holds.
//   - toggleFullScreen: also toggles, and starts an animated transition
//     between windowWill… and windowDid…EnterFullScreen: (or the Fail
//     notifications). A request made during one waits for its end and then
//     for a short settling time: a toggleFullScreen: sent from the Did
//     notification itself was lost on macOS 26 (the window stayed in full
//     screen). One that AppKit refuses (no transition starts within a
//     second) is given up. AppKit refuses full screen for non-resizable
//     windows, so a FixedSize window is made resizable for its stay there
//     (winit).
//   - Full-screen transitions resize the window, and isMiniaturized read
//     around them was seen true on macOS 26: the state stays the
//     pre-full-screen zoom, position and minimization until the window is
//     back and settled, and is read again then.
//   - addChildWindow:ordered: orders the child in and resets the parent's
//     level, so children are attached only while shown and the parent's
//     level is restored (Electron's AttachChildren).
//   - A frameless window is a titled window with a full-size content view,
//     a transparent title bar, no title and its standard buttons hidden
//     (Electron's frameless windows): AppKit then gives it the system's
//     shadow, rounded corners and native resizing from its edges, lets it
//     become key and main, and sends presses in the title bar strip to the
//     content view, which never moves the window by itself
//     (mouseDownCanMoveWindow). Its size is the frame's: AppKit's
//     content-rectangle conversions keep a title bar there.

// wsel holds the selectors of window control.
var wsel = struct {
	miniaturize, deminiaturize, isMiniaturized, zoom, isZoomed, toggleFullScreen    darwin.SEL
	styleMask, setStyleMask, setCollectionBehavior, setLevel, level, frame          darwin.SEL
	setFrameDisplay, setFrameTopLeftPoint, setFrameOrigin, convertRectToView        darwin.SEL
	convertRectToScreen, addChildWindow, removeChildWindow, parentWindow            darwin.SEL
	standardWindowButton, setHidden, setEnabled, setTitlebarTransparent             darwin.SEL
	setMaskImage                                                                    darwin.SEL
	setTitleVisibility, setTitlebarSeparator, setOpaque, setBackgroundColor         darwin.SEL
	clearColor, windowBackgroundColor, invalidateShadow, makeKeyWindow, isKeyWindow darwin.SEL
	unhide, activate, respondsTo, setMinSize, setMaxSize, setAutoresizingMask       darwin.SEL
	addSubviewPositioned, addSubview, removeFromSuperview, setMaterial, setBlending darwin.SEL
	setState, performWindowDrag, standardUserDefaults, stringForKey, boolForKey     darwin.SEL
	screens, objectAtIndex, count, mainScreen, orderFront                           darwin.SEL
	visibleFrame                                                                    darwin.SEL
}{
	miniaturize:            darwin.Sel("miniaturize:"),
	deminiaturize:          darwin.Sel("deminiaturize:"),
	isMiniaturized:         darwin.Sel("isMiniaturized"),
	zoom:                   darwin.Sel("zoom:"),
	isZoomed:               darwin.Sel("isZoomed"),
	toggleFullScreen:       darwin.Sel("toggleFullScreen:"),
	styleMask:              darwin.Sel("styleMask"),
	setStyleMask:           darwin.Sel("setStyleMask:"),
	setCollectionBehavior:  darwin.Sel("setCollectionBehavior:"),
	setLevel:               darwin.Sel("setLevel:"),
	level:                  darwin.Sel("level"),
	frame:                  darwin.Sel("frame"),
	setFrameDisplay:        darwin.Sel("setFrame:display:"),
	setFrameTopLeftPoint:   darwin.Sel("setFrameTopLeftPoint:"),
	setFrameOrigin:         darwin.Sel("setFrameOrigin:"),
	convertRectToView:      darwin.Sel("convertRect:toView:"),
	convertRectToScreen:    darwin.Sel("convertRectToScreen:"),
	addChildWindow:         darwin.Sel("addChildWindow:ordered:"),
	removeChildWindow:      darwin.Sel("removeChildWindow:"),
	parentWindow:           darwin.Sel("parentWindow"),
	standardWindowButton:   darwin.Sel("standardWindowButton:"),
	setHidden:              darwin.Sel("setHidden:"),
	setEnabled:             darwin.Sel("setEnabled:"),
	setTitlebarTransparent: darwin.Sel("setTitlebarAppearsTransparent:"),
	setMaskImage:           darwin.Sel("setMaskImage:"),
	setTitleVisibility:     darwin.Sel("setTitleVisibility:"),
	setTitlebarSeparator:   darwin.Sel("setTitlebarSeparatorStyle:"),
	setOpaque:              darwin.Sel("setOpaque:"),
	setBackgroundColor:     darwin.Sel("setBackgroundColor:"),
	clearColor:             darwin.Sel("clearColor"),
	windowBackgroundColor:  darwin.Sel("windowBackgroundColor"),
	invalidateShadow:       darwin.Sel("invalidateShadow"),
	makeKeyWindow:          darwin.Sel("makeKeyWindow"),
	isKeyWindow:            darwin.Sel("isKeyWindow"),
	unhide:                 darwin.Sel("unhide:"),
	activate:               darwin.Sel("activate"),
	respondsTo:             darwin.Sel("respondsToSelector:"),
	setMinSize:             darwin.Sel("setMinSize:"),
	setMaxSize:             darwin.Sel("setMaxSize:"),
	setAutoresizingMask:    darwin.Sel("setAutoresizingMask:"),
	addSubviewPositioned:   darwin.Sel("addSubview:positioned:relativeTo:"),
	addSubview:             darwin.Sel("addSubview:"),
	removeFromSuperview:    darwin.Sel("removeFromSuperview"),
	setMaterial:            darwin.Sel("setMaterial:"),
	setBlending:            darwin.Sel("setBlendingMode:"),
	setState:               darwin.Sel("setState:"),
	performWindowDrag:      darwin.Sel("performWindowDragWithEvent:"),
	standardUserDefaults:   darwin.Sel("standardUserDefaults"),
	stringForKey:           darwin.Sel("stringForKey:"),
	boolForKey:             darwin.Sel("boolForKey:"),
	screens:                darwin.Sel("screens"),
	objectAtIndex:          darwin.Sel("objectAtIndex:"),
	count:                  darwin.Sel("count"),
	mainScreen:             darwin.Sel("mainScreen"),
	orderFront:             darwin.Sel("orderFront:"),
	visibleFrame:           darwin.Sel("visibleFrame"),
}

// fsStartTimeout is how long a toggleFullScreen: may go without its
// windowWill…FullScreen: notification before the request is given up
// (AppKit refused it: a child window, a window on no screen).
const fsStartTimeout = time.Second

// fsSettleTime is how long after the end of a full-screen transition the
// window settles: no toggleFullScreen: is sent before (one sent from the
// Did notification was lost), and the state is read again after.
const fsSettleTime = 300 * time.Millisecond

// zoomTimeout is how long a zoom announced by windowShouldZoom:toFrame:
// may take to reach the other side before it is taken as not happening.
const zoomTimeout = time.Second

// windowCtl is the window control state of a Window.
type windowCtl struct {
	container darwin.ID // the content view: holds the effect view and TelaView (the window owns it)

	zoomed     bool        // zoomed (tracked, not isZoomed), as last known outside full screen
	pos        image.Point // the position, as last read outside full screen
	fullscreen bool        // between windowDidEnterFullScreen: and windowDidExitFullScreen:

	// zooming is set from windowShouldZoom:toFrame: (or our own zoom:)
	// until isZoomed reaches zoomTo, the side the zoom goes to; zoomAt is
	// when it started.
	// inPlace: zoomed by a SetMaximized that found the frame at the
	// standard frame already, so no zoom: was sent.
	zooming, zoomTo, inPlace bool
	zoomAt                   time.Duration

	// fsWant is the full-screen state asked for (by the app, or by the
	// user through AppKit's own controls); fsAsked is when
	// toggleFullScreen: was sent and no transition has started yet (0:
	// none); fsTransition is set between the Will and the Did (or Fail)
	// notifications; fsSettled is when the window is settled after the
	// last transition (fsSettleTime after its end; 0: nothing to wait).
	fsWant, fsTransition bool
	fsAsked, fsSettled   time.Duration
	tempResizable        bool // the Resizable mask was added for full screen
	// fsFrame is the window's frame when it entered full screen, which
	// it gets back when it leaves (fsRestore: not checked yet).
	fsFrame   darwin.CGRect
	fsRestore bool
	// maxWant is the zoom asked for; maxPending, that a SetMaximized
	// waits to be applied at the end of the pass.
	maxWant, maxPending bool
	minimizeOnShow      bool // Minimize before the window was ever shown

	onTop    bool
	blocked  bool // the standard buttons are disabled for a visible modal child
	regions  []platform.HitRegion
	caption  captionInput
	resize   resizeInput  // the app's resize handles (resize_darwin.go)
	cwatch   captionWatch // a caption press handed to the window server
	cwPoll   *time.Timer
	backdrop platform.Backdrop
	effect   darwin.ID    // NSVisualEffectView behind TelaView (owned: +1), or 0
	overlay  overlayState // a TitleBarOverlay window's buttons (overlay_darwin.go)

	// Counters for tests.
	fsEntered, fsExited, drags, doubleClicks int
}

// windowDrag starts the window server's move of w for the press ev (a
// primary press in a caption region). The window server owns the rest of
// the pointer sequence, whose release may never reach the window. Tests
// replace it.
var windowDrag = func(w *Window, ev darwin.ID) {
	w.ctl.drags++
	darwin.Send(w.ns, wsel.performWindowDrag, ev)
}

// --- creation ---

// setupWindowCtl applies the window control options of a new window,
// before its first ConfigureEvent is queued: the frameless style, full
// screen support, the size limits, the place, the level and the backdrop.
func (w *Window) setupWindowCtl() {
	opts := w.opts
	if w.fullSizeContent() {
		w.applyFrameless()
		// A full-size content view is the frame: give the frame the size
		// asked for (initWithContentRect: added a title bar to it).
		w.resizeContent(opts.Size)
	}
	darwin.Send(w.ns, wsel.setCollectionBehavior, uint(darwin.NSWindowCollectionBehaviorFullScreenPrimary))
	w.applyLimits()
	switch {
	case opts.HasPosition:
		// A position from an earlier run may be on a screen that is gone.
		w.moveTo(fitContent(opts.Position, toCocoaRect(darwin.SendRect(w.ns, wsel.frame)), w.contentRect(), screenWorkAreas()))
	case w.parent != nil:
		w.centerOnParent()
	default:
		darwin.Send(w.ns, sel.center)
	}
	w.ctl.onTop = opts.AlwaysOnTop
	w.applyLevel()
	if opts.Transparent {
		w.initTransparent() // transparent_darwin.go
	}
	// A transparent window's backdrop is shaped like its card
	// (maskEffect).
	w.ctl.backdrop = opts.Backdrop
	w.applyBackdrop()
	w.ctl.fsWant = opts.Fullscreen
	w.ctl.maxWant, w.ctl.maxPending = opts.Maximized, opts.Maximized
}

// fullSizeContent reports whether the content view is the whole frame: a
// Frameless or TitleBarOverlay window.
func (w *Window) fullSizeContent() bool { return w.opts.Frameless || w.opts.TitleBarOverlay }

// applyFrameless gives a titled window the look of a frameless one: no
// title bar drawn, no title, no separator line, and no standard buttons
// unless it is a title bar overlay (overlay_darwin.go places them).
func (w *Window) applyFrameless() {
	darwin.Send(w.ns, wsel.setTitlebarTransparent, true)
	darwin.Send(w.ns, wsel.setTitleVisibility, uintptr(darwin.NSWindowTitleHidden))
	if darwin.SendBool(w.ns, wsel.respondsTo, wsel.setTitlebarSeparator) {
		darwin.Send(w.ns, wsel.setTitlebarSeparator, uintptr(darwin.NSTitlebarSeparatorStyleNone))
	}
	if w.opts.TitleBarOverlay {
		return
	}
	for _, b := range [...]uintptr{darwin.NSWindowCloseButton, darwin.NSWindowMiniaturizeButton, darwin.NSWindowZoomButton} {
		if btn := darwin.Send(w.ns, wsel.standardWindowButton, b); btn != 0 {
			darwin.Send(btn, wsel.setHidden, true)
		}
	}
}

// centerOnParent places a child window without a position over the
// center of its parent (AppKit only centers on the screen).
func (w *Window) centerOnParent() {
	pf := darwin.SendRect(w.parent.ns, wsel.frame)
	f := darwin.SendRect(w.ns, wsel.frame)
	darwin.Send(w.ns, wsel.setFrameOrigin, darwin.CGPoint{
		X: pf.Origin.X + (pf.Size.Width-f.Size.Width)/2,
		Y: pf.Origin.Y + (pf.Size.Height-f.Size.Height)/2,
	})
}

// --- state ---

// syncState reads the window's state from AppKit and queues it
// (coalesced) when it changed: minimized, zoomed and the position (frozen
// while in or entering full screen, so the state is what the window
// returns to), full screen from the delegate's notifications, and the
// client-side decorations of a frameless window. Callers that also queue
// a ConfigureEvent call it first: the state goes before the size it comes
// with.
func (w *Window) syncState() {
	w.updateGeometry() // the accessibility bridge's view rectangle
	if w.Closed() {
		return
	}
	c := &w.ctl
	if !w.fsBusy() && !c.fullscreen {
		w.minimized = darwin.SendBool(w.ns, wsel.isMiniaturized)
		if !w.minimized {
			w.trackZoom()
			if !c.maxPending {
				c.maxWant = c.zoomed
			}
			c.pos = w.position()
		}
	}
	w.b.loop.UpdateState(w, platform.WindowState{
		Maximized:             c.zoomed,
		Fullscreen:            c.fullscreen,
		Minimized:             w.minimized,
		ClientSideDecorations: w.fullSizeContent(),
		TitleBarControls:      w.titleBarControls(),
		Transparent:           w.opts.Transparent, // the window server composites every window
		HasPosition:           true,
		Position:              c.pos,
	})
	w.updateHidden()
}

// fsBusy reports whether a full-screen toggle was sent and has not
// started, a transition runs, or the window is settling after one.
func (w *Window) fsBusy() bool {
	c := &w.ctl
	return c.fsAsked != 0 || c.fsTransition || (c.fsSettled != 0 && w.b.Now() < c.fsSettled)
}

// trackZoom updates the zoom from AppKit's isZoomed (see the top of the
// file): a zoom in progress ends when isZoomed reaches its side, or after
// zoomTimeout on the side it went to if isZoomed allows it (an unzoom of a
// window whose user frame is its standard frame, at its maximum size, does
// not move but ends unzoomed); otherwise the window stays zoomed only
// while isZoomed holds (a user resize of a zoomed window ends it), and
// never becomes zoomed by itself.
func (w *Window) trackZoom() {
	c := &w.ctl
	if w.opts.FixedSize {
		c.zoomed, c.zooming = false, false
		return
	}
	zoomed := darwin.SendBool(w.ns, wsel.isZoomed)
	switch {
	case c.zooming && zoomed == c.zoomTo:
		c.zooming = false
		c.zoomed = zoomed
	case c.zooming && w.b.Now()-c.zoomAt <= zoomTimeout:
		// Still on its way (an animated zoom resizes several times).
	case c.zooming:
		c.zooming = false
		c.zoomed = c.zoomTo && zoomed
	default:
		c.zooming = false
		c.zoomed = c.zoomed && zoomed
	}
	c.inPlace = c.inPlace && c.zoomed && !c.zooming
}

// zoomStarting records a zoom that AppKit is about to perform (from
// windowShouldZoom:toFrame:, or right before our own zoom:): it goes to
// the standard frame unless AppKit's isZoomed holds, then to the user
// frame.
func (w *Window) zoomStarting() {
	c := &w.ctl
	if c.zooming && w.b.Now()-c.zoomAt <= zoomTimeout {
		return // announced already (our zoom: asks the delegate too)
	}
	c.zooming = true
	c.zoomTo = !darwin.SendBool(w.ns, wsel.isZoomed)
	c.zoomAt = max(w.b.Now(), 1)
	time.AfterFunc(zoomTimeout+50*time.Millisecond, w.b.notify) // a zoom that never moved ends then
}

// zoom sends zoom: (a zoom of the app's, or the caption's double click).
func (w *Window) zoom() {
	w.zoomStarting()
	darwin.Send(w.ns, wsel.zoom, darwin.ID(0))
}

// applyControl runs at the end of every delivery pass: it applies the
// full-screen and zoom requests of the pass (so that a request undone in
// the same pass costs nothing) and those that waited for a transition to
// end and settle, for the window to show or to come back from the Dock.
func (w *Window) applyControl() {
	if w.Closed() {
		return
	}
	c := &w.ctl
	now := w.b.Now()
	if c.fsAsked != 0 && now-c.fsAsked > fsStartTimeout {
		// AppKit refused the toggle: no transition started.
		c.fsAsked = 0
		c.fsWant = c.fullscreen
		w.restoreResizable()
	}
	if c.fsAsked != 0 || c.fsTransition {
		return // the end of the transition runs a pass
	}
	if c.fsSettled != 0 {
		if now < c.fsSettled {
			return // a timer runs a pass when the window has settled
		}
		// Settled: read the state that was frozen meanwhile.
		c.fsSettled = 0
		w.restoreFrame()
		w.syncState()
	}
	if w.placeButtons() {
		w.syncState()
	}
	if c.zooming && now-c.zoomAt > zoomTimeout {
		w.syncState() // a zoom that never moved
	}
	if c.fsWant != c.fullscreen {
		if w.shown && !w.minimized {
			w.toggleFullscreen()
		}
		return
	}
	if c.maxPending && !c.fullscreen && !w.minimized {
		c.maxPending = false
		if !w.opts.FixedSize && c.maxWant != c.zoomed {
			switch {
			case c.maxWant && darwin.SendBool(w.ns, wsel.isZoomed):
				// AppKit's frame is there already (a window at its
				// maximum size is at its standard frame), and zoom:
				// would go the other way: only the state changes, and
				// so does the unzoom that follows.
				c.zoomed, c.inPlace = true, true
			case !c.maxWant && c.inPlace:
				c.zoomed, c.inPlace = false, false
			default:
				w.zoom()
			}
		}
		w.syncState()
	}
}

// toggleFullscreen sends toggleFullScreen:, making a non-resizable window
// resizable first (AppKit refuses full screen otherwise).
func (w *Window) toggleFullscreen() {
	c := &w.ctl
	if !c.fullscreen && darwin.SendUint(w.ns, wsel.styleMask)&darwin.NSWindowStyleMaskResizable == 0 {
		w.setStyleMask(darwin.SendUint(w.ns, wsel.styleMask) | darwin.NSWindowStyleMaskResizable)
		c.tempResizable = true
	}
	c.fsAsked = max(w.b.Now(), 1)
	darwin.Send(w.ns, wsel.toggleFullScreen, darwin.ID(0))
}

// restoreResizable takes the Resizable mask back from a FixedSize window
// that left full screen.
func (w *Window) restoreResizable() {
	if w.ctl.tempResizable {
		w.ctl.tempResizable = false
		w.setStyleMask(darwin.SendUint(w.ns, wsel.styleMask) &^ darwin.NSWindowStyleMaskResizable)
	}
}

// setStyleMask changes the style mask; AppKit may rebuild the title bar,
// so a frameless window gets its look again.
func (w *Window) setStyleMask(mask uint64) {
	darwin.Send(w.ns, wsel.setStyleMask, uint(mask))
	if w.fullSizeContent() {
		w.applyFrameless()
	}
}

// fullscreenWill handles windowWillEnterFullScreen: (entering) and
// windowWillExitFullScreen:. A transition the backend did not ask for is
// the user's (the green button, the Window menu, Control-Command-F): it
// becomes the wanted state.
func (w *Window) fullscreenWill(entering bool) {
	c := &w.ctl
	if entering {
		w.restoreButtons() // the full-screen title bar is AppKit's
		if !c.fullscreen {
			c.fsFrame = darwin.SendRect(w.ns, wsel.frame)
		}
	}
	if c.fsAsked == 0 && !c.fsTransition {
		c.fsWant = entering
	}
	c.fsAsked = 0
	c.fsTransition = true
}

// restoreFrame gives a window that left full screen the frame it entered
// it with, when AppKit restored another. On macOS 26 a full-size content
// window (Frameless, TitleBarOverlay) resized with setFrame:display: came
// back at its size and place from creation, not the frame it had when it
// entered full screen: AppKit seems to keep the frame it restores from the
// content rectangle it last set itself. Titled windows come back right,
// and this then does nothing.
func (w *Window) restoreFrame() {
	c := &w.ctl
	if !c.fsRestore || c.fullscreen {
		return
	}
	c.fsRestore = false
	f := darwin.SendRect(w.ns, wsel.frame)
	near := func(a, b float64) bool { return math.Abs(a-b) < 0.5 }
	if near(f.Origin.X, c.fsFrame.Origin.X) && near(f.Origin.Y, c.fsFrame.Origin.Y) &&
		near(f.Size.Width, c.fsFrame.Size.Width) && near(f.Size.Height, c.fsFrame.Size.Height) {
		return
	}
	darwin.Send(w.ns, wsel.setFrameDisplay, c.fsFrame, true)
}

// fullscreenDid handles the end of a transition: fullscreen is the state
// the window is in now; failed, that the transition did not happen (the
// request is given up).
func (w *Window) fullscreenDid(fullscreen, failed bool) {
	c := &w.ctl
	c.fsTransition, c.fsAsked = false, 0
	c.fullscreen = fullscreen
	switch {
	case failed:
		c.fsWant = fullscreen
	case fullscreen:
		c.fsEntered++
	default:
		c.fsExited++
		c.fsRestore = c.fsFrame.Size.Width > 0
	}
	if !fullscreen {
		w.restoreResizable()
	}
	// The full-screen flag is reported now; the rest of the state, and a
	// request that waited, after the window settled.
	c.fsSettled = max(w.b.Now()+fsSettleTime, 1)
	time.AfterFunc(fsSettleTime+20*time.Millisecond, w.b.notify)
	w.syncState()
}

// --- positions ---

// primaryTop returns the y of the primary screen's top edge in AppKit's
// screen coordinates (its height: its origin is 0,0).
func primaryTop() float64 {
	screens := darwin.SendClass(app.screen, wsel.screens)
	if screens == 0 || darwin.SendUint(screens, wsel.count) == 0 {
		return 0
	}
	f := darwin.SendRect(darwin.Send(screens, wsel.objectAtIndex, uint(0)), wsel.frame)
	return f.Origin.Y + f.Size.Height
}

// screenWorkAreas returns the visibleFrame of every screen (without the
// menu bar and the Dock) in desktop coordinates, the primary first
// (NSScreen.screens lists it first: the screen with the menu bar).
func screenWorkAreas() []image.Rectangle {
	screens := darwin.SendClass(app.screen, wsel.screens)
	if screens == 0 {
		return nil
	}
	n := darwin.SendUint(screens, wsel.count)
	top := primaryTop()
	works := make([]image.Rectangle, 0, n)
	for i := uint64(0); i < n; i++ {
		vf := darwin.SendRect(darwin.Send(screens, wsel.objectAtIndex, uint(i)), wsel.visibleFrame)
		works = append(works, desktopRect(toCocoaRect(vf), top))
	}
	return works
}

func toCocoaRect(r darwin.CGRect) cocoaRect {
	return cocoaRect{X: r.Origin.X, Y: r.Origin.Y, W: r.Size.Width, H: r.Size.Height}
}

// contentRect returns the content area (TelaView) in screen coordinates.
func (w *Window) contentRect() cocoaRect {
	inWindow := darwin.SendRect(w.view, wsel.convertRectToView, darwin.SendRect(w.view, sel.bounds), darwin.ID(0))
	return toCocoaRect(darwin.SendRect(w.ns, wsel.convertRectToScreen, inWindow))
}

// position returns the content area's top-left (windowctl.go: points, y
// down from the primary screen's top-left).
func (w *Window) position() image.Point { return desktopPosition(w.contentRect(), primaryTop()) }

// moveTo moves the window so that the content area's top-left is at pos.
func (w *Window) moveTo(pos image.Point) {
	x, y := frameTopLeft(pos, toCocoaRect(darwin.SendRect(w.ns, wsel.frame)), w.contentRect(), primaryTop())
	darwin.Send(w.ns, wsel.setFrameTopLeftPoint, darwin.CGPoint{X: x, Y: y})
}

// --- sizes ---

// resizeContent asks for a content size: setContentSize: for a titled
// window, the frame itself for a frameless one (its content view is the
// frame), keeping the top-left corner where it is.
func (w *Window) resizeContent(size platform.Size) {
	if !w.fullSizeContent() {
		darwin.Send(w.ns, sel.setContentSize, cgSize(size))
		return
	}
	f := darwin.SendRect(w.ns, wsel.frame)
	top := f.Origin.Y + f.Size.Height
	darwin.Send(w.ns, wsel.setFrameDisplay, darwin.CGRect{
		Origin: darwin.CGPoint{X: f.Origin.X, Y: top - float64(size.H)},
		Size:   cgSize(size),
	}, true)
}

// applyLimits gives AppKit the size limits (windowctl.go's sizeLimits):
// content limits for a titled window, frame limits for a frameless one
// (AppKit prefers content limits when both are set, and its content
// rectangle of a frameless window has a title bar).
func (w *Window) applyLimits() {
	lo, hi := sizeLimits(w.opts.MinSize, w.opts.MaxSize)
	minSel, maxSel := sel.setContentMinSize, sel.setContentMaxSize
	if w.fullSizeContent() {
		minSel, maxSel = wsel.setMinSize, wsel.setMaxSize
	}
	darwin.Send(w.ns, minSel, darwin.CGSize{Width: lo[0], Height: lo[1]})
	darwin.Send(w.ns, maxSel, darwin.CGSize{Width: hi[0], Height: hi[1]})
}

// --- requests ---

// Minimize miniaturizes the window into the Dock (miniaturize:); a window
// never shown is miniaturized when it is. The state arrives from
// windowDidMiniaturize:. A full-screen window does not minimize (AppKit
// ignores it).
func (w *Window) Minimize() {
	if w.Closed() {
		return
	}
	if !w.shown {
		w.ctl.minimizeOnShow = true
		return
	}
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	darwin.Send(w.ns, wsel.miniaturize, darwin.ID(0))
}

// SetMaximized zooms the window (zoom:) or zooms it back, at the end of
// the pass, when isZoomed differs; a full-screen window zooms when it
// leaves full screen, a minimized one when it comes back. FixedSize
// windows do not zoom.
func (w *Window) SetMaximized(maximized bool) {
	if w.Closed() || (!w.ctl.maxPending && maximized == w.ctl.maxWant) {
		return
	}
	w.ctl.maxWant, w.ctl.maxPending = maximized, true
	w.b.notify()
}

// SetFullscreen enters or leaves native full screen (toggleFullScreen:,
// its own space) at the end of the pass; a request made during a
// transition waits for its end, and one for a window not shown yet (or
// minimized) waits for it to show.
func (w *Window) SetFullscreen(fullscreen bool) {
	if w.Closed() || fullscreen == w.ctl.fsWant {
		return
	}
	w.ctl.fsWant = fullscreen
	w.b.notify()
}

// SetAlwaysOnTop puts the window at the floating level (palettes) or back
// at the normal one.
func (w *Window) SetAlwaysOnTop(onTop bool) {
	if w.Closed() || onTop == w.ctl.onTop {
		return
	}
	w.ctl.onTop = onTop
	w.applyLevel()
}

func (w *Window) applyLevel() {
	level := darwin.NSNormalWindowLevel
	if w.ctl.onTop {
		level = darwin.NSFloatingWindowLevel
	}
	darwin.Send(w.ns, wsel.setLevel, level)
}

// SetPosition moves the window so that the content area's top-left is at
// pos: points, y down from the top-left of the primary screen (see
// windowctl.go). A full-screen window does not move.
func (w *Window) SetPosition(pos image.Point) {
	if w.Closed() || w.ctl.fullscreen || w.ctl.fsTransition {
		return
	}
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	w.moveTo(pos)
	w.syncState()
}

// Activate brings the window back from the Dock, unhides the app,
// activates it ([NSApp activate] on macOS 14 and later, which the system
// may decline; activateIgnoringOtherApps: before) and makes the window key
// and front (or its modal child, which has the input). A hidden window
// stays hidden; one not shown yet becomes key when it shows.
func (w *Window) Activate() {
	if w.Closed() || !w.visible {
		return
	}
	w.ctl.minimizeOnShow = false
	if !w.shown {
		return
	}
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	if darwin.SendBool(w.ns, wsel.isMiniaturized) {
		darwin.Send(w.ns, wsel.deminiaturize, darwin.ID(0))
	}
	darwin.Send(app.ns, wsel.unhide, darwin.ID(0))
	activateApp()
	if !w.raiseModal() {
		darwin.Send(w.ns, sel.makeKeyAndOrderFront, darwin.ID(0))
	}
}

// ActivateWithToken is Activate: macOS has no activation tokens.
func (w *Window) ActivateWithToken(string) { w.Activate() }

func activateApp() {
	if darwin.SendBool(app.ns, wsel.respondsTo, wsel.activate) {
		darwin.Send(app.ns, wsel.activate)
	} else {
		darwin.Send(app.ns, sel.activate, true)
	}
}

// SetIcon validates the images and does nothing: macOS windows have no
// icon (the app's icon is in the Dock, from its bundle).
func (w *Window) SetIcon(images []image.Image) { platform.CheckIcon("SetIcon", images) }

// SetTitleBarColors does nothing: macOS title bars follow the window's
// appearance and backdrop, with no colors of the app's (a frameless
// window draws its own title bar).
func (w *Window) SetTitleBarColors(platform.TitleBarColors) {}

// SetHitRegions keeps the regions of the app's title bar (see "Custom
// title bars" in package platform): presses in caption regions are the
// window server's (input_darwin.go and captionInput).
func (w *Window) SetHitRegions(regions []platform.HitRegion) {
	platform.CheckHitRegions(regions)
	if w.Closed() || platform.EqualHitRegions(regions, w.ctl.regions) {
		return
	}
	w.ctl.regions = append(w.ctl.regions[:0], regions...)
	w.syncHandles() // a transparent window's resize handles (transparent_darwin.go)
	if w.opts.TitleBarOverlay {
		w.b.notify() // the traffic lights follow the caption's height (applyControl)
	}
}

// RestoreToken returns "": macOS restores windows by position.
func (w *Window) RestoreToken() string { return "" }

// --- custom title bars ---

// captionPress decides a press in the window (input_darwin.go): it
// reports whether the press is the window's (a caption region, which it
// handles) and must not be delivered.
func (w *Window) captionPress(ev darwin.ID, btn platform.PointerButton, pos platform.Point) bool {
	if len(w.ctl.regions) == 0 {
		return false
	}
	action := w.ctl.caption.press(btn, pos, eventMods(ev), w.b.eventTime(ev), w.ctl.regions, w.ptr.held, w.b.Settings())
	if action == captionContent {
		return false
	}
	if w.ctl.blocked {
		return true // a modal child has the input: raiseModal brought it forward
	}
	switch action {
	case captionDrag:
		windowDrag(w, ev)
		w.watchCaption() // a click that moved nothing (resize_darwin.go)
	case captionDoubleClick:
		w.ctl.doubleClicks++
		w.titleBarDoubleClick()
	}
	return true
}

// titleBarDoubleClick performs the user's title bar double-click action.
func (w *Window) titleBarDoubleClick() {
	switch doubleClickAction(userDoubleClickSetting()) {
	case titleBarZoom:
		if !w.opts.FixedSize && !w.ctl.fullscreen {
			w.zoom()
		}
	case titleBarMinimize:
		darwin.Send(w.ns, wsel.miniaturize, darwin.ID(0))
	}
}

// userDoubleClickSetting reads the global defaults AppleActionOnDoubleClick
// (hasAction: it is set) and AppleMiniaturizeOnDoubleClick
// (windowctl.go's doubleClickAction). Call it inside an autorelease pool.
func userDoubleClickSetting() (action string, hasAction, miniaturize bool) {
	defaults := darwin.SendClass(app.userDefaults, wsel.standardUserDefaults)
	if defaults == 0 {
		return "", false, false
	}
	key := darwin.NSString("AppleActionOnDoubleClick")
	s := darwin.Send(defaults, wsel.stringForKey, key)
	darwin.Release(key)
	key = darwin.NSString("AppleMiniaturizeOnDoubleClick")
	miniaturize = darwin.SendBool(defaults, wsel.boolForKey, key)
	darwin.Release(key)
	return darwin.GoString(s), s != 0, miniaturize
}

// --- parent and modal windows ---

// attach makes a shown window the child window of its shown parent
// (addChildWindow:ordered:, NSWindowAbove: AppKit keeps it above the
// parent, moves it with the parent and hides it with the parent's
// minimization), and attaches its own shown children. AppKit resets the
// parent's level when it gains a child, so the levels are set again.
func (w *Window) attach() {
	if p := w.parent; p != nil && !p.Closed() && p.shown && darwin.Send(w.ns, wsel.parentWindow) != p.ns {
		darwin.Send(p.ns, wsel.addChildWindow, w.ns, darwin.NSWindowAbove)
		p.applyLevel()
		w.applyLevel()
	}
	for _, c := range w.b.loop.Windows() {
		if c.parent == w && c.shown && !c.Closed() {
			c.attach()
		}
	}
}

// detach removes the window from its parent's child windows (before it
// is hidden or closed).
func (w *Window) detach() {
	if p := darwin.Send(w.ns, wsel.parentWindow); p != 0 {
		darwin.Send(p, wsel.removeChildWindow, w.ns)
	}
}

// modalChild returns the shown modal child of the window, or nil.
func (w *Window) modalChild() *Window {
	for _, c := range w.b.loop.Windows() {
		if c.parent == w && c.opts.Modal && c.shown && !c.Closed() {
			return c
		}
	}
	return nil
}

// raiseModal brings the innermost shown modal child of the window
// forward and makes it key, and reports whether there is one: the window
// itself has no input meanwhile.
func (w *Window) raiseModal() bool {
	c := w.modalChild()
	if c == nil {
		return false
	}
	for next := c.modalChild(); next != nil; next = c.modalChild() {
		c = next
	}
	darwin.Send(c.ns, sel.makeKeyAndOrderFront, darwin.ID(0))
	return true
}

// updateModal disables the standard buttons of a window while a modal
// child is shown (like EnableWindow(parent, FALSE) on Win32, the parent
// cannot be closed, minimized or zoomed from them) and enables them again
// after.
func (w *Window) updateModal() {
	if w == nil || w.Closed() {
		return
	}
	blocked := w.modalChild() != nil
	if blocked == w.ctl.blocked {
		return
	}
	w.ctl.blocked = blocked
	for _, b := range [...]uintptr{darwin.NSWindowCloseButton, darwin.NSWindowMiniaturizeButton, darwin.NSWindowZoomButton} {
		if btn := darwin.Send(w.ns, wsel.standardWindowButton, b); btn != 0 {
			darwin.Send(btn, wsel.setEnabled, !blocked)
		}
	}
}

// --- backdrop ---

var _ platform.AlphaPresenter = (*Window)(nil)

// PresentsAlpha reports whether the window shows the alpha of its frames:
// it has a vibrancy backdrop (platform.AlphaPresenter).
func (w *Window) PresentsAlpha() bool { return w.ctl.effect != 0 }

// SetBackdrop sets the material behind the window (platform.Backdrop's
// mapping onto NSVisualEffectView materials). BackdropDefault and
// BackdropNone make the window opaque again.
func (w *Window) SetBackdrop(backdrop platform.Backdrop) {
	platform.CheckBackdrop("SetBackdrop backdrop", backdrop)
	if w.Closed() || backdrop == w.ctl.backdrop {
		return
	}
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	w.ctl.backdrop = backdrop
	w.applyBackdrop()
}

// applyBackdrop puts an NSVisualEffectView (behind-window blending, a
// material that follows the window's active state as native windows'
// do) behind TelaView in the content view, and makes the window
// non-opaque with a clear background so that the frame's transparent
// pixels show it; without a material it removes the view and makes the
// window opaque again.
func (w *Window) applyBackdrop() {
	c := &w.ctl
	material, glass := backdropMaterial(c.backdrop)
	switch {
	case glass && c.effect == 0:
		if app.effectView == 0 {
			return
		}
		e := darwin.Send(darwin.SendClass(app.effectView, sel.alloc), sel.initWithFrame, darwin.SendRect(c.container, sel.bounds))
		if e == 0 {
			return
		}
		darwin.Send(e, wsel.setAutoresizingMask, uint(darwin.NSViewWidthSizable|darwin.NSViewHeightSizable))
		darwin.Send(e, wsel.setBlending, darwin.NSVisualEffectBlendingModeBehindWindow)
		darwin.Send(e, wsel.setState, darwin.NSVisualEffectStateFollowsWindowActiveState)
		darwin.Send(e, wsel.setMaterial, material)
		darwin.Send(c.container, wsel.addSubviewPositioned, e, darwin.NSWindowBelow, w.view)
		c.effect = e
		w.setOpaque(false)
		w.maskEffect() // a transparent window: the material behind its card only
	case glass:
		darwin.Send(c.effect, wsel.setMaterial, material)
	case c.effect != 0:
		darwin.Send(c.effect, wsel.removeFromSuperview)
		darwin.Release(c.effect)
		c.effect = 0
		if !w.opts.Transparent {
			w.setOpaque(true)
		}
	}
}

// maskEffect gives the NSVisualEffectView of a transparent window the
// shape of its input mask (ADR-257): a window-sized NSImage, opaque where
// the presented frame takes input (the app's card, at least
// platform.InputAlpha opaque) and clear elsewhere, as its maskImage, so the
// material shows behind the card and not over the desktop around it nor
// under the shadows. It runs when the backdrop is set and whenever a
// Present changes the mask. A layer mask would not do: AppKit documents
// maskImage as the way to shape a visual effect view's material.
func (w *Window) maskEffect() {
	if !w.opts.Transparent || w.ctl.effect == 0 {
		return
	}
	pix := maskImage(&w.trans.mask)
	var ns darwin.ID
	if pix != nil {
		cg, err := w.b.newImage(pix)
		if err != nil {
			return
		}
		b := darwin.SendRect(w.view, sel.bounds)
		ns = darwin.Send(darwin.SendClass(darwin.GetClass("NSImage"), sel.alloc), dndSel.initWithCGImageSize, cg, b.Size)
		darwin.CGImageRelease(cg) // the image keeps it
	}
	darwin.Send(w.ctl.effect, wsel.setMaskImage, ns)
	if ns != 0 {
		darwin.Release(ns) // the view keeps it
	}
}

// setOpaque makes the window opaque with the standard background, or
// transparent with a clear one.
func (w *Window) setOpaque(opaque bool) {
	darwin.Send(w.ns, wsel.setOpaque, opaque)
	color := wsel.clearColor
	if opaque {
		color = wsel.windowBackgroundColor
	}
	darwin.Send(w.ns, wsel.setBackgroundColor, darwin.SendClass(app.color, color))
	darwin.Send(w.ns, wsel.invalidateShadow)
}

// --- integration requests (later tasks) ---

// SetDropEffect and StartDrag are in dnd_darwin.go, SetMenu and PopupMenu
// in menu_darwin.go.
