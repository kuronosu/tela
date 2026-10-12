package core

import (
	"cmp"
	"math"
	"slices"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/layout"
	"github.com/kuronosu/tela/internal/style"
)

// Paint phase (ARCHITECTURE sections 8 and 13). One display list holds the
// whole tree for now; layers and repaint boundaries arrive in phase 4.
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
// coordinates (the coordinates of the enclosing transform's space under a
// transform), becomes round(edge × scale) / scale, rounding halves up, so
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
}

// paintItem is a box painted by its stacking context out of tree order.
type paintItem struct {
	bx *Box
	z  int32
}

// paintPhase paints the tree into a new display list.
func (t *Tree) paintPhase() *gfx.DisplayList {
	t.stats.Paints++
	b := t.builder
	if t.rootBox != nil && t.rootBox.cs != nil {
		b.FillRect(gfx.Rect{X1: t.viewport.W, Y1: t.viewport.H}, t.CanvasColor())
		p := painter{t: t, b: b, scale: float64(t.scale)}
		p.place(t.rootBox, gfx.Point{})
		p.paintStacked(t.rootBox)
	}
	return b.Finish()
}

// place records, for bx and its subtree, the paint-phase style and the
// absolute (unsnapped) border box. origin is the absolute position of the
// border box of bx's layout parent.
func (p *painter) place(bx *Box, origin gfx.Point) {
	if bx.ls.Display == layout.DisplayNone {
		return
	}
	l := bx.ld.Layout()
	bx.abs = gfx.RectFromOriginSize(origin.Add(l.Location), l.Size)
	bx.pcs = bx.cs
	if bx.deferred {
		bx.pcs = bx.cs.Resolved(style.PhasePaint)
	}
	o := bx.abs.Origin()
	for _, k := range bx.kids {
		p.place(k, o)
	}
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
// stacking contexts, but it does inside positioned boxes without one.
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
		p.collect(k)
	}
}

// paintStacked paints the stacking context bx (or the root) and every box
// of its stacking context.
func (p *painter) paintStacked(bx *Box) {
	pops := p.pushEffects(bx)
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
	p.paintOutline(bx)
	p.popEffects(pops)
}

// paintHoisted paints a participant e of the stacking context root, under
// the overflow clips of its ancestors on its containing-block chain.
func (p *painter) paintHoisted(root, e *Box) {
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
	n := 0
	for i := len(clips) - 1; i >= 0; i-- {
		if p.pushOverflowClip(clips[i]) {
			n++
		}
	}
	if stacking(e) {
		p.paintStacked(e)
	} else {
		p.paintSelf(e)
		p.paintInner(e)
		p.paintOutline(e)
	}
	for ; n > 0; n-- {
		p.b.PopClip()
	}
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

// paintFlow paints an in-flow box that is not a participant, atomically.
func (p *painter) paintFlow(bx *Box) {
	if bx.ls.Display == layout.DisplayNone || participant(bx) {
		return
	}
	p.paintSelf(bx)
	p.paintInner(bx)
	p.paintOutline(bx)
}

// paintInner paints the content and the in-flow children of bx, clipped
// by its overflow clip.
func (p *painter) paintInner(bx *Box) {
	clipped := clipsOverflow(bx.pcs) && p.pushOverflowClip(bx)
	if bx.pcs.Visibility == style.VisibilityVisible {
		p.paintContent(bx)
	}
	for _, k := range bx.kids {
		p.paintFlow(k)
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
		p.b.PushTransform(cs.Transform.Matrix(bx.abs))
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

// pushOverflowClip pushes the overflow clip of bx: its padding box with the
// inner radii when both axes clip, a rectangle unbounded along the axis
// that does not otherwise. It reports whether it pushed one.
func (p *painter) pushOverflowClip(bx *Box) bool {
	cs := bx.pcs
	x, y := cs.OverflowX, cs.OverflowY
	// CSS: visible becomes auto, and clip hidden, when the other axis is
	// hidden, scroll or auto; only clip and visible combine per axis.
	scrolls := func(o style.Overflow) bool {
		return o == style.OverflowHidden || o == style.OverflowScroll || o == style.OverflowAuto
	}
	both := scrolls(x) || scrolls(y) || (x == style.OverflowClip && y == style.OverflowClip)
	g := p.geom(bx)
	if both {
		if g.padding.IsRect() {
			p.b.PushClipRect(g.padding.Rect)
		} else {
			p.b.PushClipRRect(g.padding)
		}
		return true
	}
	const far = 1 << 24
	r := g.padding.Rect
	if x != style.OverflowClip {
		r.X0, r.X1 = -far, far
	}
	if y != style.OverflowClip {
		r.Y0, r.Y1 = -far, far
	}
	p.b.PushClipRect(r)
	return true
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

// geom returns the snapped geometry of bx.
func (p *painter) geom(bx *Box) boxGeom {
	cs := bx.pcs
	var g boxGeom
	r := p.snapRect(bx.abs)
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
		c := contentRect(bx.abs, l)
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
