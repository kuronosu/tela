package core

import (
	"math"
	"slices"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/layout"
	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/text"
)

// Anchored positioning (ADR-096): the engine under package aria's
// UseFloating, Floating UI's computePosition and autoUpdate.
//
// # The prop
//
// Floating(FloatOptions) makes a primitive a floating element anchored to
// another element (FloatOptions.Anchor) or to a virtual rectangle
// (AnchorRect). Its Place function is Floating UI's computePosition with
// its middleware: a pure function of rectangles (FloatInput) that returns
// where the floating element goes and, for the size middleware, the
// largest size it may take (FloatResult). Package aria writes Place; core
// runs it at the right time with the right rectangles.
//
// # The float pass
//
// Placing a floating element needs the anchor as painted: its layout,
// the scroll offsets of the containers around it and the transforms of
// the frame (deferred ones, layout animations). The paint phase knows
// them once it has placed the boxes (painter.place, or painter.update in
// a frame that only repaints), so the float pass runs there, before any
// layer is recorded: every floating element of the box tree, in tree
// order (a floating element that its anchor or an ancestor floats in is
// done first), gets its FloatInput, and Place is called when an input
// changed since the last call, when its options changed (a build gave
// the element a new Floating prop) or when a Value it read changed. The
// floating element is painted in a scroll space of its own, like a
// sticky box (sticky.go): the space's offset is the negated distance from
// its laid-out position to the result, snapped to whole device pixels
// (ADR-075), so the box, its descendants and the positioned boxes it
// contains move together, and hit-testing, ElementRef.Bounds, UseBounds,
// the semantics bounds and the compositor see the new position through
// the scroll offsets (shiftOf, floatShift), in the same frame. Moving a
// floating element records nothing: its chunk always carries its
// translation (layer.go) and the damage matches its commands (damage.go).
//
// The position is right in the first frame that shows the element:
// nothing is painted before the pass. A frame that only repaints (an
// ancestor of the anchor scrolls, a TranslateFn or a layout animation
// moves it, a sticky anchor sticks) runs the pass too, without building,
// restyling or laying out, so the floating element follows its anchor
// (Floating UI's autoUpdate) with no frame of its own: when nothing
// changed, no frame runs (0 % idle CPU). A window resize is a layout
// frame and runs it like any other.
//
// # Size limits
//
// A result with HasMaxWidth or HasMaxHeight (the size middleware) limits
// the floating element's border box: its layout style takes the smaller
// of the limit and its own max-width/max-height (a percentage of the
// window for a fixed element, of its parent's padding box otherwise), and
// the tree is laid out again within the frame, then placed again, and the
// pass runs again with the new size (Floating UI's reset). A frame runs at
// most maxFloatRelayouts such layouts; limits that still change after
// them wait for the next frame, which calls Place again and is requested
// for them, up to maxFloatRetryFrames frames in a row for each floating
// element (a flip and a limit that keep undoing each other cannot spin:
// after those frames the limits wait for a frame that runs for another
// reason). The frames are counted per element, so one whose limits never
// settle does not keep another from getting its frames; a run of frames
// that ask in a row is bounded by maxFloatRetryFrames per floating element
// as well (elements that settle in turn cannot keep asking). Scroll containers inside a
// floating element take their new extents in the same frame; a
// VirtualList measures its items in the next one.
//
// # Published data
//
// Place may publish what components render from (the final placement
// after a flip, the arrow's offset) with PublishValue: paint-phase
// readers (an arrow's TranslateFn) are evaluated again in this frame,
// before anything is recorded; builds, layout-phase readers and texts
// that read it run in this frame too, before it is shown: Frame builds,
// styles and lays out once more when the Values that Place published
// changed what they read (rebuildPublished).
//
// # Without an anchor
//
// An anchor that is not mounted or not displayed gives Anchored false.
// Place still runs (it may publish that the anchor is hidden), but the
// floating element stays where it was last placed (Floating UI leaves it
// where it is when its reference goes away); one never placed goes where
// the result says.

