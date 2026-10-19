package core

import (
	"time"

	"github.com/kuronosu/tela/internal/anim"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
)

// Layout animations (ARCHITECTURE section 12, ADR-081), after Motion's
// layout and layoutId props: AnimateLayout makes a box move from its old
// laid-out box to its new one whenever layout changes it, and LayoutID
// makes a box that mounts start from the box of the element with the
// same ID that unmounted in the same commit (or is exiting under a
// Presence).
//
// # FLIP
//
// Layout runs normally and the box is laid out at its new place at once.
// The paint phase then draws it through a transform (its flip) that maps
// the new box to the visual box of this frame, which moves from where the
// box was seen last to the new box: First, Last, Invert, Play. The flip is
// folded into the box's paint-phase transform (Box.pcs), so everything
// that already follows transforms follows it: painting, hit-testing,
// ElementRef.Bounds and UseBounds, semantics bounds. A box with a
// layout animation in progress is a deferred layer (layer.go), like
// ADR-076's transform animations: each frame of the animation marks it
// for painting only (no build, style or layout), and the compositor
// pushes the new transform.
//
// # Size changes
//
// A change of size is a scale, as in Motion: the box is painted at its
// new size and scaled to the visual size of the frame. The corner radii
// and box shadows of the box are corrected by the inverse scale (Motion's
// scale correction), so they look as they will at rest; its text and its
// children are scaled with it, unless they animate their layout too: a
// box with a layout animation inside another one is counter-scaled, so it
// keeps its own visual size (Motion's tree scale). A clip-path, background
// images and bg-clip-text are in the box's own space: they scale with the
// flip, uncorrected, as under a CSS transform (Motion has no corrector for
// them either).
//
// # Coordinates
//
// Boxes are compared in the coordinates boxes are placed in (Box.u): the
// layout positions without the scroll offsets of the containers they are
// in and without any transform. Scrolling, transforms of ancestors (a
// hover:scale) and the display scale do not move a box there, so they
// start no animation; nor does a sticky box that sticks, whose offset is
// that of its own scroll space (sticky.go), nor a fixed box when the root
// scrolls. A box inside another box with a layout animation
// (its layout parent) is measured relative to the parent's box, and its
// visual box is placed relative to the parent's visual box: a child that
// keeps its place in a parent that moves moves with it, whatever their
// timings (Motion's relative targets).
//
// # Timing and interruptions
//
// A box whose classes have duration-* or ease-* animates with a tween of
// that duration, easing and delay-* (the theme's default duration or
// easing fills the one that is missing), like a CSS transition; otherwise
// with a spring of stiffness 500 and damping 25, the one Motion animates
// x and y with. Motion's own default for layout animations is a 450 ms
// tween that restarts at speed 0 when it is interrupted; Tela uses the
// spring so that an interrupted animation keeps its velocity: a layout
// change during an animation starts the next one from the visual box of
// that frame, at the speed the box moves.
//
// # When nothing animates
//
// A box that mounts or starts being displayed appears at its place
// (unless a LayoutID hands it a box). With reduced motion, and in trees
// that no frame clock drives (Render, tela snapshot), boxes jump to their
// new place. A box keeps no transform once its animation ends: at rest it
// is painted exactly as without the prop.

// layoutSpring is the spring of a layout animation without duration-* or
// ease-* classes: Motion's spring for x and y (stiffness 500, damping 25,
// rest speed 10).
var layoutSpring = anim.MotionSpec{Spring: true, Physics: anim.Spring{Stiffness: 500, Damping: 25, Mass: 1}, RestSpeed: 10}

// layoutAnim is the layout animation state of a box.
type layoutAnim struct {
	bx *Box
	// id is the box's LayoutID ("" for AnimateLayout only).
	id string
	// rel is the frame target is measured in: the box's layout parent
	// (the nearest ancestor with a layout animation) when it has one, the
	// coordinates of Box.u otherwise.
	rel *Box
	// target is the box's border box (Box.u) in rel's frame: relative to
	// the origin of rel's border box.
	target gfx.Rect
	// m is the motion of the visual box toward target, in rel's frame,
	// as x, y, width and height; running while it is in progress.
	m       anim.Motion
	running bool
	// placed is the place pass that placed the box last (layoutState).
	placed uint64
	// The visual box of the paint memo (layoutState.paints), in Box.u
	// coordinates; origin is the visual origin of rel's frame and active
	// reports a flip (the box or its layout parent animates).
	memo   uint64
	vis    gfx.Rect
	origin gfx.Point
	active bool
	// scale is the scale of the box's visual box over its layout box,
	// which its radii and shadows are corrected for.
	scale gfx.Point
	// listed: in layoutState.active.
	listed bool
}

