package core

import (
	"github.com/kuronosu/tela/internal/gesture"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/text"
	"github.com/kuronosu/tela/internal/text/edit"
)

// Drag and drop of text fields (ADR-253 and its follow-up), on top of
// dnd.go; Chromium 152 is the reference.
//
// # The selection as a drag source
//
// A primary press of a mouse or a pen inside the selected text of a
// field (a single click without Shift; not a password field, not the Mac
// flavor's Control click) does not move the caret: it may start a drag of
// the selection (Chromium's SelectionController: "don't restart the
// selection when the mouse is pressed on an existing selection so we can
// allow for text dragging"). The press takes part in the gesture arena
// with a drag recognizer of the field (fieldSelPress); past the drag slop
// the field's OnDragStart runs, not bubbling, with a DataTransfer that
// holds the selected text as text/plain and allows copy and move (copy
// only from a read-only field), and core starts an OS drag of it as for
// a Draggable element (dnd.go), with the selected text as it is on
// screen as the drag image. When OnDragStart calls PreventDefault or
// empties the data, the press selects from the press point instead, as
// a press outside the selection does (Chromium extends the selection
// after a cancelled dragstart). A release without a drag puts the caret
// at the release point (Chromium's HandleMouseReleaseEvent).
//
// # Where the text goes
//
// While the app's own selection drag is over a window of the app, the
// drop effect a target starts from (DataTransfer.DropEffect, the field's
// default acceptance) is move, as HTML's drag of a selection in an
// editing host is, and copy while the copy modifier is held: Ctrl on
// Linux and Windows, Option (Alt) under the Mac flavor (ADR-221). A drag
// over its own selected text is refused (Chromium's CanProcessDrag).
//
// A move into a field of the tree that started the drag is done at the
// drop, as Chromium does in one document: deleteByDrag in the source
// field (OnBeforeInput may cancel the deletion; the drop still inserts,
// a copy then), then insertFromDrop at the drop point, moved back by the
// deleted length when it was after the source range in the same field.
// Any other move (a field or a target of another window, another
// application, a target that is not a field) deletes the source range at
// the DragDoneEvent with a move effect, after OnDragEnd (HTML's default
// action of dragend). In both cases the range is deleted only if the
// field's value did not change since the drag started.
//
// # The drop caret
//
// While a field's default acceptance accepts a drag of text (dropField,
// and no dragover handler called PreventDefault: Chromium clears its drag
// caret when the page handles the drag), the field paints a caret where
// the drop would insert, the hit test of the drag's position in its
// text: as wide, tall and colored as the field's caret, steady. The
// field's selection does not change. It goes away when the drag leaves
// the field or the window, drops, or is cancelled.

// fieldSelPress is a press inside a field's selection that may start a
// drag of the selection (see above), until the drag starts or the press
// ends.
type fieldSelPress struct {
	f  *field
	ps *pointerState
}

// textDrag is the selection of a field dragged as an OS drag.
type textDrag struct {
	f *field
	// start and end are the value offsets of the dragged text; value is
	// the field's value when the drag started.
	start, end int
	value      string
	// moved: the move was handled (a drop into a field of the tree that
	// started the drag), or the source must not delete the text anymore.
	moved bool
}

// intact reports whether the dragged text is still where it was: the
// field is mounted and its value did not change since the drag started.
func (d *textDrag) intact() bool {
	return d.f.alive() && !d.f.ed.Composing() && d.f.ed.Value() == d.value
}

// copyModifier reports whether mods hold the modifier that makes a drag
// of text a copy: Option (Alt) under the Mac flavor, Ctrl elsewhere.
func copyModifier(fl platform.Flavor, mods platform.Modifiers) bool {
	if fl == platform.FlavorMac {
		return mods&platform.ModAlt != 0
	}
	return mods&platform.ModCtrl != 0
}

