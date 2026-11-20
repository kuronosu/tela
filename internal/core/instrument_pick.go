package core

import (
	"fmt"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
)

// Pick mode and the highlight of an instrumented tree (ADR-299), drawn by
// the debug overlays (debug.go, ADR-062): over a copy of the application's
// pixels, never in its layers, damage, layout, hit tests or semantics.
//
// In pick mode the box under the pointer is highlighted with its box model
// (Chrome DevTools' colors: content, padding, border, margin) and a label
// with its kind and size, laid out at the display scale like the other
// overlay labels. Pointer events do not reach the app (wheel scrolls do,
// so the page can be scrolled to what is picked): a primary press picks
// the element under it, the nearest one that owns a box (the element hit
// testing finds, through transforms and scroll offsets), calls the
// callback with its ID and ends pick mode; the rest of that press (its
// moves and release) is swallowed too. Escape ends it with the callback
// given 0, and its release does not reach the app either. Highlight shows
// the same highlight for an element chosen elsewhere (the selection in
// tela devtools): a component's are the boxes of the primitives it renders
// at its top level.
//
// While either shows, the tree's overlay flags hold debugInspect, so the
// window takes the overlay path (Debug reports it, and SetDebug keeps it);
// a change of the picked or highlighted element asks for a frame that
// redraws only the overlay.

// pickState is the pick mode and highlight of an instrumented tree.
type pickState struct {
	on bool
	cb func(ElementID)
	// hover is the element under the pointer in pick mode.
	hover *Element
	// hl is the highlighted element.
	hl ElementID
	// swallow is the pointer whose press picked, until it is released;
	// escUp swallows the release of the Escape that ended pick mode.
	swallow    pointerKey
	swallowing bool
	escUp      bool
}

// SetPick turns pick mode on or off. picked is called on the UI thread
// with the ID of the element picked, or 0 when Escape cancelled; pick mode
// is off by then. Turning it off calls nothing. It does nothing while the
// instrumentation is off (SetInstrument).
func (t *Tree) SetPick(on bool, picked func(ElementID)) {
	ins := t.ins
	if ins == nil {
		return
	}
	p := &ins.pick
	p.on, p.hover, p.cb = on, nil, nil
	if on {
		p.cb = picked
	}
	t.syncInspectOverlay()
}

// Picking reports whether pick mode is on.
func (t *Tree) Picking() bool { return t.ins != nil && t.ins.pick.on }

// Highlight highlights element id as pick mode highlights the element
// under the pointer, until another call (0 clears it). It does nothing
// while the instrumentation is off; an element that unmounts is no longer
// drawn.
func (t *Tree) Highlight(id ElementID) {
	ins := t.ins
	if ins == nil || ins.pick.hl == id {
		return
	}
	ins.pick.hl = id
	t.syncInspectOverlay()
}

// Highlighted returns the highlighted element (Highlight), 0 for none.
func (t *Tree) Highlighted() ElementID {
	if t.ins == nil {
		return 0
	}
	return t.ins.pick.hl
}

// syncInspectOverlay keeps debugInspect in the overlay flags while pick
// mode or a highlight shows, and asks for a frame of the overlay.
func (t *Tree) syncInspectOverlay() {
	want := t.ins != nil && (t.ins.pick.on || t.ins.pick.hl != 0)
	d := &t.dbg
	if want == (d.flags&debugInspect != 0) {
		if want {
			t.debugTouch()
		}
		return
	}
	f := d.flags &^ debugInspect
	if want {
		f |= debugInspect
	}
	if (f == 0) == (d.flags == 0) {
		// Other overlays show: the overlay path stays.
		d.flags = f
		t.debugTouch()
		return
	}
	t.setOverlayFlags(f)
}

// pickTarget returns the element pick mode picks at pos: the element hit
// testing finds there, or its nearest ancestor that owns a box (the root
// over the window's background); nil outside the window.
func (t *Tree) pickTarget(pos platform.Point) *Element {
	for el := t.HitTest(pos.X, pos.Y); el != nil; el = el.styleParent() {
		if el.kind == elemPrimitive && el.box != nil && t.ownBox(el, false) && t.inTree(el.box) {
			return el
		}
	}
	return nil
}