// maxFloatRelayouts bounds the layouts that the size limits of floating
// elements run again in one frame; maxFloatRetryFrames the frames in a
// row that the limits of one floating element still changing after them
// ask for.
const (
	maxFloatRelayouts   = 2
	maxFloatRetryFrames = 3
)

// floatAfterFrame asks for the frame that applies the limits a float pass
// left (afterFrame: outside Frame, so that the request is made).
func (t *Tree) floatAfterFrame() {
	if t.flt.again {
		t.flt.again = false
		t.requestRepaint()
	}
}

// FloatInput is what the Place function of a floating element receives,
// in logical pixels and window coordinates (the window's top-left corner
// is 0, 0).
type FloatInput struct {
	// Anchor is the border box of the anchor as painted (ElementRef.Bounds:
	// scroll offsets and transforms applied), or the rectangle AnchorRect
	// returned. Anchored is false when the anchor is not mounted or not
	// displayed; Anchor and AnchorClip are then zero.
	Anchor   Rect
	Anchored bool
	// AnchorClip is the part of the window where the anchor can be seen:
	// the window cut by the overflow clips (padding boxes without the
	// scrollbar gutters) of the anchor's clipping ancestors, the boxes on
	// its containing-block chain that clip their overflow. An anchor
	// outside it is scrolled out of view (Floating UI's hide middleware,
	// referenceHidden). The window for a virtual anchor.
	AnchorClip Rect
	// Floating is the border-box size of the floating element as laid
	// out, with the limits of the last result, unaffected by its own
	// transform.
	Floating Size
	// Clip is the clipping rectangle of the floating element (Floating
	// UI's default boundary, "clippingAncestors", within the viewport):
	// the window cut by the overflow clips of its clipping ancestors, as
	// AnchorClip. For an element in a Portal, or fixed to the window, it
	// is the window; for one that is not (an absolute element inside a
	// scroll container) it is the visible part of those containers.
	Clip Rect
	// Viewport is the window (Floating UI's rootBoundary, "viewport").
	Viewport Rect
	// RTL reports a right-to-left floating element (its direction, Dir):
	// start and end alignments mirror (Floating UI's getRTL).
	RTL bool
	// Arrow is the border-box size of the arrow element (FloatOptions
	// .Arrow) as laid out; zero without one.
	Arrow Size
	// Scale is the display scale (device pixels per logical pixel), for
	// rounding to device pixels.
	Scale float32
}

// FloatResult is what the Place function of a floating element returns.
type FloatResult struct {
	// X and Y are where the top-left corner of the floating element's
	// border box goes, in window coordinates, before its own transform
	// (Floating UI's x and y with a fixed strategy). The element moves by
	// whole device pixels from its laid-out position. A value that is not
	// finite leaves it where it was.
	X, Y float32
	// MaxWidth and MaxHeight, when HasMaxWidth and HasMaxHeight are set,
	// limit the floating element's border box (Floating UI's size
	// middleware): the smaller of the limit and the element's own
	// max-width or max-height wins, and it is laid out again within the
	// frame (see the comment at the top of floating.go). A negative limit
	// is 0; one that is not a number is no limit.
	MaxWidth, MaxHeight       float32
	HasMaxWidth, HasMaxHeight bool
}

// FloatOptions configures Floating.
type FloatOptions struct {
	// Anchor is the reference element: the element the floating element
	// is placed next to.
	Anchor ElementRef
	// AnchorRect is a virtual reference used when Anchor is the zero
	// ElementRef (Floating UI's virtual elements): it returns a rectangle
	// in window coordinates, such as a context menu's pointer position.
	// It runs in the float pass, on the UI thread; a Value it reads with
	// Get places the element again when it changes.
	AnchorRect func() Rect
	// Arrow is the arrow element, whose size Place receives (optional).
	Arrow ElementRef
	// Place computes the position. It runs on the UI thread during the
	// paint phase, so it must be quick and must not build, set state or
	// queue Value changes; it may publish data with PublishValue, and the
	// Values it reads with Get place the element again when they change.
	// It is called again only when its input or its options change (a
	// build that renders the element again gives it new options).
	Place func(FloatInput) FloatResult
}

