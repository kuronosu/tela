package core

import (
	"image"
	"time"

	"github.com/kuronosu/tela/internal/gesture"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/gfx/raster"
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
// a Draggable element (dnd.go). The drag image is the selected text
// alone, translucent, as Chromium draws it (textDragImage). When
// OnDragStart calls PreventDefault or empties the data, the press selects
// from the press point instead, as a press outside the selection does
// (Chromium extends the selection after a cancelled dragstart). A release
// without a drag puts the caret at the release point (Chromium's
// HandleMouseReleaseEvent).
//
// Touch presses do not drag a selection: no backend reports touch
// pointers yet (Win32 and X11 deliver touches as an emulated mouse, which
// drags as a mouse does), and Window.StartDrag needs a held pointer
// button. With touch input, a long press on the selection should start
// the drag, as on Chromium for Android (ADR-265).
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
// # Autoscroll
//
// While a drag hovers over a text field near its edges, the field
// scrolls toward them, as Chromium's AutoscrollController does for a drag
// and drop (kAutoscrollForDragAndDrop), whatever the drag carries and
// whether the field takes it: the band is kAutoscrollBeltSize, 20 px,
// inside the box that scrolls (an Input's content box, its inner editor,
// where hits in its padding land too; a Textarea's border box without its
// scrollbars; LayoutBox::CalculateAutoscrollDirection), and from 0.2 s
// after the drag entered the band (kAutoscrollDelay; Chromium waits
// strictly longer, a frame's difference) every frame scrolls the point 20
// px past the pointer toward the edge into view (LayoutBox::Autoscroll:
// a 1 px square, to the edge if needed), so a pointer d px inside the
// edge scrolls 20 - d px a frame toward the start, 21 - d toward the end.
// An Input scrolls horizontally, a Textarea on both axes as far as its
// extents go (vertically when its lines wrap). The drop caret follows the
// text that comes under the pointer. Frames run only while a step moves
// the field: the delay is a deadline (NextDeadline), not frames.
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

// dragImageAlpha is the opacity of the drag image of a selection
// (Chromium's kDragImageAlpha).
const dragImageAlpha = 0.75

// textDragImage returns the drag image of the dragged text d at the
// window's scale s, and the hotspot hot (device pixels of the window) in
// its coordinates: the selected text alone, as Chromium's
// DragImageForSelection paints it (PaintFlag::kSelectionDragImageOnly):
// the glyphs of the selection in the text's own colors, without the
// selection highlight or anything of the field around them, at
// dragImageAlpha, over the selection's rectangles not cut by the field
// (AbsoluteUnclippedBounds: the part an Input scrolled out of view is in
// it). The field's transforms scale and move the text as on screen; a
// rotated field's text is drawn upright, scaled to the bounding box of
// the rotated one. It is painted on the UI thread from the field's
// layout, into an image of its own.
func (t *Tree) textDragImage(d *textDrag, s float64, hot image.Point) (*image.RGBA, image.Point) {
	f := d.f
	lay := f.layout()
	if lay == nil || f.bx.pcs == nil {
		return nil, image.Point{}
	}
	sel := text.NewSelection(text.Cursor{Offset: f.ed.ValueToDisplay(d.start)}, text.Cursor{Offset: f.ed.ValueToDisplay(d.end)})
	rects := lay.SelectionRects(sel, nil)
	// The text's coordinates to the window's: a unit square of the text
	// mapped through the field's origin, scroll and transforms.
	const k = 1024
	u := t.fieldToWindow(f, gfx.Rect{X1: k, Y1: k})
	m := gfx.Affine{u.Width() / k, 0, 0, u.Height() / k, u.X0, u.Y0}
	var b gfx.Rect
	for _, r := range rects {
		b = b.Union(m.TransformRect(r))
	}
	pr := dragImageRect(dragPixels(b, s), hot)
	if pr.Empty() {
		return nil, image.Point{}
	}
	bl := gfx.NewBuilder()
	bl.PushLayer(dragImageAlpha, gfx.SrcOver)
	bl.PushTransform(gfx.Translate(float32(-float64(pr.Min.X)/s), float32(-float64(pr.Min.Y)/s)).Mul(m))
	// Each line is painted alone, clipped to its selection rectangle
	// across and grown down and up by twice the font size, so that the
	// glyphs that overhang the line box keep their ink.
	g := 2*f.ts.Size + 1
	for _, r := range rects {
		li := lineAtTop(lay, r.Y0)
		bl.PushClipRect(gfx.Rect{X0: r.X0, Y0: r.Y0 - g, X1: r.X1, Y1: r.Y1 + g})
		lay.PaintLinesWith(bl, gfx.Point{}, li, li+1, float32(s), nil)
		bl.PopClip()
	}
	bl.PopTransform()
	bl.PopLayer()
	img := image.NewRGBA(image.Rect(0, 0, pr.Dx(), pr.Dy()))
	raster.Render(img, bl.Finish(), raster.Options{Scale: float32(s), Workers: 1})
	return img, hot.Sub(pr.Min)
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
// range. It reports whether it deleted the text from target itself: the
// drop then joins the deletion's undo step (one step for a move within a
// field, as Chromium's DragAndDropCommand).
func (t *Tree) moveDragged(d *textDrag, target *field) bool {
	if d.moved {
		return false
	}
	d.moved = true
	if !d.intact() {
		return false
	}
	at, _, _ := target.ed.SelectionRange()
	t.deleteDragged(d)
	if d.f != target || !target.alive() {
		return false
	}
	if target.ed.Value() == d.value {
		// The deletion was cancelled (OnBeforeInput's PreventDefault, or
		// a controlled field that kept its value): the caret goes back to
		// the drop point, so the drop inserts a copy there instead of
		// replacing the dragged range it selected.
		t.fieldChanged(target, target.ed.Select(at, at))
		return false
	}
	if target.ed.Value() != d.value[:d.start]+d.value[d.end:] {
		return false
	}
	switch {
	case at >= d.end:
		at -= d.end - d.start
	case at > d.start:
		at = d.start
	}
	t.fieldChanged(target, target.ed.Select(at, at))
	return true
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

// Chromium's autoscroll of a drag and drop (AutoscrollController,
// LayoutBox::CalculateAutoscrollDirection).
const (
	dragAutoscrollBelt  = 20                     // kAutoscrollBeltSize, logical px
	dragAutoscrollDelay = 200 * time.Millisecond // kAutoscrollDelay
)

// dragAutoscrollTo follows a move of the drag over the window at now
// (see Autoscroll above): the field under it, when the drag is in its
// band, is the one that scrolls, and the point to scroll into view the
// drag's position moved by the band toward the edges it is near. The
// delay starts when the drag enters a field's band.
func (t *Tree) dragAutoscrollTo(now time.Duration) {
	d := &t.dnd
	t.scr.now = max(t.scr.now, now)
	f := textField(d.target)
	var off gfx.Point
	if f != nil && f.alive() {
		off = t.dragAutoscrollDir(f, d.pos)
	}
	if off == (gfx.Point{}) {
		d.as = nil
		return
	}
	d.asRef, d.asDir = d.pos.Add(off), off
	if d.as != f {
		d.as, d.asAt = f, now
	}
	if t.dragAutoscrollDue() {
		t.requestRepaint()
	}
}

// dragAutoscrollDir returns the offset by which the drag's position p
// moves toward the edges of f's band it is in (dragAutoscrollBelt on each
// axis the field scrolls on), or zero outside it.
func (t *Tree) dragAutoscrollDir(f *field, p gfx.Point) gfx.Point {
	l := f.bx.ld.Layout()
	lp := t.localPoint(f.el, p)
	// The box that scrolls, in the field's border box coordinates.
	box := gfx.Rect{X1: l.Size.W - l.ScrollbarSize.W, Y1: l.Size.H - l.ScrollbarSize.H}
	if !f.multi {
		box = contentRect(gfx.Rect{X1: l.Size.W, Y1: l.Size.H}, l)
	}
	const b = dragAutoscrollBelt
	var off gfx.Point
	switch {
	case lp.X < box.X0+b:
		off.X = -b
	case lp.X > box.X1-b:
		off.X = b
	}
	if f.multi {
		switch {
		case lp.Y < box.Y0+b:
			off.Y = -b
		case lp.Y > box.Y1-b:
			off.Y = b
		}
	}
	return off
}

// dragAutoscrollStep returns where one autoscroll step of the drag
// scrolls its field: an Input's scrollX (x) or a Textarea's scroll
// offset (p), and whether it moves the field.
func (t *Tree) dragAutoscrollStep() (x float32, p gfx.Point, ok bool) {
	d := &t.dnd
	f := d.as
	if f == nil || !d.active || !f.alive() || f.layout() == nil {
		return 0, gfx.Point{}, false
	}
	pt := t.fieldPoint(f, d.asRef)
	// The 1 px square at the point, toward the edges it goes past.
	if d.asDir.X > 0 {
		pt.X++
	}
	if d.asDir.Y > 0 {
		pt.Y++
	}
	if f.multi {
		p, ok = t.textareaAutoscroll(f, pt)
		return 0, p, ok
	}
	x = f.scrollX
	switch w := f.textWidth(); {
	case pt.X < x:
		x = pt.X
	case pt.X > x+w:
		x = pt.X - w
	}
	lo, hi := f.scrollXRange()
	x = min(max(x, lo), hi)
	return x, gfx.Point{}, x != f.scrollX
}

// dragAutoscrollDue reports whether the drag's autoscroll moves its field
// in the next frame: past the delay, with a step that moves it.
func (t *Tree) dragAutoscrollDue() bool {
	d := &t.dnd
	if d.as == nil || t.scr.now < d.asAt+dragAutoscrollDelay {
		return false
	}
	_, _, ok := t.dragAutoscrollStep()
	return ok
}

// dragAutoscrollDeadline returns when the drag's autoscroll delay ends,
// while it waits for it with a step that would move the field.
func (t *Tree) dragAutoscrollDeadline() (time.Duration, bool) {
	d := &t.dnd
	if d.as == nil || t.scr.now >= d.asAt+dragAutoscrollDelay {
		return 0, false
	}
	if _, _, ok := t.dragAutoscrollStep(); !ok {
		return 0, false
	}
	return d.asAt + dragAutoscrollDelay, true
}

// tickDragAutoscroll asks for the frames of the drag's autoscroll when
// its delay ended (TickGestures).
func (t *Tree) tickDragAutoscroll() {
	if t.dragAutoscrollDue() {
		t.requestRepaint()
	}
}

// animateDragAutoscroll runs one autoscroll step of the drag (Animate),
// and moves the drop caret to the text now under the drag.
func (t *Tree) animateDragAutoscroll() {
	if !t.dragAutoscrollDue() {
		return
	}
	x, p, _ := t.dragAutoscrollStep()
	f := t.dnd.as
	if f.multi {
		t.scrollDirect(f.bx.scr, p)
	} else {
		f.scrollX = x
		f.markPaint()
	}
	if t.dnd.caret == f {
		if c, ok := t.dropCursor(f); ok {
			t.setDropCaret(f, c)
		}
	}
}

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
