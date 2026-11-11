package core

import (
	"image"
	"image/draw"
	"math"
	"slices"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/text"
	"github.com/kuronosu/tela/internal/text/edit"
)

// Drag and drop (ARCHITECTURE section 14; ADR-209, ADR-211) follows
// HTML's model on top of the platform's (platform.DragEvent,
// Window.SetDropEffect, Window.StartDrag): the same within a window,
// between the app's windows and with other applications, since every
// drag goes through the OS.
//
// # Targets
//
// A drag over a window is a session of platform.DragEvents (HandleDrag).
// The element under the drag is hit-tested like a pointer's (hit.go:
// pointer-events, visibility, inert subtrees, the active modal overlay
// and clips), and the events are HTML's, with its order:
//
//   - when the element under the drag changes, dragenter fires at the new
//     one and then dragleave at the old one (an unmounted old one: its
//     nearest mounted ancestor, so a counter of enters and leaves on an
//     ancestor stays balanced); every move fires dragover at the current
//     one; leaving the window fires dragleave; a drop fires drop. All four
//     bubble and stop with StopPropagation; disabled elements and their
//     descendants run none (as for pointers).
//   - Before the drop only the types are known: DataTransfer is in HTML's
//     protected mode. Its DropEffect starts at the platform's choice for
//     the allowed effects and the modifiers (DefaultDropEffect).
//   - The drop is accepted when the latest dragover called
//     PreventDefault: the effect is the DataTransfer's DropEffect when the
//     source allows it (a set is narrowed with DefaultDropEffect), else
//     none. Core answers the platform after every event
//     (Window.SetDropEffect), naming the types the drop reads: the
//     well-known types offered (text, HTML, PNG, the URI list) and the
//     custom types the handlers looked up before the drop (Has, Get...).
//   - drop carries the data; the effect the window reports to the source
//     is the DataTransfer's DropEffect after the handlers (DropNone: the
//     data was not taken). PreventDefault is not needed there to accept,
//     only to cancel the one default action, below.
//   - Text fields take text by default, as HTML's do (ADR-253): over an
//     editable Input or Textarea a drag that offers text/plain is
//     accepted without a handler, and its drop, unless a drop handler
//     called PreventDefault, focuses the field, puts the caret under the
//     drop point and inserts the text as one insertFromDrop edit
//     (OnBeforeInput, OnChange), which it selects. A read-only or
//     disabled field takes none. While it accepts the drag, the field
//     paints a drop caret where the text would go; a field's own
//     selection is a drag source too (fielddnd.go).
//   - The OS owns the pointer while a drag is over the window: at the
//     session's start the tree's pointers are cancelled and leave, so
//     hover and active states end; the next pointer event after the drag
//     brings them back.
//   - A window blocked by a modal child window refuses every drag.
//   - A drag that hovers near the edges of a scroll container, or of a
//     text field whose text overflows, scrolls it, as Chromium does
//     (dndscroll.go).
//
// # Sources
//
// An element with Draggable(true) takes part in the gesture arena with a
// drag recognizer (input.go), like one with drag handlers. When its drag
// gesture starts, OnDragStart gets an empty, writable DataTransfer whose
// EffectAllowed is DropAll. If the handlers put data in it and did not
// call PreventDefault (and left some effect allowed), core starts an OS
// drag of the data (Window.StartDrag), with the element as it is on
// screen as the drag image (a crop of the window's last frame; the
// DataTransfer's SetDragImage replaces it). The OS takes the pointer: the
// press is cancelled (its OnDrag and OnDragEnd are not dispatched), and
// the source's OnDragEnd runs when the platform reports the outcome
// (DragDoneEvent), with DataTransfer.DropEffect the effect performed.
// When the platform cannot start the drag, OnDragEnd runs at once with
// DropNone (a dev warning says why). Without data, or after
// PreventDefault, the press stays the drag gesture: OnDrag and OnDragEnd
// follow as for an element without Draggable, with a nil DataTransfer.
// OnDrag does not fire during a drag and drop (the OS owns the pointer).
// A text field's selection is a source the same way, without Draggable:
// its OnDragStart gets the selected text (fielddnd.go).

// Draggable makes a primitive the source of a drag and drop, like HTML's
// draggable attribute: its drag gesture gives OnDragStart a DataTransfer
// to fill (see dnd.go). Draggable(false) is the default.
func Draggable(on bool) Prop {
	return Prop{name: "Draggable", apply: func(d *desc) { d.draggable = on }}
}