// Floating makes a primitive a floating element placed by o.Place next
// to its anchor (see the comment at the top of floating.go). It is the
// engine of package aria's UseFloating; it is not exposed by tela or h.
// The element should be positioned (fixed, normally inside a Portal, or
// absolute): it is a stacking context, and moves with the positioned
// boxes it contains.
func Floating(o FloatOptions) Prop {
	if o.Place == nil {
		panic("core: Floating without a Place function")
	}
	opts := &o
	return Prop{name: "Floating", apply: func(d *desc) { d.float = opts }}
}

// floatLimits are the size limits of a result, as applied to the layout.
type floatLimits struct {
	w, h   float32
	hw, hh bool
}

// limitsOf returns the limits of result r.
func limitsOf(r FloatResult) floatLimits {
	var l floatLimits
	if r.HasMaxWidth && !math.IsNaN(float64(r.MaxWidth)) {
		l.w, l.hw = max(r.MaxWidth, 0), true
	}
	if r.HasMaxHeight && !math.IsNaN(float64(r.MaxHeight)) {
		l.h, l.hh = max(r.MaxHeight, 0), true
	}
	return l
}

// floatKey is what a call of Place depended on: its input and the window
// position of the floating element without its own offset.
type floatKey struct {
	in   FloatInput
	base gfx.Point
}

// floatState is the state of a floating box (Box.flt).
type floatState struct {
	opts *FloatOptions
	// space is the box's own scroll space (scroller.floating), created by
	// its first placement.
	space *scroller
	// obs and aobs track the Values read by Place and by AnchorRect.
	obs, aobs *observer
	// key is the input of the last call and called whether there was
	// one; forced asks for a call (new options).
	key    floatKey
	called bool
	forced bool
	// target is the window position of the last result the element
	// follows, and placed whether it has one.
	target gfx.Point
	placed bool
	// limits are the size limits in the layout style.
	limits floatLimits
	// pass is the float pass that visited the box last.
	pass uint64
	// dropped reports that the frame's passes left new limits of this
	// element for the next frame, and retries counts the frames in a row
	// they asked for, up to maxFloatRetryFrames.
	dropped bool
	retries int
}

// floatTree is the float pass state of a Tree (UI thread).
type floatTree struct {
	// boxes are the floating boxes of the last place, in tree order.
	boxes []*Box
	// pass numbers the passes; moved reports that an offset changed in
	// the frame.
	pass  uint64
	moved bool
	// drops are the floating elements whose new limits the frame's passes
	// left for the next frame; again asks for that frame (afterFrame), and
	// run counts the frames in a row that asked.
	drops []*floatState
	again bool
	run   int
	// published are the Values that the Place functions of the frame
	// published, for rebuildPublished: PublishValue records each one while
	// placing (a Place function runs) counts above zero, also a Value
	// already published earlier in the frame (a layout effect), which is
	// in Tree.pubs once. It may repeat a Value.
	published []*valueCore
	placing   int
}

// updateFloat gives the box of primitive el the Floating prop of its node
// (mount: old nil).
func (t *Tree) updateFloat(el *Element, old *desc) {
	d := el.node.d
	bx := el.box
	switch {
	case d.float == nil && bx.flt != nil:
		f := bx.flt
		dropObserver(f.obs)
		dropObserver(f.aobs)
		bx.flt = nil
		if f.limits != (floatLimits{}) {
			bx.queueRemap() // the limits go
		}
		t.dirty()
	case d.float != nil && bx.flt == nil:
		bx.flt = &floatState{opts: d.float, forced: true}
		t.dirty()
	case d.float != nil && (old == nil || old.float != d.float):
		bx.flt.opts, bx.flt.forced = d.float, true
	}
}

// floatSpace returns the scroll space of floating box bx, created on first
// use; up is the space bx would be painted in without it.
func floatSpace(bx *Box, up *scroller) *scroller {
	f := bx.flt
	k := f.space
	if k == nil {
		k = &scroller{box: bx, floating: true}
		f.space = k
	}
	k.up = up
	k.interest = unbounded
	return k
}

