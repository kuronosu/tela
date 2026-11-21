package core

import (
	"fmt"
	"slices"

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
// overlay labels. A primary press picks the element under it, the nearest
// one that owns a box (found by PickTest, as Chrome's inspect picker finds
// it: the box painted there, through transforms and scroll offsets,
// whatever its pointer-events and inert ones included), calls the
// callback with its ID and ends pick mode; the rest of that press (its
// moves and release) does not reach the app. Escape ends it with the
// callback given 0.
//
// Pick mode swallows the app's input: pointer events, wheel scrolls (the
// page under the picker stays still, as the DevTools window shows it),
// keys and IME text, and caption clicks. The title bar is pick mode's
// too: the window gets no hit regions while it is on (Host.syncRegions),
// so a press there picks instead of moving the window, and a caption
// click the OS took does not reach the app (it would close its overlays).
// The one exception is the end of a press the app saw begin: a pointer
// held when pick mode starts keeps its releases (and its cancel) for the
// app, and so does a key pressed before (its repeats are swallowed), so
// that the app never holds a press it cannot end. Presses swallowed keep
// their releases from the app after pick mode ends too.
//
// Highlight shows the same highlight for an element chosen elsewhere (the
// selection in tela devtools): a component's are the boxes of the
// primitives it renders at its top level.
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
	// swallow are the pointers whose presses stay out of the app until
	// they are released: the press that picked, and those made in pick
	// mode. pass are the pointers the app held pressed when pick mode
	// started, with the buttons it held: their releases reach it.
	swallow []pointerKey
	pass    map[pointerKey]platform.Buttons
	// keys are the physical keys whose presses pick mode took (Escape's
	// included): their releases stay out of the app.
	keys []platform.KeyCode
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
	if on && !p.on {
		// The presses the app holds keep their releases (the app's view of
		// its pointers: what pick mode swallowed never reached it).
		clear(p.pass)
		for _, k := range t.pointerKeys() {
			ps := t.in.ptrs[k]
			if ps == nil || ps.buttons == 0 && !ps.pressing {
				continue
			}
			held := ps.buttons
			if held == 0 {
				held = platform.ButtonPrimary.Mask() // a touch's press holds no button
			}
			if p.pass == nil {
				p.pass = map[pointerKey]platform.Buttons{}
			}
			p.pass[k] = held
		}
	}
	p.on, p.hover, p.cb = on, nil, nil
	if on {
		p.cb = picked
	}
	t.syncInspectOverlay()
}

// swallowPress keeps the rest of the press of pointer k from the app.
func (p *pickState) swallowPress(k pointerKey) {
	if !slices.Contains(p.swallow, k) {
		p.swallow = append(p.swallow, k)
	}
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

// pickTarget returns the element pick mode picks at pos: the element pick
// mode's hit test finds there (PickTest), or its nearest ancestor that
// owns a box (the root over the window's background); nil outside the
// window.
func (t *Tree) pickTarget(pos platform.Point) *Element {
	for el := t.PickTest(pos.X, pos.Y); el != nil; el = el.styleParent() {
		if el.kind == elemPrimitive && el.box != nil && t.ownBox(el, false) && t.inTree(el.box) {
			return el
		}
	}
	return nil
}

// pickPointer handles pointer event e in pick mode, the rest of the
// presses it swallowed, and the releases of the presses the app held when
// it started: it reports whether it took e, which then does not reach the
// app.
func (t *Tree) pickPointer(e platform.PointerEvent) bool {
	p := &t.ins.pick
	key := pointerKey{e.Kind, e.ID}
	if held, ok := p.pass[key]; ok {
		// A press the app saw begin ends in the app, in pick mode or after.
		switch {
		case e.Action == platform.PointerCancel:
			delete(p.pass, key)
			return false
		case e.Action == platform.PointerUp && (held&e.Button.Mask() != 0 || e.Buttons == 0):
			if held &^= e.Button.Mask(); held == 0 || e.Buttons == 0 {
				delete(p.pass, key)
			} else {
				p.pass[key] = held
			}
			return false
		}
	}
	if i := slices.Index(p.swallow, key); i >= 0 {
		if e.Action == platform.PointerCancel || e.Action == platform.PointerUp && e.Buttons == 0 {
			p.swallow = slices.Delete(p.swallow, i, i+1)
		}
		if !p.on {
			return true
		}
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
		p.swallowPress(key) // any button's: the app never sees this press
		if e.Button != platform.ButtonPrimary {
			return true
		}
		id := t.ins.idOf(t.pickTarget(e.Pos))
		cb := p.cb
		p.on, p.cb, p.hover = false, nil, nil
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

// pickKey handles key event e in pick mode, and the releases of the keys
// it took: Escape ends it, every other key is swallowed but the release
// of one pressed before. It reports whether it took e.
func (t *Tree) pickKey(e platform.KeyEvent) bool {
	p := &t.ins.pick
	if e.Action == platform.KeyRelease {
		if i := slices.Index(p.keys, e.Code); i >= 0 {
			p.keys = slices.Delete(p.keys, i, i+1)
			return true
		}
		return false // a key pressed before pick mode: the app ends it
	}
	if !p.on {
		return false
	}
	if !e.Repeat && !slices.Contains(p.keys, e.Code) {
		// A repeat is of a key held from before: its release still
		// reaches the app.
		p.keys = append(p.keys, e.Code)
	}
	if e.Key == platform.KeyEscape && !e.Repeat {
		cb := p.cb
		p.on, p.cb, p.hover = false, nil, nil
		t.syncInspectOverlay()
		if cb != nil {
			cb(0)
		}
	}
	return true
}

// pickSwallows reports whether pick mode keeps the input that has no
// press to end (wheel scrolls, IME text) from the app.
func (t *Tree) pickSwallows() bool { return t.ins != nil && t.ins.pick.on }

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
