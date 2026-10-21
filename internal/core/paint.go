package core

import (
	"cmp"
	"math"
	"slices"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/layout"
	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/text"
)

// Paint phase (ARCHITECTURE sections 8 and 13): how boxes are painted.
// The paint phase records them into layers and composes the frame from
// them (layer.go, whose top comment describes layers, composition and
// damage); place (below) prepares the geometry first.
//
// # What a box paints
//
// Each box paints, in this order (decorate.go):
//
//  1. its outer box shadows, last listed first (CSS paints the first
//     shadow on top); DrawShadow clips them to the outside of the border
//     box, as CSS does;
//  2. its background: the color, then the image layers from the last to
//     the first, in the background painting area (background-clip) with
//     its rounded corners. Gradient and image layers (url() through the
//     registered assets, tela.BgImage; assets.go) are sized, positioned
//     and tiled per background-size, -position, -repeat and -origin, and
//     blended per background-blend-mode in a group of their own;
//     background-clip: text paints them in a layer masked by the glyphs
//     of the box's text (cssfx.go). The root's background color is the
//     canvas, painted over the whole viewport first;
//  3. its inset box shadows, inside the padding box (the inner border
//     edge with the inner radii), last listed first;
//  4. its border;
//  5. its content, in its content box: a paragraph's text (broken and
//     aligned at the content width first) or an image (placed by
//     object-fit and clipped to the content box's rounded corners;
//     media.go), and
//     its in-flow children, both clipped to the padding box with the
//     inner radii when overflow is not visible;
//  6. its outline, outside the border box.
//
// Boxes with visibility: hidden or collapse skip 1-6 for themselves but
// still paint their children, which may be visible.
//
// # Scroll containers
//
// The content and in-flow children of a scroll container (scroll.go) are
// painted under its overflow clip, which leaves out the classic scrollbar
// gutters, and moved by its scroll offset, snapped to device pixels; its
// scrollbars (scrollbar.go) are painted after everything it paints itself,
// before its outline. Every box is painted at its layout position without
// the scroll offsets (Box.u), in the scroll space of the nearest scroll
// container with a content layer on its containing-block chain
// (Box.space): the painter opens a chunk of that space where needed
// (layer.go: enter), and the compositor moves it by the offsets, so an
// absolutely positioned descendant whose containing block is outside a
// scroll container does not scroll with it (CSS), and fixed boxes never
// move with the root. A sticky box is painted in a scroll space of its
// own, moved by its sticky offset (sticky.go).
//
// # Stacking (ARCHITECTURE section 13, simplified CSS 2 appendix E)
//
// A box creates a stacking context when it has a z-index other than auto
// and is positioned or a flex item, is fixed, sticky or floating
// (floating.go), has opacity < 1, a transform, a filter, a backdrop
// filter, a mix-blend-mode, a clip-path, a mask or isolation: isolate, or runs a
// transform animation (anim.go) or a layout animation (layoutanim.go). The root is a stacking context. z-index on a
// box that is neither positioned nor a flex item does nothing, as in CSS.
//
// A stacking context paints its own decorations (1-4), then its
// descendants that are positioned or stacking contexts with a negative
// z-index (lowest first, tree order among equals), then its content and
// in-flow children (5), then those with z-index auto or 0 in tree order,
// then those with a positive z-index (lowest first), and finally its
// outline (6). Positioned boxes without a z-index paint like stacking
// contexts but their own positioned descendants belong to the enclosing
// stacking context. Such a hoisted box keeps the overflow clips of its
// ancestors that are on its containing-block chain (an absolute box
// escapes the clip of a static ancestor between it and its containing
// block, as in CSS).
//
// Simplifications: an in-flow box paints atomically (its background, its
// text and its children together, in tree order), while CSS paints the
// backgrounds of all in-flow descendants before any of their text; the
// two differ only when a later sibling overlaps earlier text. Flex items
// paint in box order, not in order-modified document order.
//
// # Effects
//
// Around a stacking context, in this order: its transform
// (PushTransform with Transform.Matrix of its unsnapped border box, around
// everything it paints, outline included), its clip path (cssfx.go), its
// backdrop filter (BackdropFilter over its border box, before anything
// else of the box is painted), and one layer (PushLayer) with its opacity,
// filters and blend mode (a mask layer, PushMaskLayer, when it has a CSS
// mask: cssfx.go), also pushed, with nothing else, by a stacking
// context whose descendants blend, to isolate them (cssfx.go). Since the
// layer is a backdrop root, the backdrop filter is drawn before it, faded
// by the box's opacity on its own (and not drawn at opacity 0): the
// filtered backdrop at opacity a, then the box's content in a layer at
// opacity a, which is how Chromium composites them (ADR-084). The box's
// filters and blend mode do not apply to its backdrop.
//
// # Pixel snapping
//
// Like Chromium, the border box of every box is snapped to device pixels
// before its decorations are painted: each edge, in absolute logical
// coordinates without the scroll offsets (the coordinates of the enclosing
// transform's space under a transform; the offsets are whole device
// pixels, so snapping commutes with them), becomes round(edge × scale) /
// scale, rounding halves up, so
// a box keeps whole device pixels and adjacent boxes stay adjacent.
// Border and outline widths are snapped like CSS line widths: a width of
// at least one device pixel is floored to whole device pixels, a smaller
// non-zero one becomes one device pixel. Radii are not snapped. Text and
// images are positioned from the unsnapped layout (text is snapped by the
// rasterizer, ADR-029). Text selection highlights and scrollbar tracks and
// thumbs are snapped like border boxes, so that at fractional scales the
// highlights of consecutive lines meet without a seam and the bars stay
// sharp (ADR-075). The scale is the Tree's (SetScale); a new scale
// repaints.

