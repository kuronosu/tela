package core

import (
	"math"
	"time"

	"github.com/kuronosu/tela/internal/fmath"
	"github.com/kuronosu/tela/internal/gesture"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/layout"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/style"
)

// Scrolling (ARCHITECTURE sections 9 and 14) follows CSSOM View and the
// browsers: scroll containers, their offsets, user scrolling (wheel,
// touchpad, keyboard, touch, scrollbars), programmatic scrolling and the
// scroll events.
//
// # Scroll containers
//
// A box whose computed overflow is hidden, scroll or auto in an axis is a
// scroll container (CSS: visible next to a scrolling axis computes to
// auto, clip to hidden). Only auto and scroll axes scroll by the user;
// hidden ones scroll programmatically (ScrollTo, ScrollIntoView, focus);
// clip never scrolls. The largest offsets are the layout's scrollable
// overflow (the content's extent with the end padding, CSS's scrollable
// overflow area) minus the client size (the padding box without the
// scrollbar gutters). Offsets live on the box, so they survive rebuilds
// and relayouts; every layout clamps them again (a shrinking content or
// box pulls them back), and a box that leaves the box tree (display:
// none) loses them, as in browsers.
//
// The paint phase moves the content of a scroll container by its offset,
// snapped to device pixels, under its overflow clip; the absolute
// geometry of every box (paint.go's place) is where it is painted, so
// hit-testing, LocalX/LocalY and the hit bounds agree with the pixels.
// A descendant moves with the scroll containers on its containing-block
// chain: an absolutely positioned box whose containing block is outside
// the container does not scroll (CSS). Until layers arrive (T7b) a scroll
// repaints the whole tree; a scroll container is the natural repaint
// boundary there (see paintInner).
//
// # The viewport
//
// The root box, which fills the window, is the CSS viewport: it is a
// scroll container on both axes, with overflow: auto unless RootClass sets
// an overflow (viewportOverflow), so a page taller or wider than the
// window scrolls like a browser page, with the app's scrollbars, and is
// the last container of every scroll chain, ScrollIntoView and focus
// reveal. Content that fits looks as before (overlay bars stay hidden;
// classic ones reserve their gutter only while the page overflows). The
// root keeps the window's size, so the percentage heights of its children
// (h-full) and h-screen/min-h-screen resolve against the window height,
// as with html, body { height: 100% } in a browser; the root's content
// may be taller. Fixed boxes do not move with it; absolute boxes whose
// containing block is the root scroll with the page (CSS).
//
// # Wheel and touchpad
//
// A platform ScrollEvent becomes an OnWheel event at the element under
// the pointer, which bubbles. Unless a handler prevents it, its default
// action scrolls: wheel notches move wheelNotch pixels each (Chromium's
// distance on Linux; Shift turns a vertical notch into a horizontal one,
// the Linux and Windows convention), touchpad distances move by the
// distance itself. The scroll goes to the nearest user-scrollable
// container on the target's containing-block chain that can still move in
// the event's direction (scroll chaining); a container with
// overscroll-contain or overscroll-none in that axis ends the chain. Like
// Chromium, the chosen container is latched for the gesture: the
// following events of a touchpad gesture (until its end) or of a burst of
// wheel notches (less than latchTimeout apart, without pointer movement)
// scroll it even at its edge, without chaining. Ctrl+wheel does not
// scroll (browsers zoom).
//
// # Animation
//
// User scrolling animates unless an instant setting applies: wheel
// notches, keys and scrollbar track clicks run a userScrollDuration
// animation on Chromium's ease-in-out curve, and a new notch retargets it
// from the current position and velocity (Chromium's
// EaseInOutWithInitialVelocity). Touchpad and touch scrolling follow the
// fingers directly; when the fingers lift (a ScrollPhaseEnd from the
// platform, or a touch release) the content keeps moving with GTK 4's
// kinetic deceleration (GtkKineticScrolling: v(t) = v0 e^(-flingFriction
// t)), from the velocity of the last velocityWindow of the gesture.
// Backends without gesture ends (X11) never fling. Programmatic scrolls
// jump, unless the container has scroll-smooth (CSS scroll-behavior).
//
// Instant settings: App.InstantScroll (Config.InstantScroll) makes user
// scrolling jump, except in containers with scroll-smooth;
// scroll-instant makes it jump in one container; motion-reduce (the
// system's reduced-motion setting) makes everything jump. Nothing that
// jumps flings.
//
// Animations run on the frame clock (Tree.Animate, called by the Host
// with each FrameEvent's time): frames are requested while one is active
// and stop when they end, so an idle window uses no CPU.
//
// # Keyboard
//
// keydown default actions (keyboard.go): the arrows scroll lineStep
// pixels (Chromium's step), Page Up and Page Down the client size minus
// an overlap (Blink's 87.5% of it), Home and End to the start and the
// end, Space and Shift+Space a page down and up when the target is not a
// Button. They scroll the focused scroll container, or the nearest one
// that contains the focus and can move that way (chaining as for the
// wheel), or with nothing focused the outermost scroll container with the
// largest area.
//
// # Focus and programmatic scrolling
//
// A keyboard or programmatic focus scrolls the focused element into view
// in every scroll container on its containing-block chain (HTML's
// focusing steps; not for pointer focus, whose target is under the
// pointer). ElementRef.ScrollTo, ScrollBy and ScrollIntoView move a
// container (its own offsets, or every container around the element with
// CSS's "nearest" alignment); they jump unless the container has
// scroll-smooth.
//
// # Events and values
//
// OnScroll fires on a scroll container once per frame after its offset
// changed (user or programmatic), before the frame's build (the DOM's
// scroll steps of the rendering update); it does not bubble. UseScroll
// returns Values that follow the offsets of the element a ref is
// attached to: they are written on the UI thread when the offset changes
// (after the frame for changes a frame makes, such as clamping), so a
// paint closure that reads them repaints only (ADR-053).
//
// After a frame in which scrolled content moved, the pointer's target is
// hit-tested again (Chromium's fake mouse move): hover states, enter and
// leave events and the cursor follow what is now under a still pointer.
//
// # Scrollbars
//
// Tela draws the scrollbars (scrollbar.go): overlay bars by default,
// classic ones with Config.ClassicScrollbars or scrollbar-classic.