// floatShift returns how far bx's own floating offset moves it (zero for a
// box that is not floating), from the last paint.
func floatShift(bx *Box) gfx.Point {
	if f := bx.flt; f != nil && f.space != nil && bx.space == f.space {
		return f.space.soff.Neg()
	}
	return gfx.Point{}
}

// ownShift returns how far bx's own scroll space moves it: its sticky or
// floating offset.
func ownShift(bx *Box) gfx.Point {
	return stickyShift(bx).Add(floatShift(bx))
}

// floatPhase is the float pass of a paint (see the comment at the top of
// floating.go), after the boxes were placed or updated: it places the
// floating elements, lays the tree out again for their size limits
// (placing the boxes again: p.full is set), and evaluates again the
// paint-phase readers of the Values that Place published. It reports
// whether it placed the boxes again.
func (t *Tree) floatPhase(p *painter) (replaced bool) {
	F := &t.flt
	if len(F.boxes) == 0 {
		return false
	}
	start := len(t.paintQ)
	F.moved = false
	first := F.pass + 1 // the first pass of this frame
	for pass := 0; ; pass++ {
		F.pass++
		relayout := false
		// Index loop: the slice is rebuilt by place, after the loop.
		for i := 0; i < len(F.boxes); i++ {
			if t.floatOne(p, F.boxes[i], pass < maxFloatRelayouts) {
				relayout = true
			}
		}
		if !relayout {
			break
		}
		t.floatRelayout()
		p.full = true
		p.place(t.rootBox, nil)
		replaced = true
	}
	t.floatRetries(first)
	if F.moved && len(t.stickies) > 0 {
		// A sticky box in a floating element whose scroll container is
		// outside it.
		p.updateSticky()
	}
	if n := len(t.paintQ); n > start {
		// The paint-phase readers of published Values (and boxes marked
		// by the relayout): evaluated again now.
		marked := make([]*Box, n-start)
		copy(marked, t.paintQ[start:n])
		t.lay.needPlace = false
		p.update(marked)
		if t.lay.needPlace {
			// One of them starts or stops being a layer of its own.
			t.lay.needPlace = false
			p.full = true
			p.place(t.rootBox, nil)
			replaced = true
		}
	}
	return replaced
}

// floatRetries asks for the frame that applies the limits that the passes
// of this frame (from pass first on) dropped. Each floating element asks
// for at most maxFloatRetryFrames frames in a row: its count starts again
// in a frame that places it without dropping limits, so one whose limits
// never settle spends its own frames and cannot starve another. The
// frames that ask in a row are bounded too (maxFloatRetryFrames per
// floating element): elements that drop limits in turn, each settling
// while another drops, would otherwise keep their counts low and ask
// forever.
func (t *Tree) floatRetries(first uint64) {
	F := &t.flt
	for _, bx := range F.boxes {
		if f := bx.flt; f != nil && f.pass >= first && !f.dropped {
			f.retries = 0
		}
	}
	ask := false
	for _, f := range F.drops {
		f.dropped = false
		if f.retries < maxFloatRetryFrames {
			f.retries++
			ask = true
		}
	}
	clear(F.drops)
	F.drops = F.drops[:0]
	switch {
	case !ask:
		F.run = 0
	case F.run < maxFloatRetryFrames*len(F.boxes):
		F.run++
		F.again = true
	}
}

// floatRelayout lays the tree out again after size limits changed, with
// the extents and classic gutters of the scroll containers; virtual lists
// measure their items in the next frame (they would build in this one).
func (t *Tree) floatRelayout() {
	t.layoutPhase()
	for pass := 0; ; pass++ {
		for _, s := range t.scr.list {
			if s != nil {
				t.extents(s)
			}
		}
		t.scr.stale = false
		if pass >= maxGutterPasses || !t.updateGutters(pass) {
			break
		}
		t.layoutPhase()
	}
	if len(t.virt.lists) > 0 {
		t.virt.again = true
	}
	t.syncSizes()
}

