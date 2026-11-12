package core

import (
	"image"

	"github.com/kuronosu/tela/internal/gesture"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
	"github.com/kuronosu/tela/internal/layout"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/text"
)

// The static selection as a drag source and as the anchor of a keyboard
// context menu (ADR-271; see textsel.go).

// selPiece is a paragraph's part of a dragged static selection.
type selPiece struct {
	bx  *Box
	sel text.Selection
}

// selDragRecognizer returns the drag recognizer of a press of ps on the
// static selection, or nil.
func (t *Tree) selDragRecognizer(ps *pointerState) gesture.Recognizer {
	if t.tsel.inSel != ps {
		return nil
	}
	return &gesture.Drag{OnStart: func(u gesture.DragUpdate) { t.selDragStart(ps, u) }}
}

// selDragStart starts the drag and drop of the static selection pressed by
// ps, whose drag gesture started (u): the selected text as text/plain,
// copied (the text is not editable), from the element pressed, whose
// OnDragStart runs (not bubbling, as a field's). When it is cancelled the
// press selects from where it was, as a press outside the selection does
// (Chromium extends the selection after a cancelled dragstart).
func (t *Tree) selDragStart(ps *pointerState, u gesture.DragUpdate) {
	s := &t.tsel
	if s.inSel != ps {
		return
	}
	s.inSel = nil
	t.in.clicks.Reset()
	ps.ctxClick = false
	press := gfx.Point{X: u.Pos.X - u.Offset.X, Y: u.Pos.Y - u.Offset.Y}
	txt := t.selectedText()
	el := mountedAncestor(ps.down)
	if txt == "" || el == nil {
		return
	}
	// The highlighted paragraphs in document order (hl has none).
	var pieces []selPiece
	seen := map[*Box]bool{}
	start, end := t.selOrdered()
	t.walkRange(start, end, func(bx *Box, _, _ text.Cursor, _ int) bool {
		if pa := bx.para; !seen[bx] && pa.hl.on && !pa.hl.sel.IsCollapsed() {
			seen[bx] = true
			pieces = append(pieces, selPiece{bx, pa.hl.sel})
		}
		return true
	})
	dt := &DataTransfer{EffectAllowed: DropCopy}
	dt.SetText(txt)
	ev := DragEvent{X: u.Pos.X, Y: u.Pos.Y, Button: MouseButtonLeft}
	ev.Shift, ev.Ctrl, ev.Alt, ev.Meta = modifiers(ps.mods)
	src := &dragSource{el: el, dt: dt, sel: pieces}
	if !t.startDrag(ps, src, ev, press) {
		t.flushDiscrete()
		if root := s.root; ps.pressing && root != nil && t.selRoot(root) == root {
			if pos, ok := t.posAt(root, press); ok {
				s.unit = selChar
				t.setSel(root, pos, pos)
				s.drag = ps
				t.selDragTo(ps)
			}
		}
		return
	}
	t.flushDiscrete()
}