// Scrolling distances and timings.
const (
	// wheelNotch is the distance of one wheel notch in logical pixels:
	// Chromium's on Linux, where a notch (a wheel delta of 120) scrolls
	// 53 px (Windows scrolls 3 lines of 33 px, 100 px; Firefox 3 lines of
	// the font's height).
	wheelNotch = 53
	// lineStep is the arrow keys' step (Blink's kPixelsPerLineStep).
	lineStep = 40
	// pageFraction is the part of the client size a page step moves:
	// Blink's kMinFractionToStepWhenPaging, so 12.5% of the old page
	// stays visible.
	pageFraction = 0.875
	// userScrollDuration is the duration of a wheel, key or track
	// animation (Chromium's wheel animation lasts 6 to 12 frames).
	userScrollDuration = 150 * time.Millisecond
	// programmatic smooth scrolls last minSmoothDuration plus
	// smoothPerSqrtPx per square root of the distance, up to
	// maxSmoothDuration (Chromium's delta-based duration).
	minSmoothDuration = 150 * time.Millisecond
	maxSmoothDuration = 500 * time.Millisecond
	smoothPerSqrtPx   = 15 * time.Millisecond
	// latchTimeout ends a wheel transaction: a notch later than this
	// after the previous one picks a container again (Chromium latches
	// wheel scrolling to a container for a burst of notches; Firefox's
	// mousewheel.transaction.timeout is 1.5 s).
	latchTimeout = 500 * time.Millisecond
	// flingFriction is the kinetic deceleration (GTK 4's
	// DECELERATION_FRICTION, in 1/s): v(t) = v0 e^(-friction t).
	flingFriction = 4
	// flingMinVelocity is the release velocity (px/s) below which no
	// fling starts; flingStopVelocity ends a fling (the content then
	// moves less than half a pixel more).
	flingMinVelocity  = 50
	flingStopVelocity = 2
	// velocityWindow is the part of a touchpad gesture its fling
	// velocity is measured over (GTK 4's SCROLL_CAPTURE_THRESHOLD_MS).
	velocityWindow = 150 * time.Millisecond
)

// scroller is the scroll state of a scroll container box.
type scroller struct {
	box *Box
	// pos is the scroll offset in logical pixels, in [0, max].
	pos gfx.Point
	// max are the largest offsets and client the client size, from the
	// last layout.
	max    gfx.Point
	client gfx.Size
	// userX and userY report the user-scrollable axes (overflow auto or
	// scroll).
	userX, userY bool
	// gutterV and gutterH report classic gutters reserved for an
	// overflow: auto axis because its content overflows (a vertical
	// bar's gutter takes width, a horizontal one's height).
	gutterV, gutterH bool
	// anim and fling animate each axis (0: x, 1: y).
	anim  [2]axisAnim
	fling [2]axisFling
	// soff is the offset of the last paint, snapped to device pixels.
	soff gfx.Point
	// eventPending: OnScroll fires at the next frame. syncPending: the
	// UseScroll values follow after the frame.
	eventPending, syncPending bool

	// Scrollbars (scrollbar.go).
	bars barState
	geo  [2]barGeom
}

// clamp returns p within [0, max].
func (s *scroller) clamp(p gfx.Point) gfx.Point {
	return gfx.Point{X: min(max(p.X, 0), s.max.X), Y: min(max(p.Y, 0), s.max.Y)}
}

// target returns where s is going: the targets of its animations, or its
// offset.
func (s *scroller) target() gfx.Point {
	p := s.pos
	if s.anim[0].active {
		p.X = s.anim[0].to
	}
	if s.anim[1].active {
		p.Y = s.anim[1].to
	}
	return p
}

// animating reports whether an animation or a fling moves s.
func (s *scroller) animating() bool {
	return s.anim[0].active || s.anim[1].active || s.fling[0].active || s.fling[1].active
}

// stop ends the animations and flings of s.
func (s *scroller) stop() {
	s.anim = [2]axisAnim{}
	s.fling = [2]axisFling{}
}

// scrollState is the scrolling state of a Tree (UI thread).
type scrollState struct {
	// list holds the scroll containers (boxes with a scroller).
	list []*scroller
	// now is the latest time of the backend clock the tree saw.
	now time.Duration
	// classic and instant are Config.ClassicScrollbars and
	// Config.InstantScroll.
	classic, instant bool
	// wheel is the latched wheel transaction.
	wheel wheelLatch
	// pending are the scrollers whose UseScroll values wait for the end
	// of the frame.
	pending []*scroller
	// refs are the element refs whose UseScroll values must follow a
	// newly attached element.
	refs []*elemRef
	// reveal is the element focused by keyboard or script, to scroll
	// into view; intoView the elements of ScrollIntoView calls made while
	// the layout was not current.
	reveal   *Element
	intoView []*Element
	// moved reports a paint that moved scrolled content: the pointer's
	// target is hit-tested again after the frame.
	moved bool
}

// wheelLatch is the container a wheel or touchpad gesture scrolls.
type wheelLatch struct {
	s *scroller
	// at is the time of the latest event.
	at time.Duration
	// phased: a touchpad gesture with an end is in progress.
	phased bool
	// hist is the recent motion of a phased gesture, for its fling.
	hist []wheelSample
}

type wheelSample struct {
	at time.Duration
	d  gfx.Point
}

// ---------------------------------------------------------------------
// Animation curves.

// axisAnim animates one axis from from to to over dur from start, on a
// cubic Bézier ease-in-out whose first control point carries the initial
// velocity (Chromium's EaseInOutWithInitialVelocity: x1 = 0.42, y1 =
// 0.42 v, x2 = 0.58, y2 = 1, v the velocity in units of the distance per
// duration).
type axisAnim struct {
	active   bool
	from, to float32
	start    time.Duration
	dur      time.Duration
	y1       float64
}

const easeX1, easeX2 = 0.42, 0.58

// bezier returns the Bézier coordinate of control values c1 and c2 (with
// 0 and 1 at the ends) at parameter u, with explicitly rounded products
// (ADR-041: the same offsets on every architecture).
func bezier(c1, c2, u float64) float64 {
	v := 1 - u
	a := float64(3*float64(float64(v*v)*u)) * c1
	b := float64(3*float64(float64(u*u)*v)) * c2
	return float64(a) + float64(b) + float64(float64(u*u)*u)
}