// floatOne places floating box bx in pass F.pass, after the floating
// elements it depends on (those it or its anchor float in). relayout
// allows new size limits; it reports whether limits changed the layout
// style (a layout must run again).
func (t *Tree) floatOne(p *painter, bx *Box, relayout bool) (laid bool) {
	F := &t.flt
	f := bx.flt
	if f == nil || f.pass == F.pass {
		return false
	}
	f.pass = F.pass
	if f.space == nil || bx.space != f.space || bx.el == nil || !bx.el.mounted || !t.inTree(bx) {
		return false
	}
	for s := f.space.up; s != nil; s = s.outer() {
		if s.floating && s.box != bx {
			laid = t.floatOne(p, s.box, relayout) || laid
		}
	}
	anchorEl := f.opts.Anchor.element()
	var anchorBox *Box
	if anchorEl != nil && t.displayed(anchorEl) {
		if anchorBox = t.geomBox(anchorEl); anchorBox != nil && !t.inTree(anchorBox) {
			anchorBox = nil
		}
		if anchorBox != nil {
			for s := anchorBox.space; s != nil; s = s.outer() {
				if s.floating && s.box != bx {
					laid = t.floatOne(p, s.box, relayout) || laid
				}
			}
		}
	}

	view := gfx.Rect{X1: t.viewport.W, Y1: t.viewport.H}
	in := FloatInput{Viewport: rectOf(view), Scale: t.scale}
	l := bx.ld.Layout()
	in.Floating = Size{Width: l.Size.W, Height: l.Size.H}
	in.RTL = bx.cs.Text.Direction == text.DirectionRTL
	in.Clip = rectOf(t.clipOf(bx, view))
	switch {
	case anchorBox != nil:
		in.Anchored = true
		in.Anchor = rectOf(t.elemBounds(anchorEl))
		in.AnchorClip = rectOf(t.clipOf(anchorBox, view))
	case f.opts.Anchor.r == nil && f.opts.AnchorRect != nil:
		in.Anchored = true
		in.Anchor = t.floatAnchorRect(bx, f)
		in.AnchorClip = in.Viewport
	}
	if a := f.opts.Arrow.element(); a != nil && t.ownBox(a, false) && t.inTree(a.box) {
		al := a.box.ld.Layout()
		in.Arrow = Size{Width: al.Size.W, Height: al.Size.H}
	}
	key := floatKey{in: in, base: bx.u.Origin().Sub(shiftOf(f.space.up))}
	if f.forced || !f.called || bx.needsPaint || key != f.key {
		res := t.callPlace(bx, f, in)
		f.key, f.called, f.forced = key, true, false
		if (in.Anchored || !f.placed) && finite32(res.X) && finite32(res.Y) {
			f.target, f.placed = gfx.Point{X: res.X, Y: res.Y}, true
		}
		if lim := limitsOf(res); lim != f.limits {
			if relayout {
				f.limits = lim
				old := bx.ls
				t.remapOne(bx)
				laid = laid || bx.ls != old
			} else {
				// The frame's layouts are spent: Place runs again in the
				// next frame (its input is the same, so the key alone
				// would not call it) and applies the limits then.
				f.forced = true
				if !f.dropped {
					f.dropped = true
					F.drops = append(F.drops, f)
				}
			}
		}
	}
	var off gfx.Point
	if f.placed {
		off = gfx.Point{X: p.snap(f.target.X - key.base.X), Y: p.snap(f.target.Y - key.base.Y)}
	}
	if soff := off.Neg(); soff != f.space.soff {
		f.space.soff = soff
		F.moved = true
		t.scr.moved = true // hover follows (scrollAfterFrame)
	}
	return laid
}

// callPlace calls the Place function of floating box bx, tracking the
// Values it reads (a change places bx again).
func (t *Tree) callPlace(bx *Box, f *floatState, in FloatInput) FloatResult {
	mark := t.beginTrack()
	// What it publishes is recorded by PublishValue (placePublished).
	t.flt.placing++
	defer func() { t.flt.placing-- }()
	var res FloatResult
	func() {
		done := false
		defer func() {
			if !done { // Place panicked: keep the old subscriptions
				t.endTrack(mark)
			}
		}()
		res = f.opts.Place(in)
		done = true
	}()
	t.observe(&f.obs, obsPaint, nil, bx, t.endTrack(mark))
	return res
}