// inSelection reports whether the window point p is over the text of f
// between the value offsets start and end.
func (t *Tree) inSelection(f *field, p gfx.Point, start, end int) bool {
	if start >= end {
		return false
	}
	lay := f.layout()
	if lay == nil {
		return false
	}
	pt := t.fieldPoint(f, p)
	sel := text.NewSelection(text.Cursor{Offset: f.ed.ValueToDisplay(start)}, text.Cursor{Offset: f.ed.ValueToDisplay(end)})
	for _, r := range lay.SelectionRects(sel, nil) {
		if r.Contains(pt) {
			return true
		}
	}
	return false
}

// selectionDraggable reports whether the press of ps on f, which
// otherwise puts the caret, is a press on its selection that may start a
// drag of it (see the comment at the top of fielddnd.go).
func (t *Tree) selectionDraggable(ps *pointerState, f *field) bool {
	if ps.clickCount != 1 || ps.mods&platform.ModShift != 0 || ps.key.kind == platform.PointerTouch ||
		t.flavor == platform.FlavorMac && ps.mods&platform.ModCtrl != 0 || f.ed.Options().Password || f.ed.Composing() {
		return false
	}
	s, e, _ := f.ed.SelectionRange()
	return t.inSelection(f, ps.pos, s, e)
}

// textDragRecognizer returns the drag recognizer of a press on a field's
// selection by ps on target, or nil.
func (t *Tree) textDragRecognizer(ps *pointerState, target *Element) gesture.Recognizer {
	d := t.in.fsel
	if d == nil || d.ps != ps || d.f.el != target {
		return nil
	}
	return &gesture.Drag{OnStart: func(u gesture.DragUpdate) { t.textDragStart(d, u) }}
}

// textDragStart starts the drag of the selection of the press d, whose
// drag gesture started (u).
func (t *Tree) textDragStart(d *fieldSelPress, u gesture.DragUpdate) {
	if t.in.fsel != d {
		return
	}
	t.in.fsel = nil
	ps, f := d.ps, d.f
	t.in.clicks.Reset()
	ps.ctxClick = false
	press := gfx.Point{X: u.Pos.X - u.Offset.X, Y: u.Pos.Y - u.Offset.Y}
	if !f.alive() || f.ed.Composing() {
		return
	}
	s, e, _ := f.ed.SelectionRange()
	txt := f.ed.SelectedText()
	if txt == "" {
		return
	}
	allowed := DropCopy | DropMove
	if f.readOnly() {
		allowed = DropCopy
	}
	dt := &DataTransfer{EffectAllowed: allowed}
	dt.SetText(txt)
	ev := DragEvent{X: u.Pos.X, Y: u.Pos.Y, Button: MouseButtonLeft}
	ev.Shift, ev.Ctrl, ev.Alt, ev.Meta = modifiers(ps.mods)
	src := &dragSource{el: f.el, dt: dt, text: &textDrag{f: f, start: s, end: e, value: f.ed.Value()}}
	if !t.startDrag(ps, src, ev, press) {
		// Cancelled: the press selects from where it was, as one
		// outside the selection does, and the drag extends it (this
		// move's fieldPointerMove, then the next ones).
		t.flushDiscrete()
		if f.alive() && t.foc.el == f.el && ps.pressing && !f.ed.Composing() {
			t.fieldClickAt(f, press, false)
			t.in.fdrag = &fieldDrag{f: f, ps: ps}
		}
		return
	}
	t.flushDiscrete()
}

// fieldClickAt puts the caret of f under the window point p (ClickAt,
// with Shift: extend).
func (t *Tree) fieldClickAt(f *field, p gfx.Point, extend bool) {
	lay := f.layout()
	if lay == nil {
		return
	}
	pt := t.fieldPoint(f, p)
	r := f.ed.Do(edit.Command{Kind: edit.ClickAt, X: pt.X, Y: pt.Y, Extend: extend}, lay)
	t.caretReset(f)
	t.fieldChangedBy(f, r, false) // the caret is under the pointer
	t.fieldSelect(f, true)
}

