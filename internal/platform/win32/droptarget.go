package win32

import (
	"slices"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// dropHost is what a window's drop session needs from the backend.
type dropHost interface {
	// deliverDrag queues e for the window and runs a delivery pass at once
	// (the handler answers with SetDropEffect inside it). It reports
	// false when a pass is already running: e then waits for the next
	// one, and the answer is the previous one.
	deliverDrag(e platform.DragEvent) bool
	now() time.Duration
}

// dropSession is the pure half of a window's IDropTarget
// (dnd_windows.go): the drag over the window, the app's answer, and the
// rules of the platform contract. OLE asks for the effect from inside
// DragEnter, DragOver and Drop, so each event is delivered synchronously
// and its answer returned:
//
//   - A session is DragEnter, any number of DragOver, then one DragLeave
//     or DragDrop. It starts refused (DropNone).
//   - OLE calls DragOver on a timer too, with nothing changed: those calls
//     are answered without an event.
//   - The answer is the effect of the latest SetDropEffect when the drag
//     allows it, else DropNone (an effect outside Allowed counts as
//     DropNone).
//   - Drop with a refusing answer is DragLeave (OLE calls DragLeave
//     itself when the last answer was DROPEFFECT_NONE; this covers an
//     answer that the allowed effects of the drop no longer include). An
//     accepted drop reads the data the app asked for (read) and delivers
//     DragDrop; the effect the app holds after handling it is the outcome.
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

// answer returns the effect OLE is told.
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

// enter starts a session (IDropTarget::DragEnter) and returns the answer.
func (s *dropSession) enter(h dropHost, types []string, allowed platform.DropEffect, mods platform.Modifiers, pos platform.Point, fromApp bool) platform.DropEffect {
	if s.active {
		s.leave(h) // OLE never enters twice; keep the contract if it did
	}
	*s = dropSession{active: true, types: slices.Clone(types), allowed: allowed & platform.DropAll, fromApp: fromApp, pos: pos, mods: mods}
	h.deliverDrag(s.event(h, platform.DragEnter))
	return s.answer()
}

// over handles IDropTarget::DragOver: an event only when something
// changed.
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

// leave ends the session with DragLeave (IDropTarget::DragLeave).
func (s *dropSession) leave(h dropHost) {
	if !s.active {
		return
	}
	e := s.event(h, platform.DragLeave)
	*s = dropSession{}
	h.deliverDrag(e)
}

// drop ends the session at a release (IDropTarget::Drop) and returns the
// effect performed. read returns the data of the given types.
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