// layoutState is the layout animation state of a Tree.
type layoutState struct {
	// places counts the place passes of paintPhase, paints its paints.
	places, paints uint64
	// ids are the boxes with each LayoutID.
	ids map[string][]*layoutAnim
	// gone are the window boxes of the LayoutID elements unmounted since
	// the last paint, for the elements with their IDs that mount.
	gone map[string]gfx.Rect
	// active are the boxes with a flip at their last paint, which the
	// ticker marks for painting.
	active []*Box
	tick   *layoutTicker
}

// layoutTicker is the anim.Animator of the layout animations of a tree.
type layoutTicker struct{ t *Tree }

// Advance implements anim.Animator: it marks the boxes that move for
// painting and reports whether a motion goes on after now.
func (lt *layoutTicker) Advance(now time.Duration) anim.State {
	t := lt.t
	s := &t.la
	running := false
	live := s.active[:0]
	for _, bx := range s.active {
		la := bx.lanim
		if la == nil || bx.el == nil || !bx.el.mounted {
			if la != nil {
				la.listed = false
			}
			continue
		}
		if !la.active && !la.running {
			la.listed = false
			continue
		}
		live = append(live, bx)
		t.markPaint(bx)
		if la.running && !t.cx.ReducedMotion {
			if _, _, done := la.m.At(now); !done {
				running = true
			}
		}
	}
	clear(s.active[len(live):])
	s.active = live
	if running {
		return anim.Running
	}
	return anim.Done
}

// ticker returns the tree's layout animator.
func (s *layoutState) ticker(t *Tree) *layoutTicker {
	if s.tick == nil {
		s.tick = &layoutTicker{t: t}
	}
	return s.tick
}

// layoutActive reports whether bx is drawn through a flip.
func layoutActive(bx *Box) bool { return bx.lanim != nil && bx.lanim.active }

// wantsLayoutAnim reports whether the element of bx asks for a layout
// animation.
func (t *Tree) wantsLayoutAnim(bx *Box) bool {
	if bx.el == nil || bx == t.rootBox || bx.el.node.d == nil {
		return false
	}
	d := bx.el.node.d
	return d.animLayout || d.layoutID != ""
}

// syncLayoutAnim creates, updates or drops the layout animation state of
// bx for its element's props.
func (t *Tree) syncLayoutAnim(bx *Box) {
	if !t.wantsLayoutAnim(bx) {
		if bx.lanim != nil {
			t.unlistLayoutID(bx.lanim)
			bx.lanim = nil
		}
		return
	}
	id := bx.el.node.d.layoutID
	la := bx.lanim
	if la == nil {
		la = &layoutAnim{bx: bx, scale: gfx.Point{X: 1, Y: 1}}
		bx.lanim = la
	} else if la.id == id {
		return
	}
	t.unlistLayoutID(la)
	la.id = id
	if id != "" {
		if t.la.ids == nil {
			t.la.ids = map[string][]*layoutAnim{}
		}
		t.la.ids[id] = append(t.la.ids[id], la)
	}
}

// unlistLayoutID takes la out of the registry of its LayoutID.
func (t *Tree) unlistLayoutID(la *layoutAnim) {
	if la.id == "" {
		return
	}
	list := t.la.ids[la.id]
	for i, o := range list {
		if o == la {
			list[i] = list[len(list)-1]
			list[len(list)-1] = nil
			list = list[:len(list)-1]
			break
		}
	}
	if len(list) == 0 {
		delete(t.la.ids, la.id)
	} else {
		t.la.ids[la.id] = list
	}
}

// dropLayoutAnim forgets the layout animation of a box whose element
// unmounts. A box with a LayoutID leaves its window box, as painted last,
// for an element with the same ID that mounts in the same commit.
func (t *Tree) dropLayoutAnim(bx *Box) {
	la := bx.lanim
	if la.id != "" && la.placed != 0 && bx.pcs != nil && t.inTree(bx) {
		if t.la.gone == nil {
			t.la.gone = map[string]gfx.Rect{}
		}
		t.la.gone[la.id] = t.boxWindowRect(bx)
	}
	t.unlistLayoutID(la)
	la.running, la.active = false, false
	bx.lanim = nil
}