// rebuildPublished makes what the float pass published, and builds read,
// take effect before the frame is painted: the Values that the Place
// functions of this frame published (the final placement after a flip, say)
// invalidate their build, layout and text readers now instead of after the
// frame, and rebuildPublished reports whether any reader was hit, so that
// Frame builds, styles and lays out again before it presents anything. The
// first paint of the frame is then never shown: a menu that flips above its
// trigger is styled for the top side from its first frame, not one frame
// later (ADR-164). A Value is applied here when a Place function published
// it, also if other code had published it earlier in the frame; Values
// published only by other code wait for the end of the frame as ever.
func (t *Tree) rebuildPublished() bool {
	F := &t.flt
	pubs := F.published
	F.published = nil
	hit := false
	for _, v := range pubs {
		if !v.pub {
			continue
		}
		v.pub = false
		t.pubs = slices.DeleteFunc(t.pubs, func(o *valueCore) bool { return o == v })
		if v.el.mounted && t.valueChangedSkip(v, obsPaint) {
			hit = true
		}
	}
	return hit
}

// floatAnchorRect evaluates the virtual anchor of floating box bx,
// tracking the Values it reads.
func (t *Tree) floatAnchorRect(bx *Box, f *floatState) Rect {
	mark := t.beginTrack()
	var r Rect
	func() {
		done := false
		defer func() {
			if !done {
				t.endTrack(mark)
			}
		}()
		r = f.opts.AnchorRect()
		done = true
	}()
	t.observe(&f.aobs, obsPaint, nil, bx, t.endTrack(mark))
	return r
}

func finite32(v float32) bool {
	return !math.IsNaN(float64(v)) && !math.IsInf(float64(v), 0)
}

// displayed reports whether el is mounted, styled and not inside
// display: none.
func (t *Tree) displayed(el *Element) bool {
	if el == nil || !el.mounted {
		return false
	}
	for a := el; a != nil; a = a.styleParent() {
		if a.kind != elemPrimitive {
			continue
		}
		if cs := a.box.cs; cs == nil || cs.Display == style.DisplayNone {
			return false
		}
	}
	return true
}

// clipOf returns the part of view that bx can be seen in: view cut by the
// overflow clips of the boxes on its containing-block chain (the root's
// is the window), each its padding box without the scrollbar gutters in
// window coordinates, as painted.
func (t *Tree) clipOf(bx *Box, view gfx.Rect) gfx.Rect {
	r := view
	cb := containingBlock(bx)
	for a := bx.parent; a != nil && a != t.rootBox; a = a.parent {
		if a != cb {
			continue
		}
		if a.pcs != nil && clipsOverflow(a.pcs) {
			r = r.Intersect(t.clientWindowRect(a))
		}
		if fixedToWindow(a, containingBlock(a)) {
			break
		}
		cb = containingBlock(a)
	}
	if r.Empty() {
		return gfx.Rect{}
	}
	return r
}

// clientWindowRect returns the padding box of bx without its scrollbar
// gutters, in window coordinates as painted (its scroll space's offsets,
// the transforms of bx and its ancestors as a bounding box).
func (t *Tree) clientWindowRect(bx *Box) gfx.Rect {
	return t.transformUp(bx, clientArea(bx, windowRect(bx)))
}

// limitLayout applies the size limits of floating box b to its layout
// style ls (mapLayoutStyle).
func (f *floatState) limitLayout(b *Box, ls *layout.Style) {
	if !f.limits.hw && !f.limits.hh {
		return
	}
	// The basis of percentages: the window for a fixed box, the parent's
	// padding box otherwise (from the last layout).
	basis := gfx.Size{W: b.tree.viewport.W, H: b.tree.viewport.H}
	if (b.cs == nil || b.cs.Position != style.PositionFixed) && b.parent != nil {
		pl := b.parent.ld.Layout()
		basis = gfx.Size{W: pl.Size.W - pl.Border.Left - pl.Border.Right, H: pl.Size.H - pl.Border.Top - pl.Border.Bottom}
	}
	if f.limits.hw {
		ls.MaxWidth = minDim(ls.MaxWidth, f.limits.w, basis.W)
	}
	if f.limits.hh {
		ls.MaxHeight = minDim(ls.MaxHeight, f.limits.h, basis.H)
	}
}

