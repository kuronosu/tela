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
//     its rounded corners. Gradient layers are sized, positioned and
//     tiled per background-size, -position, -repeat and -origin; url()
//     layers and background-clip: text are not painted yet (no image
//     source, no glyph clip). The root's background color is the canvas,
//     painted over the whole viewport first;
//  3. its inset box shadows, inside the padding box (the inner border
//     edge with the inner radii), last listed first;
//  4. its border;
//  5. its content, in its content box: a paragraph's text (broken and
//     aligned at the content width first) or an image (stretched to the
//     content box and clipped to the content box's rounded corners), and
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
// move with the root.
//
// # Stacking (ARCHITECTURE section 13, simplified CSS 2 appendix E)
//
// A box creates a stacking context when it has a z-index other than auto
// and is positioned or a flex item, is fixed or sticky, has opacity < 1, a
// transform, a filter, a backdrop filter, a mix-blend-mode or isolation:
// isolate. The root is a stacking context. z-index on a box that is
// neither positioned nor a flex item does nothing, as in CSS.
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
// everything it paints, outline included), its backdrop filter
// (BackdropFilter over its border box, before anything else of the box is
// painted), and one layer (PushLayer) with its opacity and filters. The
// backdrop is therefore not faded by the box's own opacity, as CSS would
// do; mix-blend-mode is painted as normal until the rasterizer has blend
// modes (phase 6).
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
// rasterizer, ADR-029). The scale is the Tree's (SetScale); a new scale
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
	part, content := false, false
	chunked := bx.scr != nil && bx.scr.chunked
	for _, k := range bx.kids {
		p.place(k, bx)
		if k.ls.Display == layout.DisplayNone {
			continue
		}
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
	bx.ink, bx.hasPart = ink, part
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
	off := gfx.Point{X: p.snap(s.pos.X), Y: p.snap(s.pos.Y)}
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
	for _, bx := range marked {
		if !bx.deferPaint || bx.pcs == nil || bx == t.rootBox || !t.inTree(bx) {
			continue
		}
		bx.pcs = t.resolveDeferred(bx, style.PhasePaint)
		if stacking(bx) != bx.layerSC {
			t.lay.needFull, t.lay.needPlace = true, true
			return
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
	return cs.Position == style.PositionFixed || cs.Position == style.PositionSticky ||
		cs.Opacity < 1 || !cs.Transform.IsIdentity() || !cs.Filter.IsEmpty() ||
		!cs.BackdropFilter.IsEmpty() || cs.MixBlendMode != gfx.SrcOver || cs.Isolate
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
		if k.ls.Display == layout.DisplayNone {
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
// positioned ancestor (or the root) for absolute boxes, the nearest
// transformed or filtered ancestor (or the root) for fixed boxes.
func containingBlock(bx *Box) *Box {
	switch bx.pcs.Position {
	case style.PositionAbsolute:
		a := bx.parent
		for a != nil && a.parent != nil && a.pcs.Position == style.PositionStatic {
			a = a.parent
		}
		return a
	case style.PositionFixed:
		a := bx.parent
		for a != nil && a.parent != nil && a.pcs.Transform.IsIdentity() && a.pcs.Filter.IsEmpty() {
			a = a.parent
		}
		return a
	}
	return bx.parent
}

// paintFlow paints an in-flow box that is not a participant, atomically,
// unless it is outside the interest rect of the list being recorded.
func (p *painter) paintFlow(bx *Box) {
	if bx.ls.Display == layout.DisplayNone || participant(bx) {
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
	if bx.pcs.Visibility == style.VisibilityVisible {
		p.paintContent(bx)
	}
	for _, k := range bx.kids {
		p.paintFlow(k)
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
	pushedLayer
)

// pushEffects opens the transform and the layer of a stacking context and
// applies its backdrop filter; popEffects closes them.
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
	if !cs.BackdropFilter.IsEmpty() && cs.Visibility == style.VisibilityVisible {
		if g := p.geom(bx); !g.border.Rect.Empty() {
			p.b.BackdropFilter(g.border, cs.BackdropFilter.List()...)
		}
	}
	if cs.MixBlendMode != gfx.SrcOver {
		p.t.reportOnce("mix-blend-mode", "tela: mix-blend-mode is painted as normal until phase 6")
	}
	if cs.Opacity < 1 || !cs.Filter.IsEmpty() {
		p.b.PushLayer(cs.Opacity, gfx.SrcOver, cs.Filter.List()...)
		pops |= pushedLayer
	}
	return pops
}

func (p *painter) popEffects(pops int) {
	if pops&pushedLayer != 0 {
		p.b.PopLayer()
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
		if l.ScrollbarSize.W > 0 {
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

// reportOnce logs msg the first time key is reported in this tree.
func (t *Tree) reportOnce(key, msg string) {
	if t.reported[key] {
		return
	}
	t.reported[key] = true
	t.logf("%s", msg)
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
	cs := bx.pcs
	var g boxGeom
	r := p.snapRect(p.rect(bx))
	w, h := r.Width(), r.Height()
	rd := &cs.Radius
	g.border = gfx.RRect{Rect: r, Radii: [4]gfx.Point{
		{X: rd.TopLeft.Resolve(w), Y: rd.TopLeft.Resolve(h)},
		{X: rd.TopRight.Resolve(w), Y: rd.TopRight.Resolve(h)},
		{X: rd.BottomRight.Resolve(w), Y: rd.BottomRight.Resolve(h)},
		{X: rd.BottomLeft.Resolve(w), Y: rd.BottomLeft.Resolve(h)},
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
	pd := bx.ld.Layout().Padding
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
		para.lay.Paint(p.b, c.Origin())
	case modeImage:
		if bx.img == nil {
			return
		}
		g := p.geom(bx)
		c := p.contentRRect(bx, &g)
		if c.Rect.Empty() {
			return
		}
		// A replaced element's content is clipped to the content box's
		// rounded corners (CSS Backgrounds 3 section 5.3).
		rounded := !c.IsRect()
		if rounded {
			p.b.PushClipRRect(c)
		}
		p.b.DrawImage(bx.img, gfx.Rect{}, c.Rect, gfx.SampleLinear)
		if rounded {
			p.b.PopClip()
		}
	}
}