// dndState is the drag-and-drop state of a Tree (UI thread).
type dndState struct {
	// The session of a drag over the window (target side).
	active  bool
	types   []string
	allowed DropEffect
	mods    platform.Modifiers
	pos     gfx.Point
	// target is the element under the drag, whose dragenter fired, and
	// effect the answer of its latest dragover.
	target *Element
	effect DropEffect
	// wanted are the offered custom types the handlers looked up.
	wanted []string
	// fromApp: the drag was started by this app (platform.DragEvent).
	fromApp bool
	// caret is the field that paints a drop caret, at caretAt
	// (fielddnd.go).
	caret   *field
	caretAt text.Cursor
	// as is the field the drag autoscrolls, or asS the scroll container
	// (dndscroll.go), since asAt; asRef is the window point a step
	// scrolls into view, the drag's position moved by asDir.
	as           *field
	asS          *scroller
	asAt         time.Duration
	asRef, asDir gfx.Point

	// src is the drag the tree started, until its DragDoneEvent.
	src *dragSource
}

// dragSource is a drag and drop started from a Draggable element, or
// from a text field's selection (text, fielddnd.go), or from the
// selection of static text (sel, the parts of its paragraphs;
// textseldnd.go).
type dragSource struct {
	el   *Element
	dt   *DataTransfer
	pos  gfx.Point
	mods platform.Modifiers
	text *textDrag
	sel  []selPiece
}

// HandleDrag dispatches a platform drag event over the tree's window (see
// the comment at the top of dnd.go) and returns the window's answer: the
// effect a drop would have, or the final effect of a drop, and the types
// the drop reads. A Host passes them to Window.SetDropEffect, except
// after DragLeave. Call it on the UI thread.
func (t *Tree) HandleDrag(e platform.DragEvent) (effect DropEffect, types []string) {
	d := &t.dnd
	switch e.Action {
	case platform.DragEnter:
		if d.active {
			t.endDragSession() // a session whose end never came
		}
		t.beginDragSession(e)
		return t.dragOver(e), t.dropTypes()
	case platform.DragOver:
		if !d.active {
			t.beginDragSession(e) // the enter never came
		}
		return t.dragOver(e), t.dropTypes()
	case platform.DragLeave:
		if d.active {
			t.endDragSession()
		}
		return DropNone, nil
	case platform.DragDrop:
		if !d.active {
			t.beginDragSession(e)
			t.dragOver(e)
		}
		return t.drop(e)
	}
	return DropNone, nil
}

// beginDragSession starts the session of a drag over the window: the OS
// owns the pointer now, so the pointers the tree follows are cancelled
// and leave (hover and active states end).
func (t *Tree) beginDragSession(e platform.DragEvent) {
	t.cancelPointers(e.Time)
	t.dnd.active = true
	t.dnd.types = slices.Clone(e.Types)
	t.dnd.target = nil
	t.dnd.wanted = nil
}

// noteDrag takes the position, the modifiers and the allowed effects of
// e into the session.
func (t *Tree) noteDrag(e platform.DragEvent) {
	d := &t.dnd
	d.pos = gfx.Point{X: e.Pos.X, Y: e.Pos.Y}
	d.mods, d.allowed, d.fromApp = e.Mods, e.Allowed&DropAll, e.FromApp
}

// dragOver moves the drag to e's position: dragenter and dragleave when
// the element under it changed, then dragover, whose PreventDefault
// accepts the drop. It returns the accepted effect.
func (t *Tree) dragOver(e platform.DragEvent) DropEffect {
	t.noteDrag(e)
	d := &t.dnd
	target, _ := t.hitTest(d.pos)
	if target != d.target {
		old := d.target
		d.target = target
		if target != nil {
			fire(t, evDragEnter, target, t.dndEvent(t.protectedTransfer()), true)
		}
		if old = mountedAncestor(old); old != nil {
			fire(t, evDragLeave, old, t.dndEvent(t.protectedTransfer()), true)
		}
	}
	d.effect = DropNone
	if d.target == nil || !d.target.mounted {
		t.setDropCaret(nil, text.Cursor{})
		d.as, d.asS = nil, nil
		return DropNone
	}
	dt := t.protectedTransfer()
	st := fire(t, evDragOver, d.target, t.dndEvent(dt), true)
	f := t.dropField(d.target)
	if st.defaultPrevented() || f != nil {
		// A text field accepts text by default, as HTML's do.
		d.effect = acceptedEffect(dt.DropEffect, d.allowed, d.mods)
	}
	// The drop caret shows where the field's default action would put
	// the text; a handler that takes the drag over hides it (Chromium).
	var c text.Cursor
	ok := f != nil && !st.defaultPrevented() && d.effect != DropNone
	if ok {
		c, ok = t.dropCursor(f)
	}
	if !ok {
		f = nil
	}
	t.setDropCaret(f, c)
	t.dragAutoscrollTo(e.Time)
	return d.effect
}

