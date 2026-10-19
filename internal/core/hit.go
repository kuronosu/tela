package core

import (
	"cmp"
	"slices"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/layout"
	"github.com/kuronosu/tela/internal/style"
)

// Hit-testing (ARCHITECTURE section 14) finds the element under a point:
// the box tree is walked in the reverse of the paint order of paint.go
// (stacking contexts, z-index, hoisted positioned boxes, in-flow boxes),
// with the geometry of the last paint (what is on screen), so it agrees
// with the pixels:
//
//   - a box is hit inside its snapped border box with its rounded
//     corners; outlines and shadows are not hit (CSS);
//   - the overflow clips of the ancestors that clip it in paint, with
//     their rounded corners, cut it;
//   - transforms (including the paint-phase deferred ones, TranslateFn)
//     are undone, so a scaled or moved box is hit where it is drawn; a
//     box with a singular transform (scale-0) is hit nowhere;
//   - a box with pointer-events: none or visibility: hidden is not hit
//     itself, but its descendants may be (both inherit, and a descendant
//     may set them back, as in CSS);
//   - inside a paragraph, a point on text hits the inline element (Span,
//     Strong, A...) whose text it is (text.Layout.HitTestCluster), or the
//     paragraph's element; a point on the border box of an inline
//     element's fragment (its padding, inline.go) hits that element; an
//     atomic inline (a button in the text) is hit as a box, a child of the
//     paragraph box; a point between lines or beside them hits the
//     paragraph's element.
//
// Disabled elements are hit like the others; dispatch skips their
// handlers (input.go). The geometry is where boxes are painted (Box.u,
// the layout without the scroll offsets), and the point is moved into the
// scroll space of each box it is compared with (toSpace: the offsets of
// the scroll containers whose content layers hold it), so a scroll moves
// what is hit without placing the boxes again. A visible scrollbar takes
// the hit before
// what is painted under it, the scroll container's positioned
// descendants with a z-index of 0 or more included when the container is
// a stacking context (scrollbar.go); the press then targets the
// container and records the bar (inputState.bar).

// hitter is the state of a hit test. It reuses the painter for the
// snapped geometry and the collection of stacking participants.
type hitter struct {
	painter
}

// HitTest returns the element under the point (x, y) of the window, in
// logical pixels: the primitive whose handlers an event there targets,
// the root for a point on nothing else, and nil outside the window or
// before the first frame.
func (t *Tree) HitTest(x, y float32) *Element {
	return t.hitTest(gfx.Point{X: x, Y: y})
}

func (t *Tree) hitTest(pt gfx.Point) *Element {
	rb := t.rootBox
	if rb == nil || rb.pcs == nil || !(gfx.Rect{X1: t.viewport.W, Y1: t.viewport.H}).Contains(pt) {
		return nil
	}
	h := &t.in.hit
	h.t, h.scale = t, float64(t.scale)
	t.in.bar = barHit{}
	el := h.stacked(rb, pt)
	clear(h.items)
	h.items = h.items[:0]
	return el
}

// alive reports whether bx was painted and its element still exists and
// takes events: a box of an element unmounted since the last frame stays
// in its parent's children until the next box phase, and the boxes of an
// exiting Presence child are inert (presence.go).
func alive(bx *Box) bool {
	return bx.pcs != nil && bx.owner != nil && bx.owner.mounted && !bx.owner.inert
}

// hitTarget returns the element a hit on box bx targets: its own, or the
// enclosing primitive of an anonymous box.
func hitTarget(bx *Box) *Element {
	if bx.el != nil {
		return bx.el
	}
	return bx.owner
}

// toSpace moves point p from scroll space from to scroll space to (the
// coordinates boxes painted in them use: see Box.u and layer.go).
func toSpace(p gfx.Point, from, to *scroller) gfx.Point {
	if from == to {
		return p
	}
	return moveSpace(p, from, to)
}

// moveSpace is toSpace between different spaces (kept apart so that
// toSpace inlines).
func moveSpace(p gfx.Point, from, to *scroller) gfx.Point {
	return p.Add(shiftOf(to)).Sub(shiftOf(from))
}

// contentSpace returns the scroll space of the content of bx: its own
// when its content moves (a content layer), else the one bx is in.
func contentSpace(bx *Box) *scroller {
	if s := bx.scr; s != nil && s.chunked {
		return s
	}
	return bx.space
}

// stacked hit-tests the stacking context bx; p is in the coordinates of
// the scroll space bx is painted in (its own transform not applied yet).
func (h *hitter) stacked(bx *Box, p gfx.Point) *Element {
	if !bx.hitBounds.Contains(p) {
		return nil
	}
	if bx != h.t.rootBox && !bx.pcs.Transform.IsIdentity() {
		inv, ok := bx.pcs.Transform.Matrix(bx.u).Invert()
		if !ok {
			return nil // a singular transform draws nothing
		}
		p = inv.Apply(p)
	}
	start := len(h.items)
	h.collectAt(bx, p)
	items := h.items[start:]
	slices.SortStableFunc(items, func(a, b paintItem) int { return cmp.Compare(a.z, b.z) })
	el := h.stackedOrder(bx, items, p)
	clear(h.items[start:])
	h.items = h.items[:start]
	return el
}