// painter is the state of one paint pass.
type painter struct {
	t     *Tree
	b     *gfx.Builder
	scale float64
	// items is a stack of the stacking-context participants being
	// painted; each stacking context uses the part it appended.
	items []paintItem
	clips []*Box // scratch for hoisted boxes' ancestor clips
	// full: place prepares a full recording (layer.go).
	full bool
	// inMask: background layers are painted as mask layers (cssfx.go).
	inMask bool
	// The list being recorded (layer.go): the scroll space it is painted
	// in, its interest rect, whether boxes outside the interest rect are
	// skipped, its layer, and the lists of the enclosing layers.
	space    *scroller
	interest gfx.Rect
	cull     bool
	layer    *layer
	targets  []target
	acts     []uint8 // scratch of paintHoisted
}

// paintItem is a box painted by its stacking context out of tree order.
type paintItem struct {
	bx *Box
	z  int32
}

// place records, for bx and its subtree, the paint-phase style, the
// unsnapped border box where it is painted (u: its layout position,
// without the scroll offsets) and the scroll space it is painted in (the
// nearest scroll container with a content layer on its containing-block
// chain, layer.go), and its ink and hit bounds. parent is bx's layout
// parent, placed already (nil for the root).
func (p *painter) place(bx *Box, parent *Box) {
	if bx.ls.Display == layout.DisplayNone {
		return
	}
	t := p.t
	l := bx.ld.Layout()
	bx.pcs = bx.cs
	if bx.deferPaint {
		// The paint-phase deferred props, evaluated on every paint of
		// the box; the Values they read repaint it when they change.
		bx.pcs = t.resolveDeferred(bx, style.PhasePaint)
	}
	loc := l.Location
	var space *scroller
	if parent != nil {
		// The layout position; the scroll space of the containing block
		// (its own content's, when it is a scroll container with a
		// content layer): an in-flow box moves with its parent's content,
		// an absolute one with its containing block's, a fixed one never
		// with the root's.
		loc = loc.Add(parent.u.Origin())
		if cb := containingBlock(bx); cb != nil && (bx.pcs.Position != style.PositionFixed || cb != t.rootBox) {
			space = contentSpace(cb)
		}
	}
	bx.space = space
	bx.u = gfx.RectFromOriginSize(loc, l.Size)
	if parent == nil {
		clear(t.stickies)
		t.stickies = t.stickies[:0]
		clear(t.flt.boxes)
		t.flt.boxes = t.flt.boxes[:0]
	} else if bx.flt != nil {
		// A floating element is painted in a scroll space of its own,
		// moved to its place by the float pass (floating.go).
		bx.space = floatSpace(bx, space)
		t.flt.boxes = append(t.flt.boxes, bx)
	} else if isSticky(bx) {
		// Painted in a scroll space of its own, moved by its sticky
		// offset (sticky.go).
		bx.space = stickySpace(bx, space)
		p.placeSticky(bx)
		t.stickies = append(t.stickies, bx)
	}
	if bx.lanim != nil || t.wantsLayoutAnim(bx) {
		// Layout animations (layoutanim.go): the flip of this frame.
		p.layoutPlace(bx)
	}
	if s := bx.scr; s != nil {
		p.offset(s)
		p.placeScroller(bx, s)
	}
	if bx.deferPaint {
		bx.layerSC = bx != t.rootBox && stacking(bx)
	}
	// The hit bounds are in the coordinates of bx's scroll space: the
	// children in it count as they are, the content of bx's content layer
	// as bx's scrollport (it clips them), and children in other spaces
	// (boxes that escape a scroll container) make them unbounded.
	sub := bx.u
	ink := p.selfInk(bx)
	if bx.mode == modeParagraph && bx.para != nil && len(bx.para.inl) > 0 && (bx.scr == nil || !bx.scr.chunked) {
		// The fragments of inline elements are hit where they are
		// painted, outside the box too (vertical padding).
		sub = sub.Union(bx.para.fragmentsBox(contentRect(bx.u, l).Origin()))
	}
	part, content, blend := false, false, false
	chunked := bx.scr != nil && bx.scr.chunked
	for _, k := range bx.kids {
		p.place(k, bx)
		if k.ls.Display == layout.DisplayNone {
			continue
		}
		blend = blend || blendsIn(k)
		switch {
		case k.space == bx.space:
			sub = sub.Union(k.hitBounds)
		case chunked && k.space == bx.scr && k.hitBounds != unbounded:
			content = true
		default:
			sub = unbounded
		}
		pk := participant(k)
		part = part || pk || k.hasPart
		switch {
		case pk:
		case chunked:
			content = true
		default:
			ink = ink.Union(k.ink)
		}
	}
	if content {
		pad := paddingRect(bx.u, l)
		ink = ink.Union(pad)
		if sub != unbounded {
			sub = sub.Union(pad)
		}
	}
	bx.ink, bx.hasPart, bx.blendDesc = ink, part, blend
	if sub != unbounded {
		// Grown by one pixel for the snapping of the painted edges.
		sub = sub.Outset(1)
	}
	bx.hitSub = sub
	bx.hitBounds = hitBounds(bx)
}