// bezierSlope returns the derivative of bezier with respect to u.
func bezierSlope(c1, c2, u float64) float64 {
	v := 1 - u
	return float64(float64(3*float64(v*v))*c1) + float64(float64(6*float64(v*u))*float64(c2-c1)) + float64(float64(3*float64(u*u))*float64(1-c2))
}

// ease returns the curve's progress at time fraction x in [0, 1]:
// the u where bezier(x1, x2, u) = x (Newton's method, then bisection,
// like WebKit's UnitBezier), and the curve's y there.
func (a *axisAnim) ease(x float64) float64 {
	if x <= 0 {
		return 0
	}
	if x >= 1 {
		return 1
	}
	u := x
	for range 8 {
		f := bezier(easeX1, easeX2, u) - x
		if f < 1e-7 && f > -1e-7 {
			return bezier(a.y1, 1, u)
		}
		d := bezierSlope(easeX1, easeX2, u)
		if d < 1e-6 && d > -1e-6 {
			break
		}
		u -= f / d
	}
	lo, hi := 0.0, 1.0
	u = x
	for range 40 {
		f := bezier(easeX1, easeX2, u)
		if f < x {
			lo = u
		} else {
			hi = u
		}
		u = float64(lo+hi) / 2
	}
	return bezier(a.y1, 1, u)
}

// at returns the animated value at now, and whether the animation ended.
func (a *axisAnim) at(now time.Duration) (float32, bool) {
	if now >= a.start+a.dur || a.dur <= 0 {
		return a.to, true
	}
	x := float64(now-a.start) / float64(a.dur)
	y := a.ease(x)
	return a.from + float32(float64(float64(a.to-a.from)*y)), false
}

// velocity returns the animation's velocity at now in pixels per second.
func (a *axisAnim) velocity(now time.Duration) float32 {
	if !a.active || now >= a.start+a.dur || a.dur <= 0 {
		return 0
	}
	x := float64(now-a.start) / float64(a.dur)
	if x < 0 {
		x = 0
	}
	// dy/dx at x, from the derivatives with respect to the parameter.
	u := x
	for range 8 {
		f := bezier(easeX1, easeX2, u) - x
		d := bezierSlope(easeX1, easeX2, u)
		if d < 1e-6 && d > -1e-6 {
			break
		}
		u -= f / d
	}
	dx := bezierSlope(easeX1, easeX2, u)
	if dx < 1e-6 {
		dx = 1e-6
	}
	slope := bezierSlope(a.y1, 1, u) / dx
	return float32(float64(float64(slope*float64(a.to-a.from)) / a.dur.Seconds()))
}

// startAnim animates axis i of s from its offset to to over dur, keeping
// the velocity of a running animation (a retarget).
func (s *scroller) startAnim(i int, to float32, dur time.Duration, now time.Duration) {
	a := &s.anim[i]
	from := axis(s.pos, i)
	v := float32(0)
	if a.active {
		v = a.velocity(now)
	}
	s.fling[i] = axisFling{}
	if to == from {
		*a = axisAnim{}
		return
	}
	// The initial velocity in units of the distance per duration.
	nv := float64(float64(v)*dur.Seconds()) / float64(to-from)
	nv = min(max(nv, -1000), 1000)
	*a = axisAnim{active: true, from: from, to: to, start: now, dur: dur, y1: float64(easeX1 * nv)}
}

// axisFling is a kinetic fling of one axis (GTK 4's GtkKineticScrolling
// without overshoot): from from at start with velocity v0, decelerating
// exponentially.
type axisFling struct {
	active bool
	from   float32
	v0     float32
	start  time.Duration
}

// at returns the fling's position and velocity at now.
func (f *axisFling) at(now time.Duration) (pos, vel float32) {
	t := (now - f.start).Seconds()
	if t < 0 {
		t = 0
	}
	e := fmath.Exp(float64(-flingFriction * t))
	pos = f.from + float32(float64(float64(f.v0)*float64(1-e))/flingFriction)
	vel = float32(float64(f.v0) * e)
	return pos, vel
}

// startFling flings s with velocity v (pixels per second, in the
// direction of growing offsets) from now.
func (s *scroller) startFling(v gfx.Point, now time.Duration) {
	for i, vi := range [2]float32{v.X, v.Y} {
		if vi > -flingMinVelocity && vi < flingMinVelocity || !s.user(i) {
			continue
		}
		s.anim[i] = axisAnim{}
		s.fling[i] = axisFling{active: true, from: axis(s.pos, i), v0: vi, start: now}
	}
}

func axis(p gfx.Point, i int) float32 {
	if i == 0 {
		return p.X
	}
	return p.Y
}

func setAxis(p *gfx.Point, i int, v float32) {
	if i == 0 {
		p.X = v
	} else {
		p.Y = v
	}
}

func (s *scroller) user(i int) bool {
	if i == 0 {
		return s.userX
	}
	return s.userY
}

// ---------------------------------------------------------------------
// Scroll containers.

// overflowAxes returns the computed overflow of each axis of cs: visible
// next to an axis that is neither visible nor clip becomes auto, and clip
// becomes hidden (CSS).
func overflowAxes(cs *style.ComputedStyle) (x, y style.Overflow) {
	x, y = cs.OverflowX, cs.OverflowY
	scrolls := func(o style.Overflow) bool { return o != style.OverflowVisible && o != style.OverflowClip }
	if scrolls(x) || scrolls(y) {
		fix := func(o style.Overflow) style.Overflow {
			switch o {
			case style.OverflowVisible:
				return style.OverflowAuto
			case style.OverflowClip:
				return style.OverflowHidden
			}
			return o
		}
		x, y = fix(x), fix(y)
	}
	return x, y
}

// isScrollContainer reports whether a box with style cs is a scroll
// container.
func isScrollContainer(cs *style.ComputedStyle) bool {
	x, y := overflowAxes(cs)
	return x != style.OverflowVisible && x != style.OverflowClip || y != style.OverflowVisible && y != style.OverflowClip
}

func userAxis(o style.Overflow) bool { return o == style.OverflowAuto || o == style.OverflowScroll }

// viewportOverflow returns the overflow of the root box, which is the
// CSS viewport: the root's own overflow propagates to it (RootClass), with
// visible becoming auto and clip becoming hidden (CSS Overflow 3, the
// viewport's rules). So by default content taller or wider than the
// window scrolls, and RootClass "overflow-hidden" turns user scrolling
// of the window off.
func viewportOverflow(o style.Overflow) style.Overflow {
	switch o {
	case style.OverflowVisible:
		return style.OverflowAuto
	case style.OverflowClip:
		return style.OverflowHidden
	}
	return o
}