// dropField returns the field the drag's text would be dropped into by
// default: target is an editable text field (not read-only, not
// disabled), the drag offers plain text and is not over the text it
// drags (fielddnd.go); else nil.
func (t *Tree) dropField(target *Element) *field {
	f := textField(target)
	if f == nil || !f.alive() || f.readOnly() || !slices.Contains(t.dnd.types, MIMEText) {
		return nil
	}
	if td := t.ownTextDrag(); td != nil && td.f == f && td.intact() && t.inSelection(f, t.dnd.pos, td.start, td.end) {
		return nil
	}
	return f
}

// fieldDrop runs the default action of a drop of text into field f at the
// window point p: the field takes the focus, the caret goes under the drop
// point, and the text is inserted as one insertFromDrop edit (OnBeforeInput,
// cancelable, then OnChange), line breaks as spaces in a single-line
// field, as a paste, and selected. With move, the drag of a field's
// selection of this tree moved by the drop, the dragged text is deleted
// from its field first (fielddnd.go); within one field the deletion and
// the insertion are one undo step.
func (t *Tree) fieldDrop(f *field, p gfx.Point, s string, move *textDrag) {
	if s == "" {
		return
	}
	t.setFocus(f.el, focusMouse)
	if !f.alive() || t.foc.el != f.el {
		return
	}
	if f.ed.Composing() {
		t.finishComposition(f)
		t.ime.reset = true
		if !f.alive() {
			return
		}
	}
	if lay := f.layout(); lay != nil {
		pt := t.fieldPoint(f, p)
		r := f.ed.Do(edit.Command{Kind: edit.ClickAt, X: pt.X, Y: pt.Y}, lay)
		t.fieldChangedBy(f, r, false)
	}
	join := false
	if move != nil {
		join = t.moveDragged(move, f)
		if !f.alive() || t.foc.el != f.el {
			return
		}
	}
	t.fieldDo(f, edit.Command{Kind: edit.Drop, Text: s, JoinUndo: join})
}

// drop fires drop at the element under the drag with the data e carries,
// ends the session, and returns the final effect and the types read.
func (t *Tree) drop(e platform.DragEvent) (DropEffect, []string) {
	d := &t.dnd
	t.noteDrag(e)
	types := t.dropTypes()
	dt := &DataTransfer{
		DropEffect:    d.effect,
		EffectAllowed: d.allowed,
		data:          e.Data.Clone(),
		mode:          dtReadOnly,
	}
	effect := DropNone
	if tg := mountedAncestor(d.target); tg != nil {
		st := fire(t, evDrop, tg, t.dndEvent(dt), true)
		effect = acceptedEffect(dt.DropEffect, d.allowed, d.mods)
		if f := t.dropField(tg); f != nil && !st.defaultPrevented() && effect != DropNone {
			var move *textDrag
			if effect == DropMove {
				move = t.ownTextDrag()
			}
			t.fieldDrop(f, d.pos, dt.Text(), move)
		}
	}
	t.resetDragSession()
	t.flushDiscrete()
	return effect, types
}

// endDragSession ends the session over the window: dragleave at the
// element under the drag.
func (t *Tree) endDragSession() {
	if tg := mountedAncestor(t.dnd.target); tg != nil {
		fire(t, evDragLeave, tg, t.dndEvent(t.protectedTransfer()), true)
	}
	t.resetDragSession()
	t.flushDiscrete()
}

// resetDragSession forgets the session over the window; the drop caret
// goes away.
func (t *Tree) resetDragSession() {
	t.setDropCaret(nil, text.Cursor{})
	src := t.dnd.src
	t.dnd = dndState{src: src}
}

// protectedTransfer returns the DataTransfer of an event before the drop:
// the types only, the effect the platform's convention picks.
func (t *Tree) protectedTransfer() *DataTransfer {
	d := &t.dnd
	return &DataTransfer{
		DropEffect:    t.defaultDropEffect(),
		EffectAllowed: d.allowed,
		types:         d.types,
		mode:          dtProtected,
		look:          t.dndLook,
	}
}