// unbounded is the hit bounds of a subtree that cannot be culled.
var unbounded = gfx.Rect{X0: -1e30, Y0: -1e30, X1: 1e30, Y1: 1e30}

// hitBounds returns the hit bounds of bx from those of its subtree:
// transformed by its transform.
func hitBounds(bx *Box) gfx.Rect {
	if bx.hitSub == unbounded || bx == bx.tree.rootBox || bx.pcs.Transform.IsIdentity() {
		return bx.hitSub
	}
	return bx.pcs.Transform.Matrix(bx.u).TransformRect(bx.hitSub)
}

// offset snaps the painted offset of s to device pixels, like box edges,
// and decides whether its content moves (a content layer).
func (p *painter) offset(s *scroller) {
	pos := s.flip(s.pos) // negative x in a right-to-left container
	off := gfx.Point{X: p.snap(pos.X), Y: p.snap(pos.Y)}
	if off != s.soff {
		s.soff = off
		p.t.scr.moved = true
	}
	s.chunked = s.max.X > 0 || s.max.Y > 0 || off != (gfx.Point{})
}

// update places again, on a frame that only repaints, what can change
// without a build, a restyle or a layout: the boxes in marked, the scroll
// containers whose offsets or bars changed (setScroll and the bars mark
// them) and the boxes with paint-phase deferred props (whose hit bounds
// grow their ancestors'). It sets lay.needFull when the layers
// must be recorded again (a scroll container whose visible content left
// its interest rect), and lay.needPlace too when the boxes must be placed
// again first (a scroll container whose content starts or stops moving, a
// box that starts or stops being a layer of its own: scroll spaces and
// stacking change).
func (p *painter) update(marked []*Box) {
	t := p.t
	for _, bx := range marked {
		if s := bx.scr; s != nil && bx.pcs != nil {
			p.offset(s)
			p.placeScroller(bx, s)
		}
	}
	if len(t.stickies) > 0 {
		p.updateSticky()
	}
	for _, bx := range marked {
		if (bx.inPara != nil || contentsBox(bx)) && bx.el != nil && bx.el.mounted && bx.cs != nil && hasPhase(bx.cs, style.PhasePaint) {
			// An inline element's paint props (BgFn, TextColorFn), and a
			// display: contents element's TextColorFn, are read when the
			// paragraphs are recorded (inline.go, colorfn.go).
			t.lay.needFull = true
			continue
		}
		if !bx.deferPaint || bx.pcs == nil || bx == t.rootBox || !t.inTree(bx) {
			continue
		}
		old := bx.pcs
		bx.pcs = t.resolveDeferred(bx, style.PhasePaint)
		if bx.lanim != nil {
			t.layoutUpdate(bx)
		}
		if stacking(bx) != bx.layerSC {
			t.lay.needFull, t.lay.needPlace = true, true
			return
		}
		if old.BackgroundColor != bx.pcs.BackgroundColor || old.Text.Color != bx.pcs.Text.Color {
			// BgFn, TextColorFn: what the box records changes (not its
			// bounds): record the layers again without placing.
			t.lay.needFull = true
		}
		// The new hit bounds, and those of the ancestors in the same
		// space (they only grow: bounds that are too large cull less).
		hb := hitBounds(bx)
		bx.hitBounds = hb
		for c, a := bx, bx.parent; a != nil && hb != unbounded; c, a = a, a.parent {
			if c.space != a.space || a.hitSub.ContainsRect(hb) {
				break // a scrollport (unchanged) or nothing new
			}
			a.hitSub = a.hitSub.Union(hb)
			hb = hitBounds(a)
			a.hitBounds = hb
		}
	}
}