// classicBars reports whether a scroll container with style cs draws
// classic scrollbars.
func (t *Tree) classicBars(cs *style.ComputedStyle) bool {
	switch cs.Scrollbar {
	case style.ScrollbarClassic:
		return true
	case style.ScrollbarOverlay:
		return false
	}
	return t.scr.classic
}

// userAnimated reports whether user scrolling of a container with style
// cs animates and flings (see the comment at the top of scroll.go).
func (t *Tree) userAnimated(cs *style.ComputedStyle) bool {
	if t.cx.ReducedMotion {
		return false
	}
	switch cs.ScrollBehavior {
	case style.ScrollBehaviorInstant:
		return false
	case style.ScrollBehaviorSmooth:
		return true
	}
	return !t.scr.instant
}

// smoothScripted reports whether programmatic scrolls of a container
// with style cs animate (scroll-behavior: smooth).
func (t *Tree) smoothScripted(cs *style.ComputedStyle) bool {
	return !t.cx.ReducedMotion && cs.ScrollBehavior == style.ScrollBehaviorSmooth
}

// updateScroller gives box bx a scroller when its style makes it a scroll
// container, and takes it away otherwise (the offsets reset, as in
// browsers). Called by the style phase.
func (t *Tree) updateScroller(bx *Box) {
	cs := bx.cs
	if !isScrollContainer(cs) {
		if bx.scr != nil {
			t.dropScroller(bx)
		}
		return
	}
	s := bx.scr
	if s == nil {
		s = &scroller{box: bx}
		bx.scr = s
		t.scr.list = append(t.scr.list, s)
	}
	x, y := overflowAxes(cs)
	s.userX, s.userY = userAxis(x), userAxis(y)
}

// dropScroller removes the scroller of bx.
func (t *Tree) dropScroller(bx *Box) {
	s := bx.scr
	if s == nil {
		return
	}
	bx.scr = nil
	l := t.scr.list
	for i, x := range l {
		if x == s {
			copy(l[i:], l[i+1:])
			l[len(l)-1] = nil
			t.scr.list = l[:len(l)-1]
			break
		}
	}
	if t.scr.wheel.s == s {
		t.scr.wheel = wheelLatch{hist: t.scr.wheel.hist[:0]}
	}
}

// inTree reports whether bx is in the box tree (laid out and painted).
func (t *Tree) inTree(bx *Box) bool {
	for b := bx; b != nil; b = b.parent {
		if b == t.rootBox {
			return true
		}
		if b.ls.Display == layout.DisplayNone || b.inPara != nil {
			return false
		}
	}
	return false
}

// extents updates the largest offsets and the client size of s from the
// layout and clamps its offset. A box out of the box tree has none.
func (t *Tree) extents(s *scroller) {
	bx := s.box
	if bx.el == nil || !bx.el.mounted || !t.inTree(bx) {
		s.max, s.client = gfx.Point{}, gfx.Size{}
	} else {
		l := bx.ld.Layout()
		s.max = gfx.Point{X: l.ScrollWidth(), Y: l.ScrollHeight()}
		s.client = gfx.Size{
			W: max(l.Size.W-l.Border.Left-l.Border.Right-l.ScrollbarSize.W, 0),
			H: max(l.Size.H-l.Border.Top-l.Border.Bottom-l.ScrollbarSize.H, 0),
		}
	}
	t.setScroll(s, s.pos)
	for i := range 2 {
		if a := &s.anim[i]; a.active {
			a.to = min(max(a.to, 0), axis(s.max, i))
		}
	}
}

// ---------------------------------------------------------------------
// Moving.

// setScroll moves s to p, clamped to its extents, and records the
// change: a repaint, the overlay scrollbars shown, OnScroll at the next
// frame and the UseScroll values. It reports whether the offset changed.
func (t *Tree) setScroll(s *scroller, p gfx.Point) bool {
	if t.layoutCurrent() {
		p = s.clamp(p)
	} else {
		// The layout is about to change (the content may grow): clamp to
		// the start only; the frame clamps again after layout.
		p = gfx.Point{X: max(p.X, 0), Y: max(p.Y, 0)}
	}
	if p == s.pos {
		return false
	}
	s.pos = p
	t.markPaint(s.box)
	t.barActivity(s)
	s.eventPending = true
	if t.inFrame {
		if !s.syncPending {
			s.syncPending = true
			t.scr.pending = append(t.scr.pending, s)
		}
	} else {
		t.syncValues(s)
	}
	return true
}

// layoutCurrent reports whether the layout matches the tree (no pending
// work that could change it), so offsets can be clamped now.
func (t *Tree) layoutCurrent() bool { return t.inFrame || !t.changed }

// scrollUser moves s by d as user scrolling: animated (retargeting a
// running animation) when the container's settings allow, else at once.
func (t *Tree) scrollUser(s *scroller, d gfx.Point) {
	if t.userAnimated(s.box.cs) {
		tgt := s.target()
		for i := range 2 {
			di := axis(d, i)
			if di == 0 {
				continue
			}
			to := min(max(axis(tgt, i)+di, 0), axis(s.max, i))
			s.startAnim(i, to, userScrollDuration, t.scr.now)
		}
		t.requestRepaint()
		return
	}
	s.stop()
	t.setScroll(s, s.pos.Add(d))
}

// scrollDirect moves s to p at once (touchpad, touch, thumb drags),
// ending its animations.
func (t *Tree) scrollDirect(s *scroller, p gfx.Point) {
	s.stop()
	t.setScroll(s, p)
}

// scrollScripted moves s to p as a programmatic scroll: animated with
// scroll-smooth, else at once.
func (t *Tree) scrollScripted(s *scroller, p gfx.Point) {
	if !t.smoothScripted(s.box.cs) {
		s.stop()
		t.setScroll(s, p)
		return
	}
	for i := range 2 {
		to := max(axis(p, i), 0)
		if t.layoutCurrent() {
			to = min(to, axis(s.max, i))
		}
		from := axis(s.pos, i)
		d := float64(to - from)
		if d < 0 {
			d = -d
		}
		dur := minSmoothDuration + time.Duration(math.Sqrt(d)*float64(smoothPerSqrtPx))
		s.startAnim(i, to, min(dur, maxSmoothDuration), t.scr.now)
	}
	t.requestRepaint()
}