// defaultDropEffect returns the effect the drag over the window starts
// with: the platform's convention for the allowed effects and the
// modifiers, except for the drag of a field's selection of the app, which
// moves unless the copy modifier is held (fielddnd.go).
func (t *Tree) defaultDropEffect() DropEffect {
	d := &t.dnd
	if d.fromApp && t.appTextDrag() != nil {
		if copyModifier(t.flavor, d.mods) && d.allowed.Has(DropCopy) {
			return DropCopy
		}
		if d.allowed.Has(DropMove) {
			return DropMove
		}
	}
	return platform.DefaultDropEffect(d.allowed, d.mods)
}

// dndLook records a type a handler looked up before the drop, when the
// source offers it: the drop reads it.
func (t *Tree) dndLook(ty string) {
	d := &t.dnd
	if d.active && !wellKnown(ty) && slices.Contains(d.types, ty) && !slices.Contains(d.wanted, ty) {
		d.wanted = append(d.wanted, ty)
	}
}

// dropTypes returns the types the drop reads, in the source's order: the
// well-known ones and those the handlers looked up.
func (t *Tree) dropTypes() []string {
	d := &t.dnd
	var out []string
	for _, ty := range d.types {
		if wellKnown(ty) || slices.Contains(d.wanted, ty) {
			out = append(out, ty)
		}
	}
	return out
}

// dndEvent returns the event of a drop target's handlers at the drag's
// position.
func (t *Tree) dndEvent(dt *DataTransfer) DragEvent {
	d := &t.dnd
	e := DragEvent{X: d.pos.X, Y: d.pos.Y, DataTransfer: dt}
	e.Shift, e.Ctrl, e.Alt, e.Meta = modifiers(d.mods)
	return e
}

// acceptedEffect returns the one effect a handler's choice e gives for a
// drag that allows allowed: e when allowed, the platform's pick among
// the effects of a set, none otherwise.
func acceptedEffect(e, allowed DropEffect, mods platform.Modifiers) DropEffect {
	e &= allowed
	switch e {
	case DropNone, DropCopy, DropMove, DropLink:
		return e
	}
	return platform.DefaultDropEffect(e, mods)
}

// RefuseDrag ends the session of a drag over the window, if any: a Host
// calls it instead of HandleDrag while a modal child window blocks the
// window, and answers DropNone.
func (t *Tree) RefuseDrag() {
	if t.dnd.active {
		t.endDragSession()
	}
}

// ---------------------------------------------------------------------
// Sources.

// dragStart runs OnDragStart of the Draggable element el, whose gesture
// started a drag (e, pressed at press), with a DataTransfer to fill, and
// starts the OS drag of the data it got. When it does, or tries, the
// press is no longer the gesture's (ps.osDrag); without data it stays.
func (t *Tree) dragStart(ps *pointerState, el *Element, e DragEvent, press gfx.Point) {
	t.startDrag(ps, &dragSource{el: el, dt: &DataTransfer{EffectAllowed: DropAll}}, e, press)
}

// startDrag runs OnDragStart of src's element with src's DataTransfer and
// starts the OS drag of the data it holds then (see dragStart). It
// reports whether it did, or tried: false when the handlers called
// PreventDefault, left no data or no effect allowed, or unmounted it.
func (t *Tree) startDrag(ps *pointerState, src *dragSource, e DragEvent, press gfx.Point) bool {
	el, dt := src.el, src.dt
	e.DataTransfer = dt
	st := fire(t, evDragStart, el, e, false)
	if st.defaultPrevented() || len(dt.data) == 0 || dt.EffectAllowed&DropAll == 0 || !el.mounted {
		return false
	}
	dt.mode = dtReadOnly
	src.pos, src.mods = gfx.Point{X: e.X, Y: e.Y}, ps.mods
	opts := platform.DragOptions{Data: dt.data.Clone(), Allowed: dt.EffectAllowed & DropAll}
	opts.Image, opts.Hotspot = t.dragImage(src, press)
	err := platform.ErrUnsupported
	if v := t.wv; v != nil && v.alive() {
		err = v.win.StartDrag(opts)
	}
	ps.osDrag = true
	if err != nil {
		t.warnOnce("dnd-start:"+err.Error(), func() string {
			return "tela: the drag and drop of an h.Draggable element could not start (" + err.Error() + "): its OnDragEnd runs at once with DropNone"
		})
		t.dragDone(src, DropNone)
		return true
	}
	t.dnd.src = src
	return true
}

