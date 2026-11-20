package core

import (
	"fmt"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/semantics"
	"github.com/kuronosu/tela/internal/style"
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
// ancestor of it, which a point between its lines hits. id is 0 for a
// point on nothing (outside the window, before the first frame) and on an
// instrumentation that is off.
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
			if el == hit {
				return id, node, true
			}
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
// "nearest" alignment, each container as its scroll-behavior says), and
// returns the part of the box that is visible once they reach their
// targets: the box cut by the overflow clips of its containing blocks and
// by the window, transforms ignored, empty when none of it is. For an
// element laid out in a paragraph the box is the paragraph's. moved
// reports that the target of a container changed, or that the scroll waits
// for the next layout: the box moves in the frames that follow, and a
// caller that acts on it waits for them and asks again. An element that
// is not mounted or not displayed is an error.
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
	return t.visiblePart(bx), moved, nil
}

// visiblePart returns the part of bx's border box that is visible when the
// scroll containers reach their targets (targetRect): inside the overflow
// clips of the boxes on its containing-block chain, which clip it in
// paint, and inside the window. Empty when none of it is.
func (t *Tree) visiblePart(bx *Box) gfx.Rect {
	r := t.targetRect(bx)
	for b := bx; ; {
		cb := containingBlock(b)
		if cb == nil || fixedToWindow(b, cb) {
			break
		}
		if cs := paintStyle(cb); cs != nil {
			if cx, cy := clipAxes(cs); cx || cy {
				c := t.clientRect(cb)
				if cx {
					r.X0, r.X1 = max(r.X0, c.X0), min(r.X1, c.X1)
				}
				if cy {
					r.Y0, r.Y1 = max(r.Y0, c.Y0), min(r.Y1, c.Y1)
				}
			}
		}
		b = cb
	}
	r = r.Intersect(gfx.Rect{X1: t.viewport.W, Y1: t.viewport.H})
	if r.Empty() {
		return gfx.Rect{}
	}
	return r
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