// pickPointer handles pointer event e in pick mode, and the rest of the
// press that picked: it reports whether it took e, which then does not
// reach the app.
func (t *Tree) pickPointer(e platform.PointerEvent) bool {
	p := &t.ins.pick
	key := pointerKey{e.Kind, e.ID}
	if p.swallowing && p.swallow == key {
		if e.Action == platform.PointerCancel || e.Action == platform.PointerUp && e.Buttons == 0 {
			p.swallowing = false
		}
		return true
	}
	if !p.on {
		return false
	}
	switch e.Action {
	case platform.PointerEnter, platform.PointerMove:
		t.pickHover(t.pickTarget(e.Pos))
	case platform.PointerLeave:
		t.pickHover(nil)
	case platform.PointerDown:
		if e.Button != platform.ButtonPrimary {
			return true
		}
		id := t.ins.idOf(t.pickTarget(e.Pos))
		cb := p.cb
		p.on, p.cb, p.hover = false, nil, nil
		p.swallow, p.swallowing = key, e.Buttons != 0
		t.syncInspectOverlay()
		if cb != nil {
			cb(id)
		}
	}
	return true
}

// pickHover sets the element pick mode highlights.
func (t *Tree) pickHover(el *Element) {
	p := &t.ins.pick
	if el == p.hover {
		return
	}
	p.hover = el
	t.debugTouch()
}

// pickKey handles key event e in pick mode: Escape ends it. It reports
// whether it took e.
func (t *Tree) pickKey(e platform.KeyEvent) bool {
	p := &t.ins.pick
	if e.Key != platform.KeyEscape {
		return false
	}
	if e.Action == platform.KeyRelease && p.escUp {
		p.escUp = false
		return true
	}
	if !p.on || e.Action != platform.KeyPress {
		return false
	}
	cb := p.cb
	p.on, p.cb, p.hover, p.escUp = false, nil, nil, true
	t.syncInspectOverlay()
	if cb != nil {
		cb(0)
	}
	return true
}

// highlightBoxes appends the boxes that show el: its own, or for an
// element without one (a component, a display: contents primitive) those
// of the primitives it renders at its top level; a text shows its
// parent's.
func (t *Tree) highlightBoxes(el *Element, out []*Box) []*Box {
	switch el.kind {
	case elemText:
		if p := el.styleParent(); p != nil && p.parent != nil {
			return t.highlightBoxes(p, out)
		}
		return out
	case elemPrimitive:
		if el.box != nil && el.box.cs != nil && t.ownBox(el, false) && t.inTree(el.box) {
			return append(out, el.box)
		}
	}
	for _, c := range el.children {
		if c.kind != elemText {
			out = t.highlightBoxes(c, out)
		}
	}
	return out
}

// inspect draws the highlight of an instrumented tree: the highlighted
// element, then the one under the pointer in pick mode.
func (o *overlay) inspect() {
	ins := o.t.ins
	if ins == nil {
		return
	}
	if el := ins.element(ins.pick.hl); el != nil {
		o.highlight(el)
	}
	if el := ins.pick.hover; ins.pick.on && el != nil && el.mounted {
		o.highlight(el)
	}
}

// highlight draws the box model of the boxes that show el, with a label.
func (o *overlay) highlight(el *Element) {
	t := o.t
	var u gfx.Rect
	n := 0
	for _, b := range t.highlightBoxes(el, nil) {
		r := t.boxWindowRect(b)
		o.boxModel(b, r, o.win, 1)
		if n == 0 {
			u = r
		} else {
			u = u.Union(r)
		}
		n++
	}
	if n == 0 || u.Empty() {
		return
	}
	o.labels = append(o.labels, ovLabel{text: inspectLabel(el, u), box: u, accent: colHoverLine, top: true})
}

// inspectLabel is the label of a highlight: the element's kind (with the
// component that made a primitive) and its size, "button kit.Button
// 120×36".
func inspectLabel(el *Element, r gfx.Rect) string {
	kind, _ := elementKind(el)
	if el.kind == elemPrimitive {
		if c := el.component(); c != nil {
			ck, _ := elementKind(c)
			kind += " " + ck
		}
	}
	return fmt.Sprintf("%s  %s×%s", truncateRunes(kind, maxLabelRune), fmtPx(r.Width()), fmtPx(r.Height()))
}