// HandleDragDone ends the drag the tree started with its outcome: the
// source's OnDragEnd runs with DataTransfer.DropEffect = effect. Call it
// on the UI thread with the window's DragDoneEvent.
func (t *Tree) HandleDragDone(effect DropEffect) {
	src := t.dnd.src
	t.dnd.src = nil
	if src != nil {
		t.dragDone(src, effect&DropAll)
	}
}

// dragDone runs the source's OnDragEnd for the outcome effect; then a
// field's selection that moved and was not deleted by a drop of this tree
// is deleted (HTML's default action of dragend, fielddnd.go).
func (t *Tree) dragDone(src *dragSource, effect DropEffect) {
	if !src.el.mounted {
		return
	}
	src.dt.DropEffect = effect
	e := DragEvent{X: src.pos.X, Y: src.pos.Y, Button: MouseButtonLeft, DataTransfer: src.dt}
	e.Shift, e.Ctrl, e.Alt, e.Meta = modifiers(src.mods)
	fire(t, evDragEnd, src.el, e, false)
	t.flushDiscrete()
	if d := src.text; d != nil && !d.moved && effect == DropMove {
		d.moved = true
		if d.intact() {
			t.deleteDragged(d)
		}
	}
}

// dragImage returns the image of the drag src and its hotspot: the one
// the DataTransfer was given, else the element as it is in the window's
// last frame (its border box, cut by the window), or for the selection of
// a field its text alone (textDragImage), with the press point under the
// pointer.
func (t *Tree) dragImage(src *dragSource, press gfx.Point) (*image.RGBA, image.Point) {
	dt := src.dt
	if dt.imgSet {
		if dt.img == nil {
			return nil, image.Point{}
		}
		b := dt.img.Bounds()
		return cutDragImage(dt.img, b, b.Min.Add(image.Pt(dt.hotspotX, dt.hotspotY)))
	}
	v := t.wv
	if v == nil || !(v.scale > 0) {
		return nil, image.Point{}
	}
	s := v.scale
	hot := image.Pt(devicePx(press.X, s, math.Round), devicePx(press.Y, s, math.Round))
	if src.text != nil {
		return t.textDragImage(src.text, s, hot)
	}
	if src.sel != nil {
		return t.selDragImage(src.sel, s, hot)
	}
	if v.fb == nil {
		return nil, image.Point{}
	}
	pr := dragPixels(t.elemBounds(src.el), s).Intersect(v.fb.Rect)
	if pr.Empty() {
		return nil, image.Point{}
	}
	return cutDragImage(v.fb, pr, hot)
}

// devicePx returns the logical coordinate f in device pixels at scale s,
// rounded by round.
func devicePx(f float32, s float64, round func(float64) float64) int {
	return int(round(float64(f) * s))
}

// dragPixels returns the device pixels at scale s that the logical
// rectangle r touches.
func dragPixels(r gfx.Rect, s float64) image.Rectangle {
	return image.Rect(devicePx(r.X0, s, math.Floor), devicePx(r.Y0, s, math.Floor), devicePx(r.X1, s, math.Ceil), devicePx(r.Y1, s, math.Ceil))
}

// cutDragImage copies the part r of src into a new image at the origin,
// keeping at most platform.MaxDragImageSide pixels on a side (those
// around hot, dragImageRect), and returns it with hot, a point of src, in
// its coordinates.
func cutDragImage(src image.Image, r image.Rectangle, hot image.Point) (*image.RGBA, image.Point) {
	r = dragImageRect(r, hot)
	if r.Empty() {
		return nil, image.Point{}
	}
	dst := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
	draw.Draw(dst, dst.Rect, src, r.Min, draw.Src)
	return dst, hot.Sub(r.Min)
}

// dragImageRect returns r cut to at most platform.MaxDragImageSide pixels
// on a side, keeping those around hot.
func dragImageRect(r image.Rectangle, hot image.Point) image.Rectangle {
	const m = platform.MaxDragImageSide
	if r.Dx() > m {
		x0 := min(max(hot.X-m/2, r.Min.X), r.Max.X-m)
		r.Min.X, r.Max.X = x0, x0+m
	}
	if r.Dy() > m {
		y0 := min(max(hot.Y-m/2, r.Min.Y), r.Max.Y-m)
		r.Min.Y, r.Max.Y = y0, y0+m
	}
	return r
}