// requestRepaint asks for a frame that at least paints (a paint-only
// frame when nothing else changed).
func (t *Tree) requestRepaint() {
	t.repaint = true
	if t.inFrame || t.asked || t.onDirty == nil {
		return
	}
	t.asked = true
	t.onDirty()
}

// ---------------------------------------------------------------------
// Chaining.

// canMove reports whether s can still move along d (from its target, so
// that queued notches count).
func (s *scroller) canMove(d gfx.Point) bool {
	tgt := s.target()
	for i := range 2 {
		di := axis(d, i)
		if di == 0 || !s.user(i) {
			continue
		}
		if di > 0 && axis(tgt, i) < axis(s.max, i) || di < 0 && axis(tgt, i) > 0 {
			return true
		}
	}
	return false
}

// contains reports whether s ends a scroll chain along d
// (overscroll-behavior contain or none in an axis of d).
func (s *scroller) contains(d gfx.Point) bool {
	cs := s.box.cs
	return d.X != 0 && cs.OverscrollX != style.OverscrollAuto || d.Y != 0 && cs.OverscrollY != style.OverscrollAuto
}

// chainBox returns the box a scroll chain starts from for el: its own
// box, or its paragraph's, or its nearest ancestor's.
func (t *Tree) chainBox(el *Element) *Box {
	bx := t.geomBox(el)
	if bx == nil || !t.inTree(bx) {
		return nil
	}
	return bx
}

// pickScroller returns the container that scrolls along d for an event
// at el: the first user-scrollable container on el's containing-block
// chain that can move along d, or nil when none can. contained reports
// that a container with overscroll containment ended the chain first.
func (t *Tree) pickScroller(el *Element, d gfx.Point) (s *scroller, contained bool) {
	for b := t.chainBox(el); b != nil; b = containingBlockOf(b) {
		s := b.scr
		if s == nil || !s.userX && !s.userY {
			continue
		}
		if s.canMove(d) {
			return s, false
		}
		if s.contains(d) {
			return nil, true
		}
	}
	return nil, false
}

// containingBlockOf is containingBlock with the computed style for boxes
// not painted yet.
func containingBlockOf(bx *Box) *Box {
	if bx.pcs == nil {
		if bx.cs == nil {
			return bx.parent
		}
		save := bx.pcs
		bx.pcs = bx.cs
		cb := containingBlock(bx)
		bx.pcs = save
		return cb
	}
	return containingBlock(bx)
}

// ---------------------------------------------------------------------
// Wheel and touchpad.

// HandleScroll dispatches a platform scroll event (see the comment at the
// top of scroll.go): OnWheel at the element under the pointer, then the
// default scroll. Call it on the UI thread.
func (t *Tree) HandleScroll(e platform.ScrollEvent) {
	t.scr.now = max(t.scr.now, e.Time)
	if t.root == nil {
		return
	}
	notch := e.Steps != (platform.Point{})
	var d gfx.Point
	if notch {
		d = gfx.Point{X: float32(e.Steps.X * wheelNotch), Y: float32(e.Steps.Y * wheelNotch)}
		if e.Mods&platform.ModShift != 0 && d.X == 0 {
			d.X, d.Y = d.Y, 0
		}
	} else {
		d = gfx.Point{X: e.Delta.X, Y: e.Delta.Y}
	}
	g := &t.scr.wheel
	if d != (gfx.Point{}) {
		pos := gfx.Point{X: e.Pos.X, Y: e.Pos.Y}
		target := t.hitTest(pos)
		if target == nil {
			return
		}
		we := WheelEvent{X: pos.X, Y: pos.Y, DeltaX: d.X, DeltaY: d.Y}
		we.Shift, we.Ctrl, we.Alt, we.Meta = modifiers(e.Mods)
		st := fire(t, evWheel, target, we, true)
		if !st.defaultPrevented() && e.Mods&platform.ModCtrl == 0 {
			t.wheelScroll(target, d, notch, e)
		} else if e.Phase == platform.ScrollPhaseUpdate {
			// A prevented part of a gesture does not count for its fling.
			g.hist = g.hist[:0]
		}
	}
	if e.Phase == platform.ScrollPhaseEnd {
		t.endWheelGesture(e.Time)
	}
}

// wheelScroll is the default action of a wheel event at target: the
// latched container, or a new one picked along the chain, moves by d.
func (t *Tree) wheelScroll(target *Element, d gfx.Point, notch bool, e platform.ScrollEvent) {
	g := &t.scr.wheel
	s := g.s
	expired := !g.phased && e.Time-g.at > latchTimeout
	if s == nil || expired || s.box.el == nil || !s.box.el.mounted || s.box.scr != s {
		s, _ = t.pickScroller(target, d)
		g.s = s
		g.hist = g.hist[:0]
	}
	g.at = e.Time
	g.phased = e.Phase == platform.ScrollPhaseUpdate
	if s == nil {
		return
	}
	if notch {
		s.fling = [2]axisFling{}
		t.scrollUser(s, d)
		return
	}
	t.scrollDirect(s, s.pos.Add(d))
	if g.phased {
		g.hist = append(g.hist, wheelSample{at: e.Time, d: d})
		// Keep the samples of the window before the newest (GTK 4's
		// scroll history).
		n := 0
		for n < len(g.hist) && g.hist[n].at < e.Time-velocityWindow {
			n++
		}
		if n > 0 {
			g.hist = append(g.hist[:0], g.hist[n:]...)
		}
	}
}

// endWheelGesture ends a touchpad gesture at time at: the latched
// container flings with the gesture's recent velocity (GTK 4's
// scroll_history_finish), unless an instant setting applies.
func (t *Tree) endWheelGesture(at time.Duration) {
	g := &t.scr.wheel
	s := g.s
	hist := g.hist
	g.s, g.phased, g.hist = nil, false, g.hist[:0]
	if s == nil || s.box.scr != s || !t.userAnimated(s.box.cs) {
		return
	}
	// Only the motion of the window before the end counts: fingers that
	// rested before lifting do not fling.
	n := 0
	for n < len(hist) && hist[n].at < at-velocityWindow {
		n++
	}
	hist = hist[n:]
	if len(hist) < 2 {
		return
	}
	span := (hist[len(hist)-1].at - hist[0].at).Seconds()
	if span <= 0 {
		return
	}
	var sum gfx.Point
	for _, h := range hist {
		sum = sum.Add(h.d)
	}
	v := gfx.Point{X: float32(float64(sum.X) / span), Y: float32(float64(sum.Y) / span)}
	s.startFling(v, at)
	if s.animating() {
		t.requestRepaint()
	}
}