// collectAt appends to h.items the participants of the stacking context
// whose descendants are under bx that may be hit at p (in bx's scroll
// space): painter.collect, without the subtrees whose bounds miss p.
func (h *hitter) collectAt(bx *Box, p gfx.Point) {
	for _, k := range bx.kids {
		pk := toSpace(p, bx.space, k.space)
		if !k.hitBounds.Contains(pk) || k.ls.Display == layout.DisplayNone || k.pcs == nil || k.hiddenAtom {
			continue
		}
		if participant(k) {
			h.items = append(h.items, paintItem{k, zIndex(k)})
			if stacking(k) {
				continue
			}
		}
		h.collectAt(k, pk)
	}
}

// stackedOrder walks the layers of stacking context bx from the top:
// participants with z >= 0, the content and in-flow children, the
// participants with a negative z, the box itself.
func (h *hitter) stackedOrder(bx *Box, items []paintItem, p gfx.Point) *Element {
	if bx.scr != nil {
		if el := h.scrollbar(bx, p); el != nil {
			return el
		}
	}
	i := len(items) - 1
	for ; i >= 0 && items[i].z >= 0; i-- {
		if el := h.hoisted(bx, items[i].bx, p); el != nil {
			return el
		}
	}
	if el := h.inner(bx, p); el != nil {
		return el
	}
	for ; i >= 0; i-- {
		if el := h.hoisted(bx, items[i].bx, p); el != nil {
			return el
		}
	}
	if h.self(bx, p) {
		return hitTarget(bx)
	}
	return nil
}

// hoisted hit-tests participant e of stacking context root, inside the
// overflow clips of its containing-block chain (paintHoisted's clips); p
// is in root's scroll space.
func (h *hitter) hoisted(root, e *Box, p gfx.Point) *Element {
	pe := toSpace(p, root.space, e.space)
	if !e.hitBounds.Contains(pe) || !alive(e) {
		return nil
	}
	cb := containingBlock(e)
	for a := e.parent; a != nil; a = a.parent {
		if a == cb {
			if clipsOverflow(a.pcs) && !h.inClip(a, toSpace(p, root.space, a.space)) {
				return nil
			}
			cb = containingBlock(a)
		}
		if a == root {
			break
		}
	}
	if stacking(e) {
		return h.stacked(e, pe)
	}
	return h.flowBox(e, pe)
}

// flowBox hit-tests a box painted atomically: its children and content
// above itself.
func (h *hitter) flowBox(bx *Box, p gfx.Point) *Element {
	if el := h.inner(bx, p); el != nil {
		return el
	}
	if h.self(bx, p) {
		return hitTarget(bx)
	}
	return nil
}

// inner hit-tests the in-flow children (topmost first) and the content of
// bx, inside its overflow clip; p is in bx's scroll space.
func (h *hitter) inner(bx *Box, p gfx.Point) *Element {
	if bx.scr != nil {
		if el := h.scrollbar(bx, p); el != nil {
			return el
		}
	}
	if clipsOverflow(bx.pcs) && !h.inClip(bx, p) {
		return nil
	}
	for i := len(bx.kids) - 1; i >= 0; i-- {
		k := bx.kids[i]
		pk := toSpace(p, bx.space, k.space)
		if !k.hitBounds.Contains(pk) || k.ls.Display == layout.DisplayNone || !alive(k) || participant(k) || k.hiddenAtom {
			continue
		}
		if el := h.flowBox(k, pk); el != nil {
			return el
		}
	}
	if bx.mode == modeParagraph && bx.pcs.Visibility == style.VisibilityVisible {
		return h.text(bx, toSpace(p, bx.space, contentSpace(bx)))
	}
	return nil
}

// scrollbar returns the scroll container bx when p is on one of its
// visible scrollbars, recording the part hit.
func (h *hitter) scrollbar(bx *Box, p gfx.Point) *Element {
	i, part := bx.scr.barAt(p)
	if part == barNone {
		return nil
	}
	h.t.in.bar = barHit{s: bx.scr, axis: i, part: part, at: p}
	return hitTarget(bx)
}

// self reports whether bx itself is hit: inside its border box, visible
// and accepting pointer events.
func (h *hitter) self(bx *Box, p gfx.Point) bool {
	cs := bx.pcs
	if cs.Visibility != style.VisibilityVisible || cs.PointerEvents == style.PointerEventsNone {
		return false
	}
	return rrectContains(h.geom(bx).border, p)
}

