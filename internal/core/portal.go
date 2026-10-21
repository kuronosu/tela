package core

import (
	"math"
	"slices"

	"github.com/kuronosu/tela/internal/style"
)

// Portals (ARCHITECTURE section 13, ADR-077): content that belongs to a
// component but is shown above everything else, like React's createPortal
// into document.body.
//
// A Portal node is a primitive element whose box is not part of its
// parent's box structure (collect skips it): it is the portal layer, a
// child of the root box after the root's content, one per mounted Portal
// in mount order. Its style is fixed inset-0 with the largest z-index, so
// it is laid out against the window (its fixed and absolute descendants
// position against the window too), is a stacking context of its own and
// paints above the root's content, the portals mounted before it above
// nothing of it (the same z-index: tree order, which is mount order). It
// inherits the text properties of its parent in the element tree. The
// layer itself is never hit, so the content under its empty parts gets
// the events.
//
// Everything that follows the element tree is unchanged, as in React:
// state lives with the components, context flows from the Portal's
// ancestors, events bubble from the portal's content to the Portal's
// ancestors (dispatch walks Element.styleParent), and hover and focus
// states follow the elements. The orders that follow the document in the
// browser put the portal's content after the root's content, as content
// appended to body: sequential focus navigation (focus.go) and the
// semantics tree and aria snapshot (semantics.go).

// Portal returns a portal node: its children are shown in a layer above
// the window's content (see the comment at the top of portal.go). It
// takes children and a Key; other props panic, since the layer has no
// style of its own (style its content).
func Portal(children ...Child) Node {
	d := &desc{kind: nodeElement, elem: style.KindDiv, portal: true}
	for _, c := range children {
		if c == nil {
			d.children = append(d.children, Node{})
			continue
		}
		c.applyChild(d)
	}
	switch {
	case len(d.classes) > 0 || len(d.props) > 0:
		panic("h: Class or Style on Portal: the portal layer covers the window and has no style of its own; style its content: Portal(Div(Class(\"fixed inset-0 ...\"), ...))")
	case len(d.events) > 0 || d.ref != nil || d.hasTab || d.autoFocus || d.scope != nil || d.disabled ||
		d.aria != nil || d.fp != nil || d.id != "" || d.virt != nil || d.src != nil || d.img != nil || d.href != "" ||
		d.animLayout || d.layoutID != "" || d.testID != "":
		panic("h: Portal takes children and a Key only; put handlers, refs and other props on its content (events from it bubble to the Portal's ancestors)")
	}
	return Node{d}
}

// isPortal reports whether el is a mounted Portal's element.
func isPortal(el *Element) bool {
	return el.kind == elemPrimitive && el.node.d.portal
}

// portalZ is the z-index of a portal layer: above every other box of the
// root's stacking context.
const portalZ = math.MaxInt32

// mountPortal records a new Portal element (mount order) and restructures
// the root.
func (t *Tree) mountPortal(el *Element) {
	el.box.portal = true
	t.portals = append(t.portals, el)
	t.markBoxes(t.root)
}

// unmountPortal forgets an unmounted Portal element.
func (t *Tree) unmountPortal(el *Element) {
	if i := slices.Index(t.portals, el); i >= 0 {
		t.portals = slices.Delete(t.portals, i, i+1)
	}
	t.markBoxes(t.root)
}

// portalStyle makes cs, computed for a portal layer, fixed to the window
// with the largest z-index.
func portalStyle(cs *style.ComputedStyle) {
	zero := style.Dim{Kind: style.DimLength}
	cs.Display = style.DisplayBlock
	cs.Position = style.PositionFixed
	cs.Inset = style.Sides[style.Dim]{Top: zero, Right: zero, Bottom: zero, Left: zero}
	cs.ZIndex = style.ZIndex{Explicit: true, Value: portalZ}
}

// portalBoxes appends the boxes of the portal layers to the root's
// children, in mount order.
func (t *Tree) portalBoxes(kids []*Box) []*Box {
	for _, p := range t.portals {
		if !p.mounted || p.box.cs == nil {
			continue
		}
		t.ensureBuilt(p)
		kids = append(kids, p.box)
	}
	return kids
}

// portalsIn returns the mounted portals inside el (el excluded), in mount
// order.
func (t *Tree) portalsIn(el *Element, out []*Element) []*Element {
	for _, p := range t.portals {
		if p != el && p.mounted && isInside(p, el) {
			out = append(out, p)
		}
	}
	return out
}