// paddingRect returns the padding box of a box whose border box is r.
func paddingRect(r gfx.Rect, l *layout.Layout) gfx.Rect {
	return r.InsetEdges(l.Border.Left, l.Border.Top, l.Border.Right, l.Border.Bottom)
}

// selfInk returns a rectangle (u coordinates) that contains what bx
// paints itself: its border box, outer shadows, outline and text.
func (p *painter) selfInk(bx *Box) gfx.Rect {
	// The result of the previous place, when nothing it depends on
	// changed (most boxes, most frames).
	key := inkKey{u: bx.u, cs: bx.pcs, chunked: bx.scr != nil && bx.scr.chunked}
	if bx.mode == modeParagraph && bx.para != nil {
		key.lay = bx.para.lay
		key.content = contentRect(bx.u, bx.ld.Layout())
	}
	if key == bx.inkKey {
		return bx.selfInkRect
	}
	r := p.computeSelfInk(bx)
	bx.inkKey, bx.selfInkRect = key, r
	return r
}

// inkKey is what selfInk depends on.
type inkKey struct {
	u, content gfx.Rect
	cs         *style.ComputedStyle
	lay        *text.Layout
	chunked    bool
}

func (p *painter) computeSelfInk(bx *Box) gfx.Rect {
	cs := bx.pcs
	r := bx.u.Outset(1)
	if cs.Visibility != style.VisibilityVisible {
		return r
	}
	for i := range cs.BoxShadows {
		if sh := &cs.BoxShadows[i]; !sh.Inset {
			r = r.Union(bx.u.Translate(sh.OffsetX, sh.OffsetY).Outset(max(sh.Spread, 0) + 1.5*max(sh.Blur, 0) + 2))
		}
	}
	if o := &cs.Outline; o.Width > 0 {
		r = r.Union(bx.u.Outset(max(o.Offset, 0) + o.Width + 2))
	}
	if bx.mode == modeParagraph && bx.para != nil && bx.para.lay != nil {
		l := bx.ld.Layout()
		if s := bx.scr; s != nil && s.chunked {
			// The text is in the box's content layer, behind its clip.
			return r.Union(paddingRect(bx.u, l))
		}
		c := contentRect(bx.u, l)
		bx.para.prepare(p.t, max(c.Width(), 0))
		r = r.Union(bx.para.inkRect(c.Origin()))
		if len(bx.para.inl) > 0 {
			r = r.Union(bx.para.decorInk(c.Origin()))
		}
	}
	return r
}

// ---------------------------------------------------------------------
// Stacking.

// flexItem reports whether bx is laid out by a flex (or grid) container,
// where z-index applies without positioning.
func flexItem(bx *Box) bool {
	if bx.parent == nil || bx.parent.pcs == nil {
		return false
	}
	switch bx.parent.pcs.Display {
	case style.DisplayFlex, style.DisplayInlineFlex, style.DisplayGrid, style.DisplayInlineGrid:
		return true
	}
	return false
}

// zIndex returns the z-index that orders bx in its stacking context: its
// z-index where it applies, else 0.
func zIndex(bx *Box) int32 {
	cs := bx.pcs
	if cs.ZIndex.Explicit && (cs.Position != style.PositionStatic || flexItem(bx)) {
		return cs.ZIndex.Value
	}
	return 0
}

// stacking reports whether bx creates a stacking context.
func stacking(bx *Box) bool {
	cs := bx.pcs
	if cs.ZIndex.Explicit && (cs.Position != style.PositionStatic || flexItem(bx)) {
		return true
	}
	return cs.Position == style.PositionFixed || cs.Position == style.PositionSticky || bx.flt != nil ||
		(bx.anim != nil && bx.anim.layer) || layoutActive(bx) || cs.Opacity < 1 || !cs.Transform.IsIdentity() || !cs.Filter.IsEmpty() ||
		!cs.BackdropFilter.IsEmpty() || cs.MixBlendMode != gfx.SrcOver || cs.Isolate || !cs.ClipPath.IsNone() ||
		!cs.Mask.IsNone()
}

// participant reports whether bx is painted by its stacking context out of
// tree order: it is positioned or a stacking context.
func participant(bx *Box) bool {
	return bx.pcs.Position != style.PositionStatic || stacking(bx)
}

// collect appends to p.items the participants of the stacking context
// whose descendants are under bx, in tree order: it does not look inside
// stacking contexts, but it does inside positioned boxes without one, and
// skips subtrees without participants (hasPart).
func (p *painter) collect(bx *Box) {
	for _, k := range bx.kids {
		if k.ls.Display == layout.DisplayNone || k.hiddenAtom {
			continue
		}
		if participant(k) {
			p.items = append(p.items, paintItem{k, zIndex(k)})
			if stacking(k) {
				continue
			}
		}
		if k.hasPart {
			p.collect(k)
		}
	}
}