// breakLatch ends the wheel transaction (the pointer moved, or a button
// was pressed): the next notch picks a container again.
func (t *Tree) breakLatch() {
	if g := &t.scr.wheel; g.s != nil && !g.phased {
		g.s = nil
	}
}

// stopFlings ends every fling (a press stops kinetic scrolling).
func (t *Tree) stopFlings() {
	for _, s := range t.scr.list {
		s.fling = [2]axisFling{}
	}
}

// ---------------------------------------------------------------------
// Keyboard.

// keyScrollKind is what a scrolling key does.
type keyScrollKind uint8

const (
	keyLine keyScrollKind = iota
	keyPage
	keyEdge // Home, End
)

// keyScroll is the default action of a scrolling key: dir is -1 or 1
// along axis i (0: x, 1: y). See the comment at the top of scroll.go.
func (t *Tree) keyScroll(kind keyScrollKind, i int, dir float32) {
	var d gfx.Point
	setAxis(&d, i, dir)
	var s *scroller
	contained := false
	if el := t.foc.el; el != nil && el.mounted {
		s, contained = t.pickScroller(el, d)
	}
	if s == nil && !contained {
		// Nothing around the focus scrolls that way: the "document".
		s = t.outermostScroller(d)
	}
	if s == nil {
		return
	}
	var amount float32
	switch kind {
	case keyLine:
		amount = lineStep
	case keyPage:
		c := s.client.W
		if i == 1 {
			c = s.client.H
		}
		amount = max(float32(float64(c)*pageFraction), 1)
	case keyEdge:
		tgt := axis(s.target(), i)
		if dir > 0 {
			amount = axis(s.max, i) - tgt
		} else {
			amount = tgt
		}
	}
	setAxis(&d, i, dir*amount)
	t.scrollUser(s, d)
}

// outermostScroller returns the scroll container keys scroll when nothing
// is focused: among the outermost user-scrollable containers that can
// move along d, the one with the largest area (the browsers scroll the
// document, which an app's outer scroller stands for).
func (t *Tree) outermostScroller(d gfx.Point) *scroller {
	var best *scroller
	var area float32
	for _, s := range t.scr.list {
		if !s.canMove(d) || !t.inTree(s.box) {
			continue
		}
		outer := true
		for b := s.box.parent; b != nil; b = b.parent {
			if b.scr != nil && (b.scr.userX || b.scr.userY) && b.scr.canMove(d) {
				outer = false
				break
			}
		}
		if !outer {
			continue
		}
		if a := float32(s.client.W * s.client.H); best == nil || a > area {
			best, area = s, a
		}
	}
	return best
}

// ---------------------------------------------------------------------
// Touch.

// panRecognizer returns the pan recognizer of a touch press on scroll
// container s: it competes in the arena with drags and taps and wins past
// the touch slop along the axes s can scroll; the content follows the
// finger and flings on release.
func (t *Tree) panRecognizer(s *scroller) *gesture.Drag {
	ax := gesture.AxisFree
	switch {
	case s.userX && s.max.X > 0 && !(s.userY && s.max.Y > 0):
		ax = gesture.AxisHorizontal
	case s.userY && s.max.Y > 0 && !(s.userX && s.max.X > 0):
		ax = gesture.AxisVertical
	}
	return &gesture.Drag{
		Axis: ax,
		OnStart: func(gesture.DragUpdate) {
			s.stop()
			t.breakLatch()
		},
		OnUpdate: func(u gesture.DragUpdate) {
			if s.box.scr == s {
				t.scrollDirect(s, s.pos.Sub(gfx.Point{X: u.Delta.X, Y: u.Delta.Y}))
			}
		},
		OnEnd: func(u gesture.DragUpdate) {
			if s.box.scr != s {
				return
			}
			t.scrollDirect(s, s.pos.Sub(gfx.Point{X: u.Delta.X, Y: u.Delta.Y}))
			if t.userAnimated(s.box.cs) {
				s.startFling(gfx.Point{X: -u.Velocity.X, Y: -u.Velocity.Y}, u.Time)
				if s.animating() {
					t.requestRepaint()
				}
			}
		},
	}
}

// ---------------------------------------------------------------------
// The frame clock.

// Animate advances the scroll animations, the flings and the overlay
// scrollbars' fades to now, the frame clock (a FrameEvent's time; the
// Host calls it before Frame). Moves are painted by the next Frame.
func (t *Tree) Animate(now time.Duration) {
	t.scr.now = max(t.scr.now, now)
	now = t.scr.now
	for _, s := range t.scr.list {
		if s.animating() {
			p := s.pos
			for i := range 2 {
				if a := &s.anim[i]; a.active {
					v, done := a.at(now)
					setAxis(&p, i, v)
					if done {
						*a = axisAnim{}
					}
				}
				if f := &s.fling[i]; f.active {
					v, vel := f.at(now)
					setAxis(&p, i, v)
					lim := axis(s.max, i)
					if vel > -flingStopVelocity && vel < flingStopVelocity || vel < 0 && v <= 0 || vel > 0 && v >= lim {
						*f = axisFling{}
					}
				}
			}
			t.setScroll(s, p)
		}
		t.fadeBars(s, now)
	}
}

// scrollAnimating reports whether a scroll animation, a fling or a fade
// needs the next frames.
func (t *Tree) scrollAnimating() bool {
	for _, s := range t.scr.list {
		if s.animating() || s.bars.fading(t.scr.now) {
			return true
		}
	}
	return false
}

// scrollDeadline reports the earliest time an overlay scrollbar starts
// fading (the Host arms a timer for it; nothing runs until then).
func (t *Tree) scrollDeadline() (time.Duration, bool) {
	var at time.Duration
	ok := false
	for _, s := range t.scr.list {
		if d, has := s.bars.fadeAt(); has && (!ok || d < at) {
			at, ok = d, true
		}
	}
	return at, ok
}

// ---------------------------------------------------------------------
// Frame integration.