// layoutParent returns the nearest ancestor of bx with a layout animation.
func (t *Tree) layoutParent(bx *Box) *Box {
	for a := bx.parent; a != nil && a != t.rootBox; a = a.parent {
		if a.lanim != nil {
			return a
		}
	}
	return nil
}

// layoutSource returns the window box a box with LayoutID id that mounts
// starts from: the element with that ID that unmounted in this commit, or
// one that is exiting under a Presence.
func (t *Tree) layoutSource(la *layoutAnim) (gfx.Rect, bool) {
	if r, ok := t.la.gone[la.id]; ok {
		return r, true
	}
	for _, o := range t.la.ids[la.id] {
		b := o.bx
		if o != la && o.placed != 0 && b.owner != nil && b.owner.inert && b.pcs != nil && t.inTree(b) {
			return t.boxWindowRect(b), true
		}
	}
	return gfx.Rect{}, false
}

// layoutTiming returns the motion of a layout animation of a box with
// style cs and its delay: a tween when its classes set a duration or an
// easing, the layout spring otherwise.
func (t *Tree) layoutTiming(cs *style.ComputedStyle) (anim.MotionSpec, time.Duration) {
	tr := cs.Transition
	if !tr.DurationSet && !tr.TimingSet {
		return layoutSpring, 0
	}
	th := t.cx.Theme
	if th == nil {
		th = style.DefaultTheme()
	}
	d, c := th.DefaultTransition.Duration, th.DefaultTransition.Timing
	if tr.DurationSet {
		d = tr.Duration
	}
	if tr.TimingSet {
		c = tr.Timing
	}
	return anim.MotionSpec{Duration: d, Curve: animCurve(c)}, tr.Delay
}

func rectVec(r gfx.Rect) anim.Vec {
	return anim.Vec{N: 4, C: [4]float64{float64(r.X0), float64(r.Y0), float64(r.Width()), float64(r.Height())}}
}

func vecRect(v anim.Vec) gfx.Rect {
	x, y := float32(v.C[0]), float32(v.C[1])
	return gfx.Rect{X0: x, Y0: y, X1: x + float32(v.C[2]), Y1: y + float32(v.C[3])}
}

// current returns the visual box of the motion at now in rel's frame (x,
// y, width, height) and its velocity: the target, at rest, when nothing
// runs.
func (la *layoutAnim) current(now time.Duration) (pos, vel anim.Vec) {
	if la.running {
		if x, v, done := la.m.At(now); !done {
			return x, v
		}
	}
	return rectVec(la.target), anim.Vec{N: 4}
}

// startLayout begins the motion from the visual box from (in rel's
// frame), moving at vel, to la.target at now.
func (t *Tree) startLayout(la *layoutAnim, from, vel anim.Vec, now time.Duration) {
	spec, delay := t.layoutTiming(la.bx.cs)
	la.running = false
	if !spec.Spring {
		vel = anim.Vec{N: 4} // tweens do not carry a velocity
		if spec.Duration <= 0 {
			return
		}
	}
	vel.N = 4
	to := rectVec(la.target)
	if !from.Finite() || !vel.Finite() || !to.Finite() || (from.Same(to) && vel.Same(anim.Vec{N: 4})) {
		return
	}
	la.m = anim.NewMotion(spec, from, to, vel, now+delay)
	la.running = true
	t.anim.tk.Add(t.la.ticker(t))
}

// frameOrigin returns the visual origin of the frame of a box whose layout
// parent is rel at this paint (the origin of the window coordinates
// without scroll offsets when rel is nil).
func (t *Tree) frameOrigin(rel *Box) gfx.Point {
	if rel == nil || rel.lanim == nil {
		return gfx.Point{}
	}
	v, _ := t.layoutVisual(rel)
	return v.Origin()
}

