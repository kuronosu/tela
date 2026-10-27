package core

import (
	"fmt"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/style"
)

// Custom title bars (ADR-178, ADR-180): props that mark the parts of a
// window's content that are its title bar, for frameless windows (and any
// window: a drag area in a toolbar).
//
//   - WindowDrag marks a caption (drag) region: a press there moves the
//     window, a double click maximizes it, a right click opens the window
//     menu, as the OS does in its own title bar (the backend does it,
//     platform.HitCaption).
//   - Inside a drag region, the interactive elements are holes
//     (platform.HitClient), so they keep working: elements with press or
//     drag handlers, focusable elements (buttons, TabIndex), text fields
//     and links, and WindowNoDrag, the explicit opt-out (whose subtree may
//     hold drag regions again, as Electron's app-region: no-drag).
//   - WindowControlProp marks a window-control button (minimize,
//     maximize, close): a click on it does the action by itself, and its
//     region tells the OS what it is (Windows 11 shows snap layouts over
//     the maximize button).
//
// After every frame that ran the pipeline, the Host computes the window's
// regions from the boxes as painted (window coordinates, moved by scroll
// offsets and transforms, cut by the overflow clips of the ancestors and
// by the window) in tree order, later regions over earlier ones, and calls
// SetHitRegions only when they changed. A tree without these props
// computes nothing (Tree.reg.n is 0), and the walk only enters the
// subtrees that hold them (Element.winKids).

// Values of desc.winRegion.
const (
	regionNone uint8 = iota
	regionDrag
	regionNoDrag
	regionMinimize
	regionMaximize
	regionClose
)

// regionState counts the mounted primitives with a region prop.
type regionState struct{ n int }

// WindowDrag returns the prop that makes an element a drag region of the
// window's title bar (see the comment at the top of winregion.go).
func WindowDrag() Prop {
	return Prop{name: "WindowDrag", apply: func(d *desc) { d.winRegion = regionDrag }}
}

// WindowNoDrag returns the prop that keeps an element inside a drag
// region from dragging the window.
func WindowNoDrag() Prop {
	return Prop{name: "WindowNoDrag", apply: func(d *desc) { d.winRegion = regionNoDrag }}
}

// WindowControlProp returns the prop that makes an element a
// window-control button: a click on it minimizes, maximizes (or
// restores) or asks to close the window.
func WindowControlProp(c WindowControl) Prop {
	if c < WindowMinimize || c > WindowClose {
		panic(fmt.Sprintf("h: WindowControl(%v): pass tela.WindowMinimize, tela.WindowMaximize or tela.WindowClose", c))
	}
	r := regionMinimize + uint8(c-WindowMinimize)
	return Prop{name: "WindowControl", apply: func(d *desc) { d.winRegion = r }}
}

// regionChanged records that primitive el's region prop went from old to
// now (regionNone when it mounts or unmounts).
func (t *Tree) regionChanged(el *Element, old, now uint8) {
	switch {
	case old == regionNone && now != regionNone:
		t.reg.n++
		for p := el.parent; p != nil && !p.winKids; p = p.parent {
			p.winKids = true
		}
	case old != regionNone && now == regionNone:
		t.reg.n--
	}
}

// hitRegions appends the window's hit regions to out (see the comment at
// the top of winregion.go).
func (t *Tree) hitRegions(out []platform.HitRegion) []platform.HitRegion {
	if t.reg.n == 0 || t.root == nil {
		return out
	}
	vp := gfx.Rect{X1: t.viewport.W, Y1: t.viewport.H}
	return t.regionWalk(t.root, false, vp, out)
}

// regionWalk appends the regions of the subtree below el; drag reports
// that el is inside a drag region (its interactive elements are holes).
func (t *Tree) regionWalk(el *Element, drag bool, vp gfx.Rect, out []platform.HitRegion) []platform.HitRegion {
	for _, c := range el.children {
		switch c.kind {
		case elemText:
			continue
		case elemComponent:
			if drag || c.winKids {
				out = t.regionWalk(c, drag, vp, out)
			}
			continue
		}
		if cs := c.box.cs; cs == nil || cs.Display == style.DisplayNone {
			continue
		}
		switch r := c.node.d.winRegion; {
		case r >= regionMinimize:
			out = t.appendRegion(out, c, platform.HitMinimize+platform.HitKind(r-regionMinimize), vp)
		case r == regionDrag:
			out = t.appendRegion(out, c, platform.HitCaption, vp)
			out = t.regionWalk(c, true, vp, out)
		case drag && (r == regionNoDrag || interactive(c)):
			out = t.appendRegion(out, c, platform.HitClient, vp)
			if r == regionNoDrag && c.winKids {
				out = t.regionWalk(c, false, vp, out)
			}
		case drag || c.winKids:
			out = t.regionWalk(c, drag, vp, out)
		}
	}
	return out
}