// paintStacked paints the stacking context bx (or the root) and every box
// of its stacking context. A stacking context with paint-phase deferred
// props is a layer (layer.go): its effects are pushed by the compositor,
// so that a change of them only composites again.
func (p *painter) paintStacked(bx *Box) {
	cull := p.cull
	deferred := bx.layerSC && bx.deferPaint && bx != p.t.rootBox
	pops := 0
	if deferred {
		// The layer moves without being recorded again: everything in
		// it is recorded.
		p.openLayer(layerDeferred, bx, p.space)
		p.cull = false
	} else {
		pops = p.pushEffects(bx)
		if cs := bx.pcs; bx != p.t.rootBox && (!cs.Transform.IsIdentity() || !cs.Filter.IsEmpty() || !cs.BackdropFilter.IsEmpty()) {
			// The interest rects are untransformed, and filters pull in
			// what is around: no culling inside.
			p.cull = false
		}
	}
	p.paintSelf(bx)
	start := len(p.items)
	p.collect(bx)
	items := p.items[start:]
	slices.SortStableFunc(items, func(a, b paintItem) int { return cmp.Compare(a.z, b.z) })
	i := 0
	for ; i < len(items) && items[i].z < 0; i++ {
		p.paintHoisted(bx, items[i].bx)
	}
	p.paintInner(bx)
	for ; i < len(items); i++ {
		p.paintHoisted(bx, items[i].bx)
	}
	p.items = p.items[:start]
	p.paintScrollbars(bx)
	p.paintOutline(bx)
	if deferred {
		p.closeLayer()
	} else {
		p.popEffects(pops)
	}
	p.cull = cull
}

// paintHoisted paints a participant e of the stacking context root, under
// the overflow clips of its ancestors on its containing-block chain. Each
// clip is pushed in the scroll space of its box, and e is painted in its
// own (layer.go: enter).
func (p *painter) paintHoisted(root, e *Box) {
	if p.cull && !stacking(e) && !e.ink.Intersects(p.interestOf(e.space)) {
		return // outside the recorded part of its layer
	}
	start := len(p.clips)
	cb := containingBlock(e)
	for a := e.parent; a != nil; a = a.parent {
		if a == cb {
			if clipsOverflow(a.pcs) {
				p.clips = append(p.clips, a)
			}
			cb = containingBlock(a)
		}
		if a == root {
			break
		}
	}
	clips := p.clips[start:]
	astart := len(p.acts)
	for i := len(clips) - 1; i >= 0; i-- {
		var act uint8
		if p.enter(clips[i].space, clips[i]) {
			act |= 1
		}
		if p.pushOverflowClip(clips[i]) {
			act |= 2
		}
		p.acts = append(p.acts, act)
	}
	entered := p.enter(e.space, e)
	if stacking(e) {
		p.paintStacked(e)
	} else {
		p.paintSelf(e)
		p.paintInner(e)
		p.paintScrollbars(e)
		p.paintOutline(e)
	}
	if entered {
		p.leave()
	}
	for k := len(p.acts) - 1; k >= astart; k-- {
		if p.acts[k]&2 != 0 {
			p.b.PopClip()
		}
		if p.acts[k]&1 != 0 {
			p.leave()
		}
	}
	p.acts = p.acts[:astart]
	p.clips = p.clips[:start]
}

// containingBlock returns the box whose padding box contains bx's layout
// (CSS containing block): the parent for in-flow boxes, the nearest
// positioned ancestor or ancestor that contains positioned descendants
// (containsPositioned), or the root, for absolute boxes, the nearest
// ancestor that contains positioned descendants (or the root) for fixed
// boxes. It agrees with layout (layout.Style.ContainingBlock and Fixed).
func containingBlock(bx *Box) *Box {
	switch bx.pcs.Position {
	case style.PositionAbsolute:
		a := bx.parent
		for a != nil && a.parent != nil && a.pcs.Position == style.PositionStatic && !containsPositioned(a) {
			a = a.parent
		}
		return a
	case style.PositionFixed:
		a := bx.parent
		for a != nil && a.parent != nil && !containsPositioned(a) {
			a = a.parent
		}
		return a
	}
	return bx.parent
}

// containsPositioned reports whether bx is the containing block of its
// absolutely positioned and fixed descendants even when it is static:
// like Chromium, a transform, a filter or a backdrop filter makes it one.
// What decides is stable between restyles, so that layout (which reads it
// when the layout style is mapped) and paint agree: the target style of
// the classes and props (not the value of a running transition, which
// would flip it while it passes through the identity, without a relayout)
// and, like Chromium's running transform animations, a keyframe animation
// of the transform (animate-spin) while it is attached. A paint-phase
// TranslateFn alone does not count, nor does a transition back to no
// transform (Chromium counts the transform while it runs).
func containsPositioned(bx *Box) bool {
	cs := bx.cs
	if ba := bx.anim; ba != nil {
		if ba.target != nil {
			cs = ba.target
		}
		if k := ba.kf; k != nil && k.kfs != nil && k.kfs.AnimatesTransform() {
			return true
		}
	}
	return !cs.Transform.IsIdentity() || !cs.Filter.IsEmpty() || !cs.BackdropFilter.IsEmpty()
}