// layoutPlace is called by place for a box whose element has
// AnimateLayout or LayoutID, once its border box (u) and scroll space are
// known: it starts, retargets or ends its animation and folds the flip of
// this frame into its paint-phase style.
func (p *painter) layoutPlace(bx *Box) {
	t := p.t
	t.syncLayoutAnim(bx)
	la := bx.lanim
	if la == nil {
		bx.deferPaint = hasPhase(bx.cs, style.PhasePaint)
		return
	}
	rel := t.layoutParent(bx)
	tgt := bx.u
	if rel != nil {
		tgt = tgt.Translate(-rel.u.X0, -rel.u.Y0)
	}
	now := t.anim.tk.Now()
	animate := t.anim.driven && !t.cx.ReducedMotion
	switch places := t.la.places; {
	case la.placed == places:
		// Placed again in the same pass (ReferenceFrame): what it found
		// holds.
	case la.placed == 0 || la.placed+1 != places:
		// The box appears (it mounts, or its display: none ended): at
		// its place, or from the box a LayoutID hands it.
		la.rel, la.target, la.running = rel, tgt, false
		if la.id == "" || !animate {
			break
		}
		if src, ok := t.layoutSource(la); ok {
			// The window box, in Box.u coordinates (transforms of the
			// ancestors are not undone: they are rare around shared
			// elements), then in the frame.
			d := shiftOf(bx.space)
			o := t.frameOrigin(rel)
			from := src.Translate(d.X-o.X, d.Y-o.Y)
			t.startLayout(la, rectVec(from), anim.Vec{N: 4}, now)
		}
	case tgt != la.target || rel != la.rel:
		if !animate {
			la.rel, la.target, la.running = rel, tgt, false
			break
		}
		// Retarget from the visual box of this frame, at its speed.
		cur, vel := la.current(now)
		if rel != la.rel {
			o := t.frameOrigin(rel)
			cur.C[0] += float64(la.origin.X - o.X)
			cur.C[1] += float64(la.origin.Y - o.Y)
		}
		la.rel, la.target = rel, tgt
		t.startLayout(la, cur, vel, now)
	}
	la.placed = t.la.places
	la.memo = 0 // computed again with the new target
	t.layoutStyle(bx, true)
}

// layoutVisual returns the visual box of bx at this paint (Box.u
// coordinates) and whether it is drawn through a flip (it or its layout
// parent moves), computing it once per paint.
func (t *Tree) layoutVisual(bx *Box) (gfx.Rect, bool) {
	la := bx.lanim
	if la.memo == t.la.paints {
		return la.vis, la.active
	}
	la.memo = t.la.paints
	var origin gfx.Point
	parent := false
	if r := la.rel; r != nil && r.lanim != nil && r != bx {
		v, a := t.layoutVisual(r)
		origin, parent = v.Origin(), a
	}
	own := la.target
	if la.running {
		x, _, done := la.m.At(t.anim.tk.Now())
		if done || t.cx.ReducedMotion {
			la.running = false
		} else {
			own = vecRect(x)
		}
	}
	la.origin = origin
	la.vis = own.Translate(origin.X, origin.Y)
	la.active = la.running || parent
	return la.vis, la.active
}

// layoutStyle sets the paint-phase style of bx (with its deferred props
// resolved already) to draw it at its visual box of this paint: its
// transform is preceded by the flip, and its radii and shadows are
// corrected for the flip's scale. A box that does not move keeps its
// style.
//
// placing reports a call from place, which records the layers after it;
// on a frame that only repaints, a box that starts or stops being a layer
// of its own asks for placing and recording everything again.
func (t *Tree) layoutStyle(bx *Box, placing bool) {
	la := bx.lanim
	vis, active := t.layoutVisual(bx)
	prevScale := la.scale
	la.scale = gfx.Point{X: 1, Y: 1}
	defer0 := bx.deferPaint
	bx.deferPaint = hasPhase(bx.cs, style.PhasePaint) || active
	if bx.deferPaint != defer0 && !placing {
		t.lay.needFull, t.lay.needPlace = true, true
	}
	if !active {
		if !placing && prevScale != la.scale && scaleCorrected(bx.pcs) {
			t.lay.needFull = true
		}
		return
	}
	if !la.listed {
		la.listed = true
		t.la.active = append(t.la.active, bx)
	}
	n := bx.u
	// M maps the layout box n to the visual box; A is what the layout
	// parent's flips do to everything inside it (n_P to vis_P). The box's
	// own flip is A⁻¹ ∘ M, so that it ends up at vis.
	sm := gfx.Point{X: ratio(vis.Width(), n.Width()), Y: ratio(vis.Height(), n.Height())}
	s, base := sm, vis.Origin()
	if r := la.rel; r != nil && r.lanim != nil {
		rv, ra := t.layoutVisual(r) // this paint's, whatever the order of the boxes
		if !ra {
			rv = r.u
		}
		sa := gfx.Point{X: ratio(rv.Width(), r.u.Width()), Y: ratio(rv.Height(), r.u.Height())}
		if sa.X == 0 || sa.Y == 0 {
			sa = gfx.Point{X: 1, Y: 1}
		}
		s = gfx.Point{X: s.X / sa.X, Y: s.Y / sa.Y}
		// A⁻¹(vis.min) = r.u.min + (vis.min - rv.min) / sa
		base = gfx.Point{X: r.u.X0 + (vis.X0-rv.X0)/sa.X, Y: r.u.Y0 + (vis.Y0-rv.Y0)/sa.Y}
	}
	if !placing && sm != prevScale && scaleCorrected(bx.pcs) {
		// The corrected radii and shadows are in the box's layer, which
		// the compositor does not change: record the layers again.
		t.lay.needFull = true
	}
	la.scale = sm
	c := *bx.pcs
	c.Transform = foldFlip(c.Transform, n, s, base.Sub(n.Origin()))
	if len(c.BoxShadows) > 0 && sm != (gfx.Point{X: 1, Y: 1}) {
		c.BoxShadows = correctShadows(c.BoxShadows, sm)
	}
	bx.pcs = &c
}

