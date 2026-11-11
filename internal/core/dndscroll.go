package core

import (
	"time"

	"github.com/kuronosu/tela/internal/gfx"
)

// The autoscroll of a drag and drop over the window (ADR-265, ADR-272),
// as Chromium's AutoscrollController does it (kAutoscrollForDragAndDrop,
// UpdateDragAndDrop and Animate; LayoutBox::FindAutoscrollable,
// CalculateAutoscrollDirection and Autoscroll), whatever the drag carries
// and whether a target accepts it:
//
//   - What scrolls is the autoscrollable of the element under the drag:
//     the element itself when it is a text field whose text overflows
//     (fielddnd.go), else the nearest box up the box tree whose scroll
//     container overflows in an axis the user can scroll (overflow auto or
//     scroll; Chromium's IsUserScrollable), the root included; none when a
//     box fixed to the window comes first (Chromium does not autoscroll
//     from inside a position: fixed element).
//   - Its band is 20 px (kAutoscrollBeltSize) inside its border box
//     without the scrollbar gutters (a field's: fielddnd.go), on both axes:
//     a drag in it moves the point to reveal 20 px past the pointer toward
//     the edges it is near. Outside the band nothing scrolls, also when an
//     outer container's band holds the pointer: Chromium looks no further
//     than the innermost autoscrollable.
//   - From 0.2 s (kAutoscrollDelay) after the drag entered the band of
//     that autoscrollable, every frame scrolls the 1 px square at that
//     point into view, to the edge, at once (ScrollRectToVisible with
//     ToEdgeIfNeeded and a user scroll): in the autoscrollable, then in the
//     scroll containers of its containing-block chain, each in the axes the
//     user can scroll, the point moving with the content each one
//     scrolled. So a container scrolls 20 - d px a frame toward its start
//     and 21 - d toward its end from a pointer d px inside the edge, and,
//     once it is at its extent, the containers around it scroll only when
//     the point is past their own edges (it sits at their edge).
//   - Frames run only while a step moves something: the delay is a
//     deadline (NextDeadline).
//
// The element under the drag, and with it the autoscrollable, follow the
// platform's drag events: content that scrolls under a resting drag is hit
// again at the next one.

// Chromium's autoscroll of a drag and drop (AutoscrollController,
// LayoutBox::CalculateAutoscrollDirection).
const (
	dragAutoscrollBelt  = 20                     // kAutoscrollBeltSize, logical px
	dragAutoscrollDelay = 200 * time.Millisecond // kAutoscrollDelay
)

// dragAutoscrollTo follows a move of the drag over the window at now (see
// the comment at the top of dndscroll.go): the autoscrollable of the
// element under it, when the drag is in its band, is what scrolls, and the
// point to scroll into view the drag's position moved by the band toward
// the edges it is near. The delay starts when the drag enters the band of
// an autoscrollable, and again when the autoscrollable changes.
func (t *Tree) dragAutoscrollTo(now time.Duration) {
	d := &t.dnd
	t.scr.now = max(t.scr.now, now)
	f, s := t.dragAutoscrollable(d.target)
	var off gfx.Point
	switch {
	case f != nil:
		off = t.fieldAutoscrollDir(f, d.pos)
	case s != nil:
		off = autoscrollBand(scrollingBox(s.box), t.boxPoint(s.box, d.pos))
	}
	if off == (gfx.Point{}) {
		d.as, d.asS = nil, nil
		return
	}
	d.asRef, d.asDir = d.pos.Add(off), off
	if d.as != f || d.asS != s {
		d.as, d.asS, d.asAt = f, s, now
	}
	if t.dragAutoscrollDue() {
		t.requestRepaint()
	}
}

// dragAutoscrollable returns what a drag over target autoscrolls: target's
// text field when its text overflows, or the scroll container of the
// nearest box up the box tree that the user can scroll and whose content
// overflows (see the comment at the top of dndscroll.go); nil and nil for
// none.
func (t *Tree) dragAutoscrollable(target *Element) (*field, *scroller) {
	if target == nil || !target.mounted {
		return nil, nil
	}
	if f := textField(target); f != nil && f.alive() && f.fieldOverflows() {
		return f, nil
	}
	for b := t.geomBox(target); b != nil; b = b.parent {
		// A field's own scroller is the field's (above).
		if s := b.scr; s != nil && b.fld == nil && b.pcs != nil && userOverflows(s) {
			return nil, s
		}
		if b == t.rootBox || fixedToWindow(b, containingBlock(b)) {
			break
		}
	}
	return nil, nil
}

// userOverflows reports whether the user can scroll s in an axis its
// content overflows (Chromium's LayoutBox::IsUserScrollable).
func userOverflows(s *scroller) bool {
	return s.userX && s.max.X > 0 || s.userY && s.max.Y > 0
}

// scrollingBox returns the border box of bx without its scrollbar
// gutters, in the coordinates of its border box (Chromium's
// AbsoluteBoundingBoxRect with ExcludeScrollbars): the box the band of a
// drag's autoscroll is inside.
func scrollingBox(bx *Box) gfx.Rect {
	l := bx.ld.Layout()
	r := gfx.Rect{X1: l.Size.W - l.ScrollbarSize.W, Y1: l.Size.H - l.ScrollbarSize.H}
	if s := bx.scr; s != nil && s.rtl {
		// The vertical scrollbar's gutter is on the left.
		r = r.Translate(l.ScrollbarSize.W, 0)
	}
	return r
}