// paintFlow paints an in-flow box that is not a participant, atomically,
// unless it is outside the interest rect of the list being recorded.
func (p *painter) paintFlow(bx *Box) {
	if bx.ls.Display == layout.DisplayNone || participant(bx) || bx.hiddenAtom {
		return
	}
	if p.cull && !bx.ink.Intersects(p.interest) {
		return
	}
	p.paintSelf(bx)
	p.paintInner(bx)
	p.paintScrollbars(bx)
	p.paintOutline(bx)
}

// paintInner paints the content and the in-flow children of bx, clipped
// by its overflow clip. The content of a scroll container that can move
// is its content layer (layer.go): painted unscrolled in a list of its
// own, which the compositor moves by the scroll offset.
func (p *painter) paintInner(bx *Box) {
	clipped := clipsOverflow(bx.pcs) && p.pushOverflowClip(bx)
	entered := false
	if s := bx.scr; s != nil && s.chunked {
		entered = p.enter(s, bx)
	}
	if bx.mode == modeParagraph && bx.para != nil && bx.para.lay != nil && bx.para.decorated() {
		// Line by line, with the atomic inlines (its children) and the
		// decorations of its inline elements (inline.go).
		c := contentRect(p.rect(bx), bx.ld.Layout())
		bx.para.prepare(p.t, max(c.Width(), 0))
		p.paintParagraph(bx, c.Origin(), bx.pcs.Visibility == style.VisibilityVisible)
	} else {
		if bx.pcs.Visibility == style.VisibilityVisible {
			p.paintContent(bx)
		}
		for _, k := range bx.kids {
			p.paintFlow(k)
		}
	}
	if entered {
		p.leave()
	}
	if clipped {
		p.b.PopClip()
	}
}

// ---------------------------------------------------------------------
// Effects and clips.

// effect kinds pushed by pushEffects.
const (
	pushedTransform = 1 << iota
	pushedClip
	pushedLayer
)

// pushEffects opens the transform, the clip path and the layer of a
// stacking context and applies its backdrop filter; popEffects closes
// them. The layer carries the opacity, the filters and the blend mode; a
// stacking context with blending descendants gets one to isolate them
// (cssfx.go).
func (p *painter) pushEffects(bx *Box) int {
	if bx == p.t.rootBox {
		return 0
	}
	cs := bx.pcs
	pops := 0
	if !cs.Transform.IsIdentity() {
		p.b.PushTransform(cs.Transform.Matrix(p.rect(bx)))
		pops |= pushedTransform
	}
	if p.pushClipPath(bx) {
		pops |= pushedClip
	}
	if !cs.BackdropFilter.IsEmpty() && cs.Visibility == style.VisibilityVisible && cs.Opacity > 0 {
		// The filtered backdrop is part of the box: its opacity fades it
		// (ADR-084). It is drawn outside the layer, which would be its
		// backdrop root, so it is faded by itself, as Chromium does.
		if g := p.geom(bx); !g.border.Rect.Empty() {
			p.b.BackdropFilterOpacity(g.border, cs.Opacity, cs.BackdropFilter.List()...)
		}
	}
	if !cs.Mask.IsNone() {
		// The mask multiplies the group after its filters, before its
		// opacity and blending (CSS: filter, clip, mask, opacity).
		if m, ok := p.cssMask(bx); ok {
			p.b.PushMaskLayer(gfx.Mask{List: m, Mode: gfx.MaskAlpha}, cs.Opacity, cs.MixBlendMode, cs.Filter.List()...)
			return pops | pushedLayer
		}
	}
	if cs.Opacity < 1 || !cs.Filter.IsEmpty() || cs.MixBlendMode != gfx.SrcOver || bx.blendDesc {
		p.b.PushLayer(cs.Opacity, cs.MixBlendMode, cs.Filter.List()...)
		pops |= pushedLayer
	}
	return pops
}

func (p *painter) popEffects(pops int) {
	if pops&pushedLayer != 0 {
		p.b.PopLayer()
	}
	if pops&pushedClip != 0 {
		p.b.PopClip()
	}
	if pops&pushedTransform != 0 {
		p.b.PopTransform()
	}
}

// clipsOverflow reports whether a box with style cs clips its content.
func clipsOverflow(cs *style.ComputedStyle) bool {
	return cs.OverflowX.Clips() || cs.OverflowY.Clips()
}