// fireScrollEvents fires OnScroll on the containers whose offset changed
// since the previous frame (start of Frame). It reports whether any
// handler ran.
func (t *Tree) fireScrollEvents() bool {
	ran := false
	for _, s := range t.scr.list {
		if !s.eventPending {
			continue
		}
		s.eventPending = false
		el := s.box.el
		if el == nil || !el.mounted || el.node.d.evMask&evScroll.mask() == 0 {
			continue
		}
		dispatch(t, evScroll, el, s.event(), false, false)
		ran = true
	}
	return ran
}

// event returns the OnScroll event of s.
func (s *scroller) event() ScrollEvent {
	return ScrollEvent{
		ScrollLeft: s.pos.X, ScrollTop: s.pos.Y,
		ScrollWidth: s.client.W + s.max.X, ScrollHeight: s.client.H + s.max.Y,
		ClientWidth: s.client.W, ClientHeight: s.client.H,
	}
}

// maxGutterPasses bounds the relayouts of one frame for classic gutters.
const maxGutterPasses = 3

// scrollPhase runs after layout and the layout effects, before paint: it
// reads the containers' extents and clamps their offsets, reserves or
// drops the classic gutters of overflow: auto containers whose overflow
// changed (laying out again), and scrolls the focused element and the
// pending ScrollIntoView targets into view.
func (t *Tree) scrollPhase() {
	if len(t.scr.list) == 0 && t.scr.reveal == nil && len(t.scr.intoView) == 0 {
		return
	}
	for pass := 0; ; pass++ {
		for _, s := range t.scr.list {
			t.extents(s)
		}
		if pass == maxGutterPasses || !t.updateGutters(pass) {
			break
		}
		t.layoutPhase()
	}
	if el := t.scr.reveal; el != nil {
		t.scr.reveal = nil
		if el.mounted && t.foc.el == el {
			t.scrollIntoView(el)
		}
	}
	for len(t.scr.intoView) > 0 {
		el := t.scr.intoView[0]
		t.scr.intoView = t.scr.intoView[1:]
		if el.mounted {
			t.scrollIntoView(el)
		}
	}
	t.scr.intoView = nil
}

// updateGutters reserves the classic gutter of an overflow: auto axis
// whose content overflows, and drops it when the content fits (browsers
// with classic scrollbars). A gutter is dropped only in the first pass of
// a frame, so content that overflows only without the gutter keeps it
// instead of oscillating. It reports whether a layout style changed.
func (t *Tree) updateGutters(pass int) bool {
	changed := false
	for _, s := range t.scr.list {
		bx := s.box
		if bx.cs == nil {
			continue
		}
		wantV, wantH := false, false
		if t.classicBars(bx.cs) {
			x, y := overflowAxes(bx.cs)
			wantV = y == style.OverflowAuto && (s.max.Y > 0 || s.gutterV && pass > 0)
			wantH = x == style.OverflowAuto && (s.max.X > 0 || s.gutterH && pass > 0)
		}
		if wantV == s.gutterV && wantH == s.gutterH {
			continue
		}
		s.gutterV, s.gutterH = wantV, wantH
		if bx == t.rootBox {
			changed = true // remapPhase maps the root's style again
			continue
		}
		old := bx.ls
		t.remapOne(bx)
		changed = changed || bx.ls != old
	}
	return changed
}

// scrollAfterFrame runs after Frame (outside it, so the work it marks
// requests frames): the UseScroll values of offsets the frame changed,
// the values of refs attached to new elements, a new hit test of the
// pointers when scrolled content moved, and the next frame while
// something animates or OnScroll waits.
func (t *Tree) scrollAfterFrame() {
	for _, s := range t.scr.pending {
		if s.syncPending {
			s.syncPending = false
			t.syncValues(s)
		}
	}
	clear(t.scr.pending)
	t.scr.pending = t.scr.pending[:0]
	for _, r := range t.scr.refs {
		t.syncRef(r)
	}
	clear(t.scr.refs)
	t.scr.refs = t.scr.refs[:0]
	if t.scr.moved {
		t.scr.moved = false
		t.rehover()
	}
	pending := false
	for _, s := range t.scr.list {
		if !s.eventPending {
			continue
		}
		if el := s.box.el; el != nil && el.mounted && el.node.d.evMask&evScroll.mask() != 0 {
			pending = true
		} else {
			s.eventPending = false // nothing listens
		}
	}
	if pending || t.scrollAnimating() {
		t.requestRepaint()
	}
}

// rehover hit-tests the hovering pointers again after scrolled content
// moved under them (Chromium's fake mouse move): their hover states and
// enter and leave events follow the new target. Pressed and captured
// pointers keep their target.
func (t *Tree) rehover() {
	for _, ps := range t.in.ptrs {
		if !ps.inside || ps.key.kind == platform.PointerTouch || ps.buttons != 0 || ps.capture != nil {
			continue
		}
		target := t.hitTest(ps.pos)
		if target != ps.target {
			ps.target = target
			t.updateOver(ps, target)
			t.fireBoundary(ps)
		}
		t.hoverBars(ps)
	}
}

// ---------------------------------------------------------------------
// Programmatic scrolling.

// scrollerOf returns the scroller of el's box, or nil.
func scrollerOf(el *Element) *scroller {
	if el == nil || el.kind != elemPrimitive {
		return nil
	}
	return el.box.scr
}

// scrollIntoView scrolls every scroll container on el's containing-block
// chain so that el's border box is visible, with CSS's "nearest"
// alignment, each container as its scroll-behavior says. Transforms are
// ignored.
func (t *Tree) scrollIntoView(el *Element) {
	bx := t.chainBox(el)
	if bx == nil {
		return
	}
	r := t.targetRect(bx)
	for b := bx; ; {
		cb := containingBlockOf(b)
		if cb == nil {
			return
		}
		if s := cb.scr; s != nil {
			c := t.clientRect(cb)
			tgt := s.target()
			d := gfx.Point{X: nearest(r.X0, r.X1, c.X0, c.X1), Y: nearest(r.Y0, r.Y1, c.Y0, c.Y1)}
			to := s.clamp(tgt.Add(d))
			t.scrollScripted(s, to)
			r = r.Translate(tgt.X-to.X, tgt.Y-to.Y)
		}
		b = cb
	}
}

