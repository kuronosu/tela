package core

import (
	"fmt"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/semantics"
	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/text"
)

// Actionability on an instrumented tree (ADR-299), for the tools that drive
// a running app with real input (tela mcp --attach): before acting on an
// element they scroll it into view (RevealElement) and check that the
// point they act on reaches it (HitElement), as Playwright does before a
// click. telatest's actions make the same checks on semantics nodes
// (telatest/actions.go).

// HitElement returns the element hit testing finds at the point (x, y) of
// the window, in logical pixels (HitTest: the geometry of the last paint,
// with scroll offsets, overflow clips, transforms and the active modal;
// pick mode plays no part), its node in the last semantics tree or its
// nearest ancestor's (HitNode), and whether the point reaches element
// target: the element hit is target or a descendant of it, or, for a
// target laid out in a paragraph (an inline element or a text), an
// ancestor of it hit on one of the target's rectangles (paraParts: a
// point between the lines of a fragment hits the paragraph's element, a
// point on the rest of the paragraph does not reach the target). id is 0
// for a point on nothing (outside the window, before the first frame) and
// on an instrumentation that is off.
func (t *Tree) HitElement(x, y float32, target ElementID) (id ElementID, node semantics.NodeID, within bool) {
	ins := t.ins
	if ins == nil {
		return 0, 0, false
	}
	hit := t.HitTest(x, y)
	if hit == nil {
		return 0, 0, false
	}
	id = ins.idOf(hit)
	for el := hit; el != nil; el = el.parent {
		if el.semID != 0 && t.sem.byID[semantics.NodeID(el.semID)] == el {
			node = semantics.NodeID(el.semID)
			break
		}
	}
	tgt := ins.element(target)
	if tgt == nil {
		return id, node, false
	}
	for el := hit; el != nil; el = el.parent {
		if el == tgt {
			return id, node, true
		}
	}
	if inParagraph(tgt) {
		for el := tgt.parent; el != nil; el = el.parent {
			if el != hit {
				continue
			}
			pt := gfx.Point{X: x, Y: y}
			for _, r := range t.paraParts(tgt, false, nil) {
				if r.Contains(pt) {
					return id, node, true
				}
			}
			break
		}
	}
	return id, node, false
}

// inParagraph reports whether el is laid out in a paragraph's text: a
// text, or an inline element without a box of its own.
func inParagraph(el *Element) bool {
	switch el.kind {
	case elemText:
		return true
	case elemPrimitive:
		return el.box != nil && el.box.inPara != nil
	}
	return false
}

// RevealElement scrolls the scroll containers around element id so that
// its border box is in view, as ElementRef.ScrollIntoView does (CSS's
// "nearest" alignment, each container as its scroll-behavior says;
// transforms ignored, and an element laid out in a paragraph brings the
// paragraph's box), and returns the part of the box that is visible once
// they reach their targets, where it is painted: the box through the
// transforms around it, cut by the overflow clips of its containing
// blocks (through theirs) and by the window, empty when none of it is
// (visiblePart). For an element laid out in a paragraph (an inline
// element, a text) it is the visible part of the first of its rectangles
// that has one (paraParts: one per line or run), as Playwright acts on
// the first of an element's boxes. moved reports that the target of a
// container changed, or that the scroll waits for the next layout: the
// box moves in the frames that follow, and a caller that acts on it waits
// for them and asks again. An element that is not mounted or not
// displayed is an error.
func (t *Tree) RevealElement(id ElementID) (visible gfx.Rect, moved bool, err error) {
	var el *Element
	if t.ins != nil {
		el = t.ins.element(id)
	}
	if el == nil {
		return gfx.Rect{}, false, fmt.Errorf("%w: element %d", ErrNodeGone, id)
	}
	bx := t.chainBox(el)
	if bx == nil {
		return gfx.Rect{}, false, fmt.Errorf("core: element %d is not displayed", id)
	}
	if !t.extentsCurrent() {
		t.intoView(el) // after the next layout (the scroll phase)
		return gfx.Rect{}, true, nil
	}
	moved = t.scrollIntoView(el)
	return t.visiblePart(el, bx), moved, nil
}