// selDragImage returns the drag image of the dragged static selection
// pieces at the window's scale s, and the hotspot hot (device pixels of
// the window) in its coordinates: the selected text alone in its own
// colors at dragImageAlpha, as a field's (textDragImage) and Chromium's
// DragImageForSelection paint it, each paragraph through its transforms
// as one matrix (paraTextMatrix: a rotated container drags rotated text,
// ADR-272), in the bounding box of the transformed rectangles.
func (t *Tree) selDragImage(pieces []selPiece, s float64, hot image.Point) (*image.RGBA, image.Point) {
	type placed struct {
		lay   *text.Layout
		m     gfx.Affine
		rects []text.LineRect
		g     float32
	}
	var ps []placed
	var b gfx.Rect
	for _, pc := range pieces {
		bx := pc.bx
		if !paraBox(bx) || bx.pcs == nil || !t.inTree(bx) {
			continue
		}
		lay := bx.para.lay
		rects := lay.HighlightLineRects(pc.sel, false, text.AllLayers, nil)
		if len(rects) == 0 {
			continue
		}
		m := t.paraTextMatrix(bx)
		for _, r := range rects {
			b = b.Union(m.TransformRect(r.Rect))
		}
		ps = append(ps, placed{lay, m, rects, 2*max(bx.para.em, 1) + 1})
	}
	if len(ps) == 0 {
		return nil, image.Point{}
	}
	pr := dragImageRect(dragPixels(b, s), hot)
	if pr.Empty() {
		return nil, image.Point{}
	}
	bl := gfx.NewBuilder()
	bl.PushLayer(dragImageAlpha, gfx.SrcOver)
	for _, p := range ps {
		bl.PushTransform(gfx.Translate(float32(-float64(pr.Min.X)/s), float32(-float64(pr.Min.Y)/s)).Mul(p.m))
		for _, r := range p.rects {
			bl.PushClipRect(gfx.Rect{X0: r.X0, Y0: r.Y0 - p.g, X1: r.X1, Y1: r.Y1 + p.g})
			p.lay.PaintLinesWith(bl, gfx.Point{}, r.Line, r.Line+1, float32(s), nil)
			bl.PopClip()
		}
		bl.PopTransform()
	}
	bl.PopLayer()
	img := image.NewRGBA(image.Rect(0, 0, pr.Dx(), pr.Dy()))
	raster.Render(img, bl.Finish(), raster.Options{Scale: float32(s), Workers: 1})
	return img, hot.Sub(pr.Min)
}

// ---------------------------------------------------------------------
// Keyboard context menu.

// selHasFocus reports whether the static selection has the focus, so that
// a context menu asked for with the keyboard opens at it (Chromium's
// FrameSelection::SelectionHasFocus): it is a range, and nothing is
// focused, or the focused element (not a text field, whose selection is
// its own) contains the range's start, or the range contains the start of
// the focused element.
func (t *Tree) selHasFocus() bool {
	s := &t.tsel
	if !s.active() || s.anchor == s.focus || !t.posValid(s.anchor, s.root) || !t.posValid(s.focus, s.root) {
		return false
	}
	f := t.foc.el
	if f == nil || !f.mounted {
		return true
	}
	if isTextFieldEl(f) {
		return false
	}
	start, end := t.selOrdered()
	if isInside(selPosElement(start), f) {
		return true
	}
	fb := t.geomBox(f)
	if fb == nil || t.selRoot(fb) != s.root {
		return false
	}
	fp, ok := t.firstPosIn(fb)
	if !ok {
		return false
	}
	return t.cmpPos(start, fp) <= 0 && t.cmpPos(fp, end) <= 0
}

// selPosElement returns the element whose text holds position p: an
// inline element of its paragraph, or the paragraph's.
func selPosElement(p selPos) *Element {
	if el := p.bx.para.targetAt(p.cur.Offset); el != nil && el.mounted {
		return el
	}
	return p.bx.owner
}

// firstPosIn returns the first position in the subtree of bx: the start
// of its first paragraph (selectable or not), else the nearest one after
// it (Chromium's FirstPositionInNode, canonicalized).
func (t *Tree) firstPosIn(bx *Box) (selPos, bool) {
	var walk func(b *Box) *Box
	walk = func(b *Box) *Box {
		if b.ls.Display == layout.DisplayNone {
			return nil
		}
		if paraBox(b) && !b.para.empty {
			return b
		}
		for _, k := range b.kids {
			if r := walk(k); r != nil {
				return r
			}
		}
		return nil
	}
	if p := walk(bx); p != nil {
		s, _ := docBounds(p)
		return selPos{p, s}, true
	}
	return t.nearestPos(t.tsel.root, bx)
}