// pushOverflowClip pushes the overflow clip of bx (overflowClip). It
// reports whether it pushed one: the root's (the viewport's) is not pushed
// when it covers the whole window.
func (p *painter) pushOverflowClip(bx *Box) bool {
	c := p.overflowClip(bx)
	if bx == p.t.rootBox && c.IsRect() && c.Rect.ContainsRect(gfx.Rect{X1: p.t.viewport.W, Y1: p.t.viewport.H}) {
		// The viewport's clip is the window: nothing to cut.
		return false
	}
	if c.IsRect() {
		p.b.PushClipRect(c.Rect)
	} else {
		p.b.PushClipRRect(c)
	}
	return true
}

// overflowClip returns the overflow clip of a box that clips: its padding
// box with the inner radii when both axes clip, a rectangle unbounded
// along the axis that does not otherwise. Hit-testing uses the same shape.
func (p *painter) overflowClip(bx *Box) gfx.RRect {
	cs := bx.pcs
	x, y := cs.OverflowX, cs.OverflowY
	// CSS: visible becomes auto, and clip hidden, when the other axis is
	// hidden, scroll or auto; only clip and visible combine per axis.
	scrolls := func(o style.Overflow) bool {
		return o == style.OverflowHidden || o == style.OverflowScroll || o == style.OverflowAuto
	}
	both := scrolls(x) || scrolls(y) || (x == style.OverflowClip && y == style.OverflowClip)
	g := p.geom(bx)
	pad := g.padding
	if l := bx.ld.Layout(); l.ScrollbarSize.W > 0 || l.ScrollbarSize.H > 0 {
		// The classic scrollbar gutters are not part of the scrollport:
		// the content is clipped to the rounded padding box and the
		// scrollport rectangle, whose corners at a gutter are square.
		r, rd := pad.Rect, pad.Radii
		switch {
		case l.ScrollbarSize.W > 0 && bx.scr != nil && bx.scr.rtl:
			r.X0 = min(r.X0+p.snapWidth(l.ScrollbarSize.W), r.X1)
			rd[0], rd[3] = gfx.Point{}, gfx.Point{}
		case l.ScrollbarSize.W > 0:
			r.X1 = max(r.X1-p.snapWidth(l.ScrollbarSize.W), r.X0)
			rd[1], rd[2] = gfx.Point{}, gfx.Point{}
		}
		if l.ScrollbarSize.H > 0 {
			r.Y1 = max(r.Y1-p.snapWidth(l.ScrollbarSize.H), r.Y0)
			rd[2], rd[3] = gfx.Point{}, gfx.Point{}
		}
		pad = gfx.RRect{Rect: r, Radii: rd}.Normalize()
	}
	if both {
		return pad
	}
	const far = 1 << 24
	r := pad.Rect
	if x != style.OverflowClip {
		r.X0, r.X1 = -far, far
	}
	if y != style.OverflowClip {
		r.Y0, r.Y1 = -far, far
	}
	return gfx.RRect{Rect: r}
}

// ---------------------------------------------------------------------
// Geometry.

// boxGeom is the snapped geometry of a box.
type boxGeom struct {
	// border is the snapped border box with its radii (clamped as in CSS
	// Backgrounds 3 section 5.5); padding is the inner border edge, with
	// the inner radii.
	border, padding gfx.RRect
	// bw are the snapped border widths: top, right, bottom, left.
	bw [4]float32
}

// snap rounds a logical coordinate to the device pixel grid.
func (p *painter) snap(v float32) float32 {
	d := math.Floor(float64(float64(v)*p.scale) + 0.5)
	return float32(d / p.scale)
}

// snapRect snaps the edges of r.
func (p *painter) snapRect(r gfx.Rect) gfx.Rect {
	return gfx.Rect{X0: p.snap(r.X0), Y0: p.snap(r.Y0), X1: p.snap(r.X1), Y1: p.snap(r.Y1)}
}

// snapWidth snaps a line width (border, outline) like CSS: at least one
// device pixel is floored to whole device pixels, less becomes one.
func (p *painter) snapWidth(w float32) float32 {
	if !(w > 0) {
		return 0
	}
	d := float64(float64(w) * p.scale)
	if d < 1 {
		return float32(1 / p.scale)
	}
	return float32(math.Floor(d) / p.scale)
}

// rect returns the unsnapped border box of bx where it is painted (u).
func (p *painter) rect(bx *Box) gfx.Rect { return bx.u }

// geom returns the snapped geometry of bx.
func (p *painter) geom(bx *Box) boxGeom {
	if la := bx.lanim; la != nil && la.active && la.scale != (gfx.Point{X: 1, Y: 1}) {
		// Scaled by a layout animation: the radii are corrected so that
		// they look as at rest (layoutanim.go).
		return p.geomScaled(bx.pcs, p.rect(bx), la.scale)
	}
	return p.geomOf(bx.pcs, p.rect(bx))
}