// visiblePart returns the part of element el, whose box or paragraph's box
// is bx (chainBox), that is visible when the scroll containers reach their
// targets (targetRect), in window coordinates as paint places it: bx's
// border box through the transforms of bx and its ancestors (the first of
// el's rectangles that shows, for an element laid out in a paragraph),
// inside the overflow clips of the boxes on bx's containing-block chain
// (and of the paragraph's box, which clips its own text), each through its
// box's transforms, and inside the window. Empty when none of it is.
//
// A transformed rectangle is its bounding rectangle, so a rotation counts
// more of a box (and of a clip) than paint shows.
func (t *Tree) visiblePart(el *Element, bx *Box) gfx.Rect {
	clip := gfx.Rect{X1: t.viewport.W, Y1: t.viewport.H}
	cut := func(cb *Box) {
		cs := paintStyle(cb)
		if cs == nil {
			return
		}
		cx, cy := clipAxes(cs)
		if !cx && !cy {
			return
		}
		c := t.clientRect(cb)
		if !cx {
			c.X0, c.X1 = -farEdge, farEdge
		}
		if !cy {
			c.Y0, c.Y1 = -farEdge, farEdge
		}
		clip = clip.Intersect(t.targetUp(cb, c))
	}
	inPara := inParagraph(el)
	if inPara {
		cut(bx)
	}
	for b := bx; ; {
		cb := containingBlock(b)
		if cb == nil || fixedToWindow(b, cb) {
			break
		}
		cut(cb)
		b = cb
	}
	if clip.Empty() {
		return gfx.Rect{}
	}
	if !inPara {
		return t.targetUp(bx, t.targetRect(bx)).Intersect(clip)
	}
	for _, r := range t.paraParts(el, true, nil) {
		if v := r.Intersect(clip); !v.Empty() {
			return v
		}
	}
	return gfx.Rect{}
}

// farEdge stands for no edge: the extent of an overflow clip along the axis
// it does not clip, finite so that a transform maps it without NaNs.
const farEdge = 1 << 24

// targetUp is transformUp in the coordinates of targetRect: it maps r
// through the transforms of bx and its ancestors with their boxes where
// they will be when the scroll containers reach their targets.
func (t *Tree) targetUp(bx *Box, r gfx.Rect) gfx.Rect {
	for b := bx; b != nil && b != t.rootBox; b = b.parent {
		cs := paintStyle(b)
		if cs == nil || cs.Transform.IsIdentity() {
			continue
		}
		r = cs.Transform.Matrix(t.targetRect(b)).TransformRect(r)
	}
	return r
}

// paraParts appends to out the rectangles that show el, a text or an
// inline element laid out in a paragraph, in window coordinates: a text's
// selection rectangles; an inline element's fragments (their border
// boxes) and those of its content (its texts and inline elements, the
// border boxes of its atomic inlines), in that order. They are placed as
// the last frame painted them, or, target, where they will be when the
// scroll containers reach their targets (targetRect), each through the
// transforms around it (its bounding rectangle). Empty ones are left out.
func (t *Tree) paraParts(el *Element, target bool, out []gfx.Rect) []gfx.Rect {
	switch el.kind {
	case elemText:
		return t.fragRects(el, el.paraBox, target, out)
	case elemPrimitive:
		b := el.box
		if b == nil || b.cs == nil || b.cs.Display == style.DisplayNone {
			return out
		}
		switch {
		case b.inPara != nil:
			out = t.fragRects(el, b.inPara, target, out)
		case t.ownBox(el, false) && t.inTree(b):
			// An atomic inline: a box of its own.
			var r gfx.Rect
			if target {
				r = t.targetUp(b, t.targetRect(b))
			} else {
				r = t.boxWindowRect(b)
			}
			if !r.Empty() {
				out = append(out, r)
			}
			return out
		}
	}
	for _, c := range el.children {
		out = t.paraParts(c, target, out)
	}
	return out
}

// fragRects appends the rectangles of text or inline element el in
// paragraph box p (a text's selection rectangles, an inline element's
// fragments) to out, in window coordinates as paraParts says; none when
// p is not laid out or el's text is no longer where it was.
func (t *Tree) fragRects(el *Element, p *Box, target bool, out []gfx.Rect) []gfx.Rect {
	if !semLaidOut(p) {
		return out
	}
	n := len(out)
	if el.kind == elemText {
		off, end := int(el.paraOff), int(el.paraOff)+len(el.text)
		if end > len(p.para.text) || p.para.text[off:end] != el.text {
			return out // a stale location: the text moved
		}
		out = p.para.lay.SelectionRects(text.NewSelection(text.Cursor{Offset: off}, text.Cursor{Offset: end}), out)
	} else {
		out = p.para.inlineRects(el, out)
	}
	o := t.paraContentOrigin(p, target)
	k := n
	for _, r := range out[n:] {
		if r.Empty() {
			continue
		}
		r = r.Translate(o.X, o.Y)
		if target {
			r = t.targetUp(p, r)
		} else {
			r = t.transformUp(p, r)
		}
		out[k] = r
		k++
	}
	return out[:k]
}

// clipAxes reports the axes the overflow of cs clips: as in CSS, visible
// becomes auto when the other axis is hidden, scroll or auto, so both
// clip; only clip and visible combine per axis (overflowClip, paint.go).
func clipAxes(cs *style.ComputedStyle) (x, y bool) {
	scrolls := func(o style.Overflow) bool { return o.Clips() && o != style.OverflowClip }
	if scrolls(cs.OverflowX) || scrolls(cs.OverflowY) {
		return true, true
	}
	return cs.OverflowX.Clips(), cs.OverflowY.Clips()
}