// nearest returns the scroll delta that brings [lo, hi] into [clo, chi]
// with CSSOM View's "nearest" alignment.
func nearest(lo, hi, clo, chi float32) float32 {
	switch {
	case lo < clo && hi > chi, lo >= clo && hi <= chi:
		return 0 // covers the whole box, or already inside
	case lo < clo && hi-lo <= chi-clo, hi > chi && hi-lo > chi-clo:
		return lo - clo // align the start edges
	default:
		return hi - chi // align the end edges
	}
}

// targetRect returns the border box of bx in window coordinates as it
// will be painted when the scroll animations reach their targets: the
// layout positions minus the targets of the containers on its
// containing-block chain.
func (t *Tree) targetRect(bx *Box) gfx.Rect {
	var o gfx.Point
	for b := bx; b != nil; b = b.parent {
		o = o.Add(b.ld.Layout().Location)
	}
	for b := containingBlockOf(bx); b != nil; b = containingBlockOf(b) {
		if b.scr != nil {
			o = o.Sub(b.scr.target())
		}
	}
	return gfx.RectFromOriginSize(o, bx.ld.Layout().Size)
}

// clientRect returns the client area of scroll container bx (its padding
// box without the gutters) in the coordinates of targetRect.
func (t *Tree) clientRect(bx *Box) gfx.Rect {
	r := t.targetRect(bx)
	l := bx.ld.Layout()
	return gfx.Rect{
		X0: r.X0 + l.Border.Left, Y0: r.Y0 + l.Border.Top,
		X1: r.X1 - l.Border.Right - l.ScrollbarSize.W, Y1: r.Y1 - l.Border.Bottom - l.ScrollbarSize.H,
	}
}

// ScrollTo scrolls the element to offsets (x, y), clamped to its extents
// (CSSOM View's scrollTo): at once, or animated when it has
// scroll-smooth. It does nothing for an element that is not a scroll
// container (overflow auto, scroll or hidden) or not mounted.
func (r ElementRef) ScrollTo(x, y float32) {
	if s := scrollerOf(r.element()); s != nil {
		s.box.tree.scrollScripted(s, gfx.Point{X: x, Y: y})
	}
}

// ScrollBy scrolls the element by (dx, dy) from its current offsets (see
// ScrollTo).
func (r ElementRef) ScrollBy(dx, dy float32) {
	if s := scrollerOf(r.element()); s != nil {
		s.box.tree.scrollScripted(s, s.pos.Add(gfx.Point{X: dx, Y: dy}))
	}
}

// ScrollOffset returns the element's scroll offsets (the DOM's
// scrollLeft and scrollTop): zero when it is not a scroll container or
// not mounted. During a smooth scroll it is the current, animated
// position.
func (r ElementRef) ScrollOffset() (x, y float32) {
	if s := scrollerOf(r.element()); s != nil {
		return s.pos.X, s.pos.Y
	}
	return 0, 0
}

// ScrollIntoView scrolls every scroll container around the element so
// that it is visible, moving each as little as possible (CSSOM View's
// scrollIntoView with block and inline "nearest"); each container jumps
// or animates as its scroll-behavior says. It does nothing when the
// element is not mounted or not displayed.
func (r ElementRef) ScrollIntoView() {
	el := r.element()
	if el == nil {
		return
	}
	t := el.tree
	if t.inFrame || !t.changed {
		t.scrollIntoView(el)
		return
	}
	// The layout is stale: after the next layout.
	t.scr.intoView = append(t.scr.intoView, el)
	t.requestRepaint()
}

// ---------------------------------------------------------------------
// UseScroll.

// scrollFollow is the slot of one UseScroll call: two Values that follow
// the offsets of the element ref is attached to.
type scrollFollow struct {
	ref  *elemRef
	x, y *Value[float32]
}

func (f *scrollFollow) unmount(t *Tree) {
	f.x.unmount(t)
	f.y.unmount(t)
	f.detach()
}

// detach removes f from its ref's followers.
func (f *scrollFollow) detach() {
	l := f.ref.follow
	for i, x := range l {
		if x == f {
			copy(l[i:], l[i+1:])
			l[len(l)-1] = nil
			f.ref.follow = l[:len(l)-1]
			return
		}
	}
}

// UseScroll returns Values that follow the scroll offsets of the element
// ref is attached to (zero while it is not a mounted scroll container):
// read them in deferred style closures to tie effects to scrolling
// without rebuilding (scroll.go). The same Values are returned on every
// build; another ref moves them to its element.
func UseScroll(r ElementRef) (x, y *Value[float32]) {
	if r.r == nil {
		panic("tela: UseScroll(zero ElementRef): pass the ElementRef returned by tela.UseElementRef and attach it with h.Ref to the scroll container")
	}
	el, i := nextSlot(hookScroll)
	if i < len(el.hooks) {
		f, ok := el.hooks[i].(*scrollFollow)
		if !ok {
			el.tree.slotMismatch(el, i, "UseScroll")
		}
		if f.ref != r.r {
			f.detach()
			f.ref = r.r
			r.r.follow = append(r.r.follow, f)
			el.tree.queueRefSync(r.r)
		}
		return f.x, f.y
	}
	f := &scrollFollow{ref: r.r, x: newValue(el, float32(0)), y: newValue(el, float32(0))}
	r.r.follow = append(r.r.follow, f)
	el.hooks = append(el.hooks, f)
	el.tree.queueRefSync(r.r)
	return f.x, f.y
}

// queueRefSync makes the UseScroll values of r follow its element after
// the next frame.
func (t *Tree) queueRefSync(r *elemRef) {
	for _, x := range t.scr.refs {
		if x == r {
			return
		}
	}
	t.scr.refs = append(t.scr.refs, r)
	t.requestRepaint()
}

// syncValues writes the offsets of s to the UseScroll values that follow
// its element.
func (t *Tree) syncValues(s *scroller) {
	el := s.box.el
	if el == nil {
		return
	}
	if r := el.node.d.ref; r != nil && r.el == el && len(r.follow) > 0 {
		for _, f := range r.follow {
			f.x.storeUI(s.pos.X)
			f.y.storeUI(s.pos.Y)
		}
	}
}

// syncRef writes the offsets of r's element (zero for none) to its
// followers.
func (t *Tree) syncRef(r *elemRef) {
	var p gfx.Point
	if el := r.el; el != nil && el.mounted && el.tree == t {
		if s := el.box.scr; s != nil {
			p = s.pos
		}
	}
	for _, f := range r.follow {
		f.x.storeUI(p.X)
		f.y.storeUI(p.Y)
	}
}