// inClip reports whether p is inside the overflow clip of bx.
func (h *hitter) inClip(bx *Box, p gfx.Point) bool {
	return rrectContains(h.overflowClip(bx), p)
}

// text returns the inline element whose text of paragraph box bx is under
// p (in the scroll space of bx's content), or nil for a point off the text
// or on text of the paragraph's own element (the caller hit-tests the box
// itself next).
func (h *hitter) text(bx *Box, p gfx.Point) *Element {
	para := bx.para
	if para == nil || para.lay == nil || para.empty {
		return nil
	}
	c := contentRect(bx.u, bx.ld.Layout())
	lp := gfx.Point{X: p.X - c.X0, Y: p.Y - c.Y0}
	fk, fline := -1, -1
	if len(para.inl) > 0 {
		// The decorations of inline elements (inline.go): under the text
		// of their line, over the lines before.
		fk, fline = para.fragmentAt(lp)
	}
	frag := func() *Element {
		if fk >= 0 && para.inl[fk].mounted {
			return para.inl[fk]
		}
		return nil
	}
	hit, ok := para.lay.HitTestCluster(lp.X, lp.Y)
	if !ok || hit.Cluster < 0 || hit.Box >= 0 {
		return frag()
	}
	if fk >= 0 && fline > hit.Line {
		return frag()
	}
	owner := bx.owner
	for el := para.targetAt(para.lay.Cluster(hit.Cluster).TextStart); el != nil && el != owner; el = el.styleParent() {
		if !el.mounted || el.kind != elemPrimitive || el.inert {
			continue
		}
		if cs := el.box.cs; cs != nil && cs.PointerEvents == style.PointerEventsAuto && cs.Visibility == style.VisibilityVisible {
			return el
		}
	}
	return frag()
}

// rrectContains reports whether p is inside rr (half-open edges, like
// gfx.Rect.Contains), its elliptical corners included.
func rrectContains(rr gfx.RRect, p gfx.Point) bool {
	r := rr.Rect
	if !r.Contains(p) {
		return false
	}
	// The corners: top-left, top-right, bottom-right, bottom-left.
	for i, c := range rr.Radii {
		if c.X <= 0 || c.Y <= 0 {
			continue
		}
		cx, cy := r.X0+c.X, r.Y0+c.Y
		inX, inY := p.X < cx, p.Y < cy
		switch i {
		case 1:
			cx, inX = r.X1-c.X, p.X > r.X1-c.X
		case 2:
			cx, inX = r.X1-c.X, p.X > r.X1-c.X
			cy, inY = r.Y1-c.Y, p.Y > r.Y1-c.Y
		case 3:
			cy, inY = r.Y1-c.Y, p.Y > r.Y1-c.Y
		}
		if !inX || !inY {
			continue
		}
		dx := float64(p.X-cx) / float64(c.X)
		dy := float64(p.Y-cy) / float64(c.Y)
		return float64(dx*dx)+float64(dy*dy) <= 1
	}
	return true
}

// ---------------------------------------------------------------------
// Local coordinates.

// geomBox returns the box whose border box gives el its local
// coordinates: its own box when it is in the box tree; for an inline
// element of a paragraph (which has no box of its own) the paragraph's;
// for a display: contents or not painted element, the nearest ancestor's.
func (t *Tree) geomBox(el *Element) *Box {
	for e := el; e != nil; e = e.styleParent() {
		if e.kind != elemPrimitive {
			continue
		}
		b := e.box
		if b.inPara != nil && alive(b.inPara) {
			return b.inPara
		}
		if b.pcs != nil && (b.parent != nil || b == t.rootBox) {
			return b
		}
	}
	return nil
}

// localPoint returns the window point p in the coordinates of el's border
// box: the scroll offsets and every transform of its box and of the box's
// ancestors undone, relative to its top-left corner.
func (t *Tree) localPoint(el *Element, p gfx.Point) gfx.Point {
	bx := t.geomBox(el)
	if bx == nil {
		return p
	}
	chain := t.in.chain[:0]
	for b := bx; b != nil; b = b.parent {
		chain = append(chain, b)
	}
	var space *scroller
	for i := len(chain) - 1; i >= 0; i-- {
		b := chain[i]
		p, space = toSpace(p, space, b.space), b.space
		if b == t.rootBox || b.pcs == nil || b.pcs.Transform.IsIdentity() {
			continue
		}
		if inv, ok := b.pcs.Transform.Matrix(b.u).Invert(); ok {
			p = inv.Apply(p)
		}
	}
	clear(chain)
	t.in.chain = chain[:0]
	return p.Sub(bx.u.Origin())
}

// windowRect returns the unsnapped border box of bx in window coordinates
// (transforms ignored): where it is painted, moved by the scroll offsets.
func windowRect(bx *Box) gfx.Rect {
	d := shiftOf(bx.space)
	return bx.u.Translate(-d.X, -d.Y)
}