// selMenuPoint returns where a context menu asked for with the keyboard
// opens at the selection, in window coordinates, when it has the focus
// (Chromium's ShowNonLocatedContextMenu): the middle of the caret at the
// range's start, or, when a focused element's visible rectangle cuts it
// away, at its end, else the middle of the part of the box of both that
// the element shows.
func (t *Tree) selMenuPoint() (gfx.Point, bool) {
	if !t.selHasFocus() {
		return gfx.Point{}, false
	}
	start, end := t.selOrdered()
	caretMid := func(p selPos) gfx.Point {
		c := p.bx.para.lay.Caret(p.cur)
		r := t.paraToWindow(p.bx, gfx.Rect{X0: c.X, Y0: c.Top, X1: c.X, Y1: c.Top + c.Height})
		return gfx.Point{X: r.X0, Y: (r.Y0 + r.Y1) / 2}
	}
	sp, ep := caretMid(start), caretMid(end)
	rect := gfx.Rect{X0: min(sp.X, ep.X), Y0: min(sp.Y, ep.Y), X1: max(sp.X, ep.X), Y1: max(sp.Y, ep.Y)}
	if f := t.foc.el; f != nil && f.mounted {
		if c := t.elemBounds(f); !c.Empty() {
			rect = gfx.Rect{X0: max(rect.X0, c.X0), Y0: max(rect.Y0, c.Y0), X1: min(rect.X1, c.X1), Y1: min(rect.Y1, c.Y1)}
		}
	}
	in := func(p gfx.Point) bool { return p.X >= rect.X0 && p.X <= rect.X1 && p.Y >= rect.Y0 && p.Y <= rect.Y1 }
	switch {
	case in(sp):
		return sp, true
	case in(ep):
		return ep, true
	}
	return gfx.Point{X: (rect.X0 + rect.X1) / 2, Y: (rect.Y0 + rect.Y1) / 2}, true
}

// selContextMenu is the default action of the ContextMenu key and
// Shift+F10 with the selection having the focus and no handler that took
// the key (an aria context menu area around the focus takes it): an
// OnContextMenu at the selection (selMenuPoint), on the focused element,
// else on the element of the selection's start, with ClickCount 0 and
// KeyboardContextMenu set. It reports whether it fired one.
func (t *Tree) selContextMenu(e KeyboardEvent) bool {
	p, ok := t.selMenuPoint()
	if !ok {
		return false
	}
	target := t.foc.el
	if target == nil || !target.mounted {
		start, _ := t.selOrdered()
		target = selPosElement(start)
	}
	if target == nil || !target.mounted {
		return false
	}
	me := MouseEvent{X: p.X, Y: p.Y, Button: MouseButtonRight, Shift: e.Shift}
	dispatchWith(t, evContextMenu, target, me, true, true, &eventState{keyMenu: true})
	t.flushDiscrete()
	return true
}

// isMenuKey reports the keys that ask for a context menu: ContextMenu and
// Shift+F10, without Ctrl, Alt or Meta.
func isMenuKey(e KeyboardEvent) bool {
	if e.Ctrl || e.Alt || e.Meta {
		return false
	}
	return e.Key == KeyValue(platform.KeyContextMenu) || e.Key == KeyValue(platform.KeyF10) && e.Shift
}

// ---------------------------------------------------------------------
// For package aria.

// NoTextSelection makes the press of pointerdown e leave the selection of
// static text as it is and start none, as React Aria's usePress does by
// setting user-select: none on the pressed element (its
// allowTextSelectionOnPress). For package aria.
func NoTextSelection(e PointerEvent) {
	if e.st != nil {
		e.st.noSelect = true
	}
}

// KeyboardContextMenu reports whether contextmenu event e was asked for
// with the keyboard at the selection of static text (ADR-271), so that
// the menu focuses its first item. For package aria.
func KeyboardContextMenu(e MouseEvent) bool { return e.st != nil && e.st.keyMenu }

// SelectionPoint returns where a context menu asked for with the keyboard
// opens at the static selection of the window r's element is in, and
// false when the selection does not have the focus (selMenuPoint). For
// package aria.
func SelectionPoint(r ElementRef) (Point, bool) {
	el := r.element()
	if el == nil {
		return Point{}, false
	}
	p, ok := el.tree.selMenuPoint()
	return Point{X: p.X, Y: p.Y}, ok
}