// scaleCorrected reports whether a box with style cs paints something
// that a scale correction changes: radii with a length, or box shadows.
func scaleCorrected(cs *style.ComputedStyle) bool {
	r := &cs.Radius
	return len(cs.BoxShadows) > 0 || r.TopLeft.Px != 0 || r.TopRight.Px != 0 || r.BottomRight.Px != 0 || r.BottomLeft.Px != 0
}

// ratio returns a / b, or 1 when b is not positive (a box without width
// cannot be scaled to one).
func ratio(a, b float32) float32 {
	if !(b > 0) {
		return 1
	}
	return a / b
}

// foldFlip returns the transform tf of a box with border box box preceded
// by the flip p ↦ box.min + d + s·(p − box.min) (an axis-aligned scale s
// and translation d of its top-left corner), as one transform: the
// translation absorbs the flip's and the scale multiplies the box's
// (Motion's order: the flip's scale is applied with the box's own, inside
// its rotation, which is exact without rotation and skew).
func foldFlip(tf style.Transform, box gfx.Rect, s, d gfx.Point) style.Transform {
	w, h := box.Width(), box.Height()
	ox, oy := tf.OriginX.Resolve(w), tf.OriginY.Resolve(h)
	tx, ty := tf.TranslateX.Resolve(w), tf.TranslateY.Resolve(h)
	out := tf
	// T(p) = O + Tr' + L'(p − O) with Tr' = d + (s − 1)·o + s·tr, the
	// origin o relative to the box (explicit roundings: fusion-safe).
	out.TranslateX = style.PxLP(d.X + float32((s.X-1)*ox) + float32(s.X*tx))
	out.TranslateY = style.PxLP(d.Y + float32((s.Y-1)*oy) + float32(s.Y*ty))
	out.ScaleX = float32(tf.ScaleX * s.X)
	out.ScaleY = float32(tf.ScaleY * s.Y)
	return out
}

// correctShadows returns the shadows of a box drawn at scale s corrected
// so that they look unscaled (Motion's box-shadow scale correction):
// offsets divided per axis, blur and spread by the mean scale.
func correctShadows(sh []style.Shadow, s gfx.Point) []style.Shadow {
	out := make([]style.Shadow, len(sh))
	avg := (s.X + s.Y) / 2
	for i, x := range sh {
		if s.X > 0 {
			x.OffsetX /= s.X
		}
		if s.Y > 0 {
			x.OffsetY /= s.Y
		}
		if avg > 0 {
			x.Blur /= avg
			x.Spread /= avg
		}
		out[i] = x
	}
	return out
}

// layoutUpdate is update's part for a box with a layout animation on a
// frame that only repaints: the flip of this frame.
func (t *Tree) layoutUpdate(bx *Box) {
	la := bx.lanim
	la.memo = 0
	t.layoutStyle(bx, false)
}

// endLayoutPaint ends a paint: the boxes left by unmounted LayoutID
// elements were for this commit only.
func (t *Tree) endLayoutPaint() {
	if len(t.la.gone) > 0 {
		clear(t.la.gone)
	}
}

// AnimateLayout makes a primitive animate the changes of its laid-out box
// (tela.h.AnimateLayout).
func AnimateLayout() Prop {
	return Prop{name: "AnimateLayout", apply: func(d *desc) { d.animLayout = true }}
}

// LayoutID gives a primitive a shared layout identity (tela.h.LayoutID).
func LayoutID(id string) Prop {
	return Prop{name: "LayoutID", apply: func(d *desc) { d.layoutID = id }}
}