// minDim returns the smaller of max size d (resolved against basis) and
// limit v, as a length; v alone for a max size that is not a length or a
// percentage.
func minDim(d layout.Dim, v, basis float32) layout.Dim {
	switch d.Unit {
	case layout.UnitPx:
		return layout.Px(min(d.Value, v))
	case layout.UnitPercent:
		return layout.Px(min(float32(d.Value*basis), v))
	case layout.UnitCalc:
		return layout.Px(min(d.Px+float32(d.Value*basis), v))
	}
	return layout.Px(v)
}

// ---------------------------------------------------------------------
// Publishing.

// PublishValue stores x in v at once on the UI thread, for code that runs
// inside a frame: the Place function of a floating element publishing
// the final placement or the arrow's offset. The paint-phase readers of
// v (a TranslateFn, an OpacityFn) are evaluated again in this frame,
// before anything is painted; what a Place function publishes is read by
// builds, layout-phase readers and texts in the same frame (the frame runs
// again before it is shown), the Values published by other code in the
// next frame. Outside a frame, and in a component build (in a frame or
// not), it is the UI thread's direct store: every reader is invalidated at
// once, so the phases still to come read the value (in a frame's build,
// the rest of the build phase, the styles, layout, texts, the text fields
// bound to v and paint; a controlled color picker's hex field and code
// change in the frame whose build changed the color, ADR-162). Queued
// Set and Update calls of v still apply after it; a comparable value
// == to the current one changes nothing (but a Place function publishing
// the value that other code published earlier in the frame still has its
// readers follow in this frame). Call it on the UI thread only.
func PublishValue[T any](v *Value[T], x T) {
	cur := v.load("PublishValue")
	t := v.el.tree
	if v.comparable && safeEqual(any(*cur), any(x)) {
		if v.pub {
			t.placePublished(&v.valueCore) // pending since earlier in the frame
		}
		return
	}
	p := new(T)
	*p = x
	if !t.inFrame || (current != nil && current.tree == t) {
		v.store(p)
		return
	}
	v.cur.Store(p)
	t.publishChanged(&v.valueCore)
}

// publishChanged marks the paint-phase readers of v, changed inside a
// frame, for painting, and queues its other readers for the end of the
// frame (publishAfterFrame), or for rebuildPublished when a Place
// function published it.
func (t *Tree) publishChanged(v *valueCore) {
	for _, s := range v.subs {
		if o := s.obs; o.kind == obsPaint && (o.box.parent != nil || o.box.inPara != nil || contentsBox(o.box)) {
			t.markPaint(o.box)
		}
	}
	if !v.pub {
		v.pub = true
		t.pubs = append(t.pubs, v)
	}
	t.placePublished(v)
}

// placePublished records published Value v for rebuildPublished when a
// Place function is running (callPlace). v is recorded whether or not this
// publish queued it: a Value that a layout effect published earlier in the
// frame is in t.pubs already, and what Place publishes to it must still be
// read in this frame (ADR-164).
func (t *Tree) placePublished(v *valueCore) {
	if t.flt.placing > 0 {
		t.flt.published = append(t.flt.published, v)
	}
}

// publishAfterFrame invalidates the readers other than paint-phase ones
// of the Values published in the frame (afterFrame: outside Frame, so
// that they request the next one).
func (t *Tree) publishAfterFrame() {
	if len(t.pubs) == 0 {
		return
	}
	pubs := t.pubs
	t.pubs = nil
	for _, v := range pubs {
		v.pub = false
		if v.el.mounted {
			t.valueChangedSkip(v, obsPaint)
		}
	}
	clear(pubs)
	t.pubs = pubs[:0]
}