// textDragRect returns the window rectangle of the dragged text of d (its
// selection rectangles, cut by the field's box), for the drag image.
func (t *Tree) textDragRect(d *textDrag) gfx.Rect {
	f := d.f
	lay := f.layout()
	if lay == nil {
		return gfx.Rect{}
	}
	sel := text.NewSelection(text.Cursor{Offset: f.ed.ValueToDisplay(d.start)}, text.Cursor{Offset: f.ed.ValueToDisplay(d.end)})
	var u gfx.Rect
	for _, r := range lay.SelectionRects(sel, nil) {
		u = u.Union(t.fieldToWindow(f, r))
	}
	return u.Intersect(t.elemBounds(f.el))
}

// appTextDrag returns the selection drag of a field of the app in
// progress, started by this tree or another window's, or nil.
func (t *Tree) appTextDrag() *textDrag {
	if s := t.dnd.src; s != nil && s.text != nil {
		return s.text
	}
	if v := t.wv; v != nil && v.host != nil {
		for _, w := range v.host.views {
			if w.tree != nil && w.tree != t && w.tree.dnd.src != nil && w.tree.dnd.src.text != nil {
				return w.tree.dnd.src.text
			}
		}
	}
	return nil
}

// ownTextDrag returns the selection drag this tree started, or nil.
func (t *Tree) ownTextDrag() *textDrag {
	if s := t.dnd.src; s != nil {
		return s.text
	}
	return nil
}

// moveDragged deletes the dragged text d from its field for a move into
// the field target, before the drop inserts it there (whose caret is at
// the drop point): deleteByDrag, then target's caret moved back by the
// deleted length when it is the source and the drop point was after the
// range.
func (t *Tree) moveDragged(d *textDrag, target *field) {
	if d.moved {
		return
	}
	d.moved = true
	if !d.intact() {
		return
	}
	at, _, _ := target.ed.SelectionRange()
	t.deleteDragged(d)
	if d.f != target || !target.alive() || target.ed.Value() != d.value[:d.start]+d.value[d.end:] {
		return
	}
	switch {
	case at >= d.end:
		at -= d.end - d.start
	case at > d.start:
		at = d.start
	}
	t.fieldChanged(target, target.ed.Select(at, at))
}

// deleteDragged deletes the dragged text d from its field (deleteByDrag:
// OnBeforeInput, which may cancel it, then OnChange). The focused field
// keeps its blinking caret.
func (t *Tree) deleteDragged(d *textDrag) {
	f := d.f
	t.fieldChanged(f, f.ed.Select(d.start, d.end))
	t.fieldDo(f, edit.Command{Kind: edit.DeleteByDrag})
	if ff := t.focusedField(); ff != nil && ff != f {
		t.caretReset(ff)
	}
}

// ---------------------------------------------------------------------
// The drop caret.

// dropCursor returns the position of f's text under the drag, where its
// drop would insert.
func (t *Tree) dropCursor(f *field) (text.Cursor, bool) {
	lay := f.layout()
	if lay == nil {
		return text.Cursor{}, false
	}
	pt := t.fieldPoint(f, t.dnd.pos)
	return lay.HitTest(pt.X, pt.Y), true
}

// setDropCaret shows the drop caret in f at c, or hides it (nil f), and
// repaints the fields it leaves and enters.
func (t *Tree) setDropCaret(f *field, c text.Cursor) {
	d := &t.dnd
	if d.caret == f && (f == nil || d.caretAt == c) {
		return
	}
	if old := d.caret; old != nil && old.alive() {
		old.markPaint()
	}
	d.caret, d.caretAt = f, c
	if f != nil {
		f.markPaint()
	}
}

// dropCaret returns where f paints its drop caret, if it does.
func (t *Tree) dropCaret(f *field) (text.Cursor, bool) {
	d := &t.dnd
	if !d.active || d.caret != f || d.caretAt.Offset > len(f.ed.Display()) {
		return text.Cursor{}, false
	}
	return d.caretAt, true
}
