package cocoa

import (
	"image"
	"slices"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// The pure half of Cocoa drag and drop (dnd_darwin.go): the mapping of
// NSDragOperation masks to the contract's effects in both directions, the
// pasteboard types a view registers for, the drag image's frame, and the
// drop session of a window with the rules of the platform contract. It has
// no build constraint, so it is tested on every OS.

// NSDragOperation bits (NSDragging.h), repeated here because
// internal/darwin exists only on darwin.
const (
	opCopy    uint64 = 1
	opLink    uint64 = 2
	opGeneric uint64 = 4
	opPrivate uint64 = 8
	opMove    uint64 = 16
	opDelete  uint64 = 32
)

// dropEffects returns the effects of a source's operation mask
// (draggingSourceOperationMask): copy, link, and move for move or generic
// (AppKit's generic operation is what a plain drag performs; Finder moves
// a file within a volume with it, and Chromium reads it as a move too).
// Private and delete have no effect of their own. NSDragOperationEvery is
// every effect.
func dropEffects(mask uint64) platform.DropEffect {
	var e platform.DropEffect
	if mask&opCopy != 0 {
		e |= platform.DropCopy
	}
	if mask&(opMove|opGeneric) != 0 {
		e |= platform.DropMove
	}
	if mask&opLink != 0 {
		e |= platform.DropLink
	}
	return e
}

// targetOperation returns the NSDragOperation a destination answers for
// the effect e (DropNone or one effect) of a drag whose source mask is
// mask: copy and link are their bits, and move is NSDragOperationMove when
// the source allows it, else generic (a source that offers only generic
// must get an operation of its mask). 0 when the mask lacks the effect.
func targetOperation(e platform.DropEffect, mask uint64) uint64 {
	switch e {
	case platform.DropCopy:
		return mask & opCopy
	case platform.DropLink:
		return mask & opLink
	case platform.DropMove:
		if mask&opMove != 0 {
			return opMove
		}
		return mask & opGeneric
	}
	return 0
}

// sourceMask returns the operation mask of a drag that allows the effects
// allowed (draggingSession:sourceOperationMaskForDraggingContext:): a move
// is offered as move and generic, as Chromium's drag source does, so that
// destinations that perform the generic operation (Finder within a volume)
// take it.
func sourceMask(allowed platform.DropEffect) uint64 {
	var m uint64
	if allowed.Has(platform.DropCopy) {
		m |= opCopy
	}
	if allowed.Has(platform.DropMove) {
		m |= opMove | opGeneric
	}
	if allowed.Has(platform.DropLink) {
		m |= opLink
	}
	return m
}

// doneEffect returns the effect of the operation a drag of this app ended
// with (draggingSession:endedAtPoint:operation:), DropNone when it was
// cancelled or refused: generic and delete (the Dock's Trash) are moves.
// An effect outside allowed is DropNone.
func doneEffect(op uint64, allowed platform.DropEffect) platform.DropEffect {
	var e platform.DropEffect
	switch {
	case op == 0:
		return platform.DropNone
	case op&opMove != 0, op&opGeneric != 0, op&opDelete != 0:
		e = platform.DropMove
	case op&opCopy != 0:
		e = platform.DropCopy
	case op&opLink != 0:
		e = platform.DropLink
	}
	if !allowed.Has(e) {
		return platform.DropNone
	}
	return e
}

// draggedTypes returns the pasteboard types a view registers for
// (registerForDraggedTypes:): the well-known ones of the clipboard's
// mapping (text in every encoding AppKit knows, HTML, PNG and TIFF, file
// URLs and URLs), then extra, without duplicates. AppKit sends a view the
// destination methods only for a drag whose pasteboard offers one of its
// registered types; the custom types of this app's own drags are the
// extra ones (see dnd_darwin.go).
func draggedTypes(extra []string) []string {
	out := make([]string, 0, len(wellKnownTypes)+len(extra))
	for _, w := range wellKnownTypes {
		if !slices.Contains(out, w.uti) {
			out = append(out, w.uti)
		}
	}
	for _, t := range extra {
		if !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return out
}

// customDragTypes returns the pasteboard types of the custom entries of a
// drag's data (any type but the four well-known ones), each through
// utiOf (the UTType of the MIME type, else its customType identifier).
func customDragTypes(d platform.Data, utiOf func(mime string) string) []string {
	var out []string
	for _, e := range d {
		switch e.Type {
		case platform.MIMEText, platform.MIMEHTML, platform.MIMEPNG, platform.MIMEURIList:
			continue
		}
		if t := utiOf(e.Type); !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	return out
}

// dragFrame is an NSRect in a flipped view: points, origin at the top-left
// corner.
type dragFrame struct{ X, Y, W, H float64 }

// imageFrame returns the dragging frame of a drag image of size px
// (physical pixels of a window of scale scale) whose point hotspot lies
// under the pointer at pos (view points): the frame's top-left corner is
// pos minus the hotspot in points, and its size the image's in points.
func imageFrame(pos platform.Point, px, hotspot image.Point, scale float64) dragFrame {
	if !(scale > 0) {
		scale = 1
	}
	return dragFrame{
		X: float64(pos.X) - float64(hotspot.X)/scale,
		Y: float64(pos.Y) - float64(hotspot.Y)/scale,
		W: float64(px.X) / scale,
		H: float64(px.Y) / scale,
	}
}

// cancel ends the pointer sequence the OS took away (a drag starts from
// it): a PointerCancel when a button is held, and nothing more of that
// sequence (the buttons are forgotten, so their releases are dropped). A
// leave that waited for the release follows it.
func (p *pointerState) cancel(emit func(platform.PointerEvent)) {
	if p.held == 0 {
		return
	}
	p.held = 0
	emit(platform.PointerEvent{Action: platform.PointerCancel, Pos: p.pos})
	if p.leavePending {
		p.leavePending = false
		p.leave(emit)
	}
}

// dropHost is what a window's drop session needs from the backend.
type dropHost interface {
	// deliverDrag queues e for the window and runs a delivery pass at once
	// (the handler answers with SetDropEffect inside it). It reports
	// false when a pass is already running: e then waits for the next
	// one, and the answer is the previous one.
	deliverDrag(e platform.DragEvent) bool
	now() time.Duration
}

// dropSession is a window's side of a drag over it (the
// NSDraggingDestination methods of TelaView, dnd_darwin.go): the session,
// the app's answer, and the rules of the platform contract. AppKit asks
// for the operation from inside draggingEntered:, draggingUpdated: and
// performDragOperation:, so each event is delivered synchronously and its
// answer returned, as the Win32 backend does inside IDropTarget:
//
//   - A session is DragEnter, any number of DragOver, then one DragLeave
//     or DragDrop. It starts refused (DropNone).
//   - draggingUpdated: comes for every move and modifier change; calls
//     where neither the position, the modifiers nor the allowed effects
//     changed are answered without an event.
//   - The answer is the effect of the latest SetDropEffect when the drag
//     allows it, else DropNone.
//   - A drop with a refusing answer is DragLeave (AppKit sends
//     draggingExited: itself at a release over a destination whose last
//     answer was none; this covers an answer the drop's allowed effects
//     no longer include). An accepted drop reads the data the app asked
//     for (read) and delivers DragDrop; the effect the app holds after
//     handling it is the outcome. Nothing is delivered after the drop:
//     draggingExited: and draggingEnded: find no session.
type dropSession struct {
	active  bool
	types   []string
	allowed platform.DropEffect
	fromApp bool
	pos     platform.Point
	mods    platform.Modifiers
	// effect and want are the app's answer (SetDropEffect).
	effect platform.DropEffect
	want   []string
}

// answer returns the effect AppKit is told.
func (s *dropSession) answer() platform.DropEffect {
	if s.allowed.Has(s.effect) {
		return s.effect
	}
	return platform.DropNone
}

func (s *dropSession) event(h dropHost, a platform.DragAction) platform.DragEvent {
	return platform.DragEvent{Action: a, Pos: s.pos, Types: slices.Clone(s.types), Allowed: s.allowed,
		Mods: s.mods, FromApp: s.fromApp, Time: h.now()}
}

// enter starts a session (draggingEntered:) and returns the answer.
func (s *dropSession) enter(h dropHost, types []string, allowed platform.DropEffect, mods platform.Modifiers, pos platform.Point, fromApp bool) platform.DropEffect {
	if s.active {
		s.leave(h) // a second enter without an exit: keep the contract
	}
	*s = dropSession{active: true, types: slices.Clone(types), allowed: allowed & platform.DropAll, fromApp: fromApp, pos: pos, mods: mods}
	h.deliverDrag(s.event(h, platform.DragEnter))
	return s.answer()
}

// over handles draggingUpdated:: an event only when something changed.
func (s *dropSession) over(h dropHost, allowed platform.DropEffect, mods platform.Modifiers, pos platform.Point) platform.DropEffect {
	if !s.active {
		return platform.DropNone
	}
	allowed &= platform.DropAll
	if pos == s.pos && mods == s.mods && allowed == s.allowed {
		return s.answer()
	}
	s.pos, s.mods, s.allowed = pos, mods, allowed
	h.deliverDrag(s.event(h, platform.DragOver))
	return s.answer()
}

// leave ends the session with DragLeave (draggingExited:, or
// draggingEnded: of a session nothing else ended).
func (s *dropSession) leave(h dropHost) {
	if !s.active {
		return
	}
	e := s.event(h, platform.DragLeave)
	*s = dropSession{}
	h.deliverDrag(e)
}

// drop ends the session at a release (performDragOperation:) and returns
// the effect performed. read returns the data of the given types.
func (s *dropSession) drop(h dropHost, allowed platform.DropEffect, mods platform.Modifiers, pos platform.Point, read func(types []string) platform.Data) platform.DropEffect {
	if !s.active {
		return platform.DropNone
	}
	s.pos, s.mods, s.allowed = pos, mods, allowed&platform.DropAll
	if s.answer() == platform.DropNone {
		s.leave(h)
		return platform.DropNone
	}
	e := s.event(h, platform.DragDrop)
	e.Data = read(platform.DropTypes(s.types, s.want))
	// The session stays active while the drop is handled: SetDropEffect
	// sets the final effect.
	h.deliverDrag(e)
	effect := s.answer()
	*s = dropSession{}
	return effect
}

// setAnswer records SetDropEffect; it does nothing without a session.
func (s *dropSession) setAnswer(effect platform.DropEffect, types []string) {
	if s.active {
		s.effect, s.want = effect, slices.Clone(types)
	}
}