// geomOf returns the snapped geometry of a box with style cs whose
// unsnapped border box is r.
func (p *painter) geomOf(cs *style.ComputedStyle, r gfx.Rect) boxGeom {
	return p.geomScaled(cs, r, gfx.Point{X: 1, Y: 1})
}

// geomScaled is geomOf for a box drawn at scale k: the lengths of its
// radii are divided by k, per axis (percentages scale with the box).
func (p *painter) geomScaled(cs *style.ComputedStyle, r gfx.Rect, k gfx.Point) boxGeom {
	var g boxGeom
	r = p.snapRect(r)
	w, h := r.Width(), r.Height()
	rd := &cs.Radius
	res := func(l style.LP, basis, k float32) float32 {
		if k == 1 || !(k > 0) {
			return l.Resolve(basis)
		}
		return style.LP{Px: l.Px / k, Pct: l.Pct}.Resolve(basis)
	}
	g.border = gfx.RRect{Rect: r, Radii: [4]gfx.Point{
		{X: res(rd.TopLeft, w, k.X), Y: res(rd.TopLeft, h, k.Y)},
		{X: res(rd.TopRight, w, k.X), Y: res(rd.TopRight, h, k.Y)},
		{X: res(rd.BottomRight, w, k.X), Y: res(rd.BottomRight, h, k.Y)},
		{X: res(rd.BottomLeft, w, k.X), Y: res(rd.BottomLeft, h, k.Y)},
	}}.Normalize()
	g.bw = [4]float32{
		p.snapWidth(cs.Border.Top.Width), p.snapWidth(cs.Border.Right.Width),
		p.snapWidth(cs.Border.Bottom.Width), p.snapWidth(cs.Border.Left.Width),
	}
	g.padding = insetRRect(g.border, g.bw[0], g.bw[1], g.bw[2], g.bw[3])
	return g
}

// contentRRect returns the content box of bx (the padding box inset by the
// padding, snapped) with its radii.
func (p *painter) contentRRect(bx *Box, g *boxGeom) gfx.RRect {
	return p.contentRRectOf(g, bx.ld.Layout().Padding)
}

// contentRRectOf returns the content box of geometry g for padding widths
// pd.
func (p *painter) contentRRectOf(g *boxGeom, pd layout.Insets) gfx.RRect {
	r := g.padding.Rect
	c := p.snapRect(gfx.Rect{X0: r.X0 + pd.Left, Y0: r.Y0 + pd.Top, X1: r.X1 - pd.Right, Y1: r.Y1 - pd.Bottom})
	return insetRRect(g.padding, c.Y0-r.Y0, r.X1-c.X1, r.Y1-c.Y1, c.X0-r.X0)
}

// insetRRect returns rr shrunk by the given distance on each side, with
// each radius reduced by the widths of the sides it touches (CSS inner
// border radii). An inset larger than the rectangle collapses it.
func insetRRect(rr gfx.RRect, top, right, bottom, left float32) gfx.RRect {
	r := rr.Rect
	out := gfx.RRect{Rect: gfx.Rect{X0: r.X0 + left, Y0: r.Y0 + top, X1: r.X1 - right, Y1: r.Y1 - bottom}}
	if out.Rect.X1 < out.Rect.X0 {
		out.Rect.X1 = out.Rect.X0
	}
	if out.Rect.Y1 < out.Rect.Y0 {
		out.Rect.Y1 = out.Rect.Y0
	}
	rd := rr.Radii
	out.Radii = [4]gfx.Point{
		{X: max(rd[0].X-left, 0), Y: max(rd[0].Y-top, 0)},
		{X: max(rd[1].X-right, 0), Y: max(rd[1].Y-top, 0)},
		{X: max(rd[2].X-right, 0), Y: max(rd[2].Y-bottom, 0)},
		{X: max(rd[3].X-left, 0), Y: max(rd[3].Y-bottom, 0)},
	}
	return out.Normalize()
}

// ---------------------------------------------------------------------
// Content.

// paintContent paints the content of a leaf box in its content box.
func (p *painter) paintContent(bx *Box) {
	l := bx.ld.Layout()
	switch bx.mode {
	case modeParagraph:
		para := bx.para
		if para == nil || para.lay == nil {
			return
		}
		// A paragraph that scrolls its own text paints it unscrolled in
		// its content layer (paintInner).
		c := contentRect(p.rect(bx), l)
		para.prepare(p.t, max(c.Width(), 0))
		para.lay.PaintRecolored(p.b, c.Origin(), float32(p.scale), p.textPaints(bx))
	case modeField:
		if bx.fld != nil {
			p.paintField(bx)
		}
	case modeImage:
		p.paintReplaced(bx) // media.go
	}
}