// pressMask are the handlers that make an element interactive for the
// pointer: it is a hole in a drag region.
const pressMask = clickMask | dragMask | 1<<evContextMenu | 1<<evMouseDown | 1<<evMouseUp |
	1<<evPointerDown | 1<<evPointerUp | 1<<evLongPress

// interactive reports whether primitive el takes pointer input of its
// own: press or drag handlers, focusable (a Button, a TabIndex), a text
// field or a link.
func interactive(el *Element) bool {
	d := el.node.d
	return d.evMask&pressMask != 0 || d.hasTab || el.box.kind == style.KindButton ||
		el.box.fld != nil || d.elem.IsTextField() || d.href != ""
}

// appendRegion appends the region of kind for el's bounds, cut by the
// overflow clips of its ancestors and by the window, unless empty.
func (t *Tree) appendRegion(out []platform.HitRegion, el *Element, kind platform.HitKind, vp gfx.Rect) []platform.HitRegion {
	r := t.clipUp(el, t.elemBounds(el)).Intersect(vp)
	if r.Empty() {
		return out
	}
	return append(out, platform.HitRegion{Kind: kind, Rect: platform.Rect{X: r.X0, Y: r.Y0, W: r.Width(), H: r.Height()}})
}

// clipUp cuts r, in window coordinates, by the overflow clips of the boxes
// above el's (their padding boxes, on the axes they clip).
func (t *Tree) clipUp(el *Element, r gfx.Rect) gfx.Rect {
	var b *Box
	if el.kind == elemPrimitive && t.ownBox(el, false) && t.inTree(el.box) {
		b = el.box.parent
	} else if p := el.styleParent(); p != nil {
		b = p.box
	}
	for ; b != nil && b != t.rootBox && !r.Empty(); b = b.parent {
		cs := b.cs
		if cs == nil || b.el == nil {
			continue
		}
		cx, cy := cs.OverflowX.Clips(), cs.OverflowY.Clips()
		if !cx && !cy {
			continue
		}
		c := t.paddingWindowRect(b)
		if cx {
			r.X0, r.X1 = max(r.X0, c.X0), min(r.X1, c.X1)
		}
		if cy {
			r.Y0, r.Y1 = max(r.Y0, c.Y0), min(r.Y1, c.Y1)
		}
	}
	return r
}

// paddingWindowRect returns the bounding rectangle of bx's transformed
// padding box in window coordinates.
func (t *Tree) paddingWindowRect(bx *Box) gfx.Rect {
	l := bx.ld.Layout()
	o := t.semOrigin(bx)
	r := gfx.Rect{
		X0: o.X + l.Border.Left, Y0: o.Y + l.Border.Top,
		X1: o.X + l.Size.W - l.Border.Right, Y1: o.Y + l.Size.H - l.Border.Bottom,
	}
	return t.transformUp(bx, r)
}

// windowControlAt returns the region of the nearest window-control
// element at or above target, regionNone without one.
func windowControlAt(target *Element) uint8 {
	for el := mountedAncestor(target); el != nil; el = el.styleParent() {
		if el.kind == elemPrimitive {
			if r := el.node.d.winRegion; r >= regionMinimize {
				return r
			}
		}
	}
	return regionNone
}

// windowControl performs the action of a window-control button (region
// r) on the tree's window.
func (t *Tree) windowControl(r uint8) {
	v := t.wv
	if v == nil || !v.alive() {
		return
	}
	switch r {
	case regionMinimize:
		v.win.Minimize()
	case regionMaximize:
		v.win.SetMaximized(!v.os.Maximized)
	case regionClose:
		v.host.requestClose(v)
	}
}

// HitRegions returns the hit regions of the tree's window as the Host
// computes them after a frame (custom title bars), for tests.
func (t *Tree) HitRegions() []platform.HitRegion { return t.hitRegions(nil) }