// autoscrollBand returns the offset by which the point p moves toward the
// edges of box's band it is in, dragAutoscrollBelt inside box on each
// axis, or zero outside the band.
func autoscrollBand(box gfx.Rect, p gfx.Point) gfx.Point {
	const b = dragAutoscrollBelt
	var off gfx.Point
	switch {
	case p.X < box.X0+b:
		off.X = -b
	case p.X > box.X1-b:
		off.X = b
	}
	switch {
	case p.Y < box.Y0+b:
		off.Y = -b
	case p.Y > box.Y1-b:
		off.Y = b
	}
	return off
}

// dragAutoscrollStep runs one autoscroll step of the drag when apply is
// set, else only looks at it, and reports whether it moves something:
// the autoscrollable scrolls the point into view, then the scroll
// containers of its containing-block chain (see the comment at the top of
// dndscroll.go).
func (t *Tree) dragAutoscrollStep(apply bool) bool {
	d := &t.dnd
	if !d.active {
		return false
	}
	q := d.asRef
	switch {
	case d.as != nil:
		f := d.as
		x, p, ok := t.fieldAutoscrollStep(f, d.asRef, d.asDir)
		if !f.alive() {
			return false
		}
		if ok {
			// The text at the point moves with the field.
			if f.multi {
				s := f.bx.scr
				q = q.Sub(s.flip(p).Sub(s.flip(s.pos)))
			} else {
				q.X -= x - f.scrollX
			}
		}
		if ok && apply {
			if f.multi {
				t.scrollDirect(f.bx.scr, p)
			} else {
				f.scrollX = x
				f.markPaint()
			}
		}
		return t.autoscrollChain(f.bx, false, q, apply) || ok
	case d.asS != nil:
		s := d.asS
		if el := s.box.el; el == nil || !el.mounted || s.box.scr != s || s.box.pcs == nil {
			return false
		}
		return t.autoscrollChain(s.box, true, q, apply)
	}
	return false
}

// autoscrollChain scrolls the 1 px square at the window point q into
// view, to the nearest edge, at once and in the axes the user can scroll:
// in bx's scroll container when self is set, then in those of bx's
// containing-block chain, up to a box fixed to the window (Chromium's
// ScrollRectToVisible from LayoutBox::Autoscroll). q moves with the
// content each container scrolls (transforms between them are ignored).
// With apply unset it only looks. It reports whether a container moves.
func (t *Tree) autoscrollChain(bx *Box, self bool, q gfx.Point, apply bool) bool {
	moved := false
	for b := bx; b != nil; {
		if s := b.scr; s != nil && (self || b != bx) && (s.userX || s.userY) && b.pcs != nil {
			if to := t.autoscrollTarget(s, q); to != s.pos {
				moved = true
				q = q.Sub(s.flip(to).Sub(s.flip(s.pos)))
				if apply {
					t.scrollDirect(s, to)
				}
			}
		}
		cb := containingBlock(b)
		if cb == nil || fixedToWindow(b, cb) {
			break
		}
		b = cb
	}
	return moved
}

// autoscrollTarget returns the offset of s that brings the 1 px square at
// the window point q (its top-left corner) into its client area, moving
// only the axes the user can scroll, within its extents.
func (t *Tree) autoscrollTarget(s *scroller, q gfx.Point) gfx.Point {
	bx := s.box
	l := bx.ld.Layout()
	lp := t.boxPoint(bx, q)
	c := clientArea(bx, gfx.Rect{X1: l.Size.W, Y1: l.Size.H})
	var d gfx.Point
	if s.userX {
		d.X = nearest(lp.X, lp.X+1, c.X0, c.X1)
	}
	if s.userY {
		d.Y = nearest(lp.Y, lp.Y+1, c.Y0, c.Y1)
	}
	return s.clamp(s.pos.Add(s.flip(d)))
}

// dragAutoscrollDue reports whether the drag's autoscroll moves something
// in the next frame: past the delay, with a step that moves it.
func (t *Tree) dragAutoscrollDue() bool {
	d := &t.dnd
	if d.as == nil && d.asS == nil || t.scr.now < d.asAt+dragAutoscrollDelay {
		return false
	}
	return t.dragAutoscrollStep(false)
}

// dragAutoscrollDeadline returns when the drag's autoscroll delay ends,
// while it waits for it with a step that would move something.
func (t *Tree) dragAutoscrollDeadline() (time.Duration, bool) {
	d := &t.dnd
	if d.as == nil && d.asS == nil || t.scr.now >= d.asAt+dragAutoscrollDelay {
		return 0, false
	}
	if !t.dragAutoscrollStep(false) {
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
	t.dragAutoscrollStep(true)
	if f := t.dnd.caret; f != nil && f.alive() {
		if c, ok := t.dropCursor(f); ok {
			t.setDropCaret(f, c)
		}
	}
}
