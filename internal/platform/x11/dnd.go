//go:build linux

package x11

import (
	"slices"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/x11/internal/xgb/xproto"
)

// Drag and drop target side, XDND version 5 (freedesktop.org's XDND
// specification; GTK 3's gdkdnd-x11.c and Qt's qxcbdrag.cpp are the
// reference implementations):
//
//   - Every window has XdndAware = 5 (NewWindow).
//   - XdndEnter starts a session: the source window, the protocol version
//     (the lower of the source's and 5), and the types: the three in the
//     message, or the source's XdndTypeList when bit 0 says there are
//     more. Types are normalized with platform.SelectionType.
//   - The first XdndPosition is the DragEnter, later ones DragOver when
//     the position, the modifiers or the allowed effects changed. The
//     root position is translated with the QueryPointer reply that also
//     gives the modifiers (the pointer's mask; X sends no key events to a
//     window while another client grabs the pointer for a drag). Allowed
//     is the requested action, or the source's XdndActionList for
//     XdndActionAsk; a version-0/1 source (no action) and
//     XdndActionPrivate allow copy.
//   - Every XdndPosition is answered with XdndStatus after the handler
//     saw the event (a callback posted behind it): accepted when the
//     window's sticky SetDropEffect is in Allowed, with that action, and
//     an empty rectangle with "send positions" set, so every move is
//     reported.
//   - XdndLeave is DragLeave. XdndDrop, also behind the queued events: a
//     refused session ends with DragLeave and XdndFinished (not
//     accepted); an accepted one converts XdndSelection to the best
//     target of each wanted type with the drop's timestamp, one after
//     the other (INCR included; a type that fails or times out is left
//     out), delivers DragDrop with the data, and sends XdndFinished with
//     the effect the window holds once the handler returned.

// xdndVersion is the XDND protocol version the backend speaks.
const xdndVersion = 5

// xdndMinVersion is the oldest source version accepted (GTK 3 and Qt
// accept 3 and later; earlier ones predate XdndAware versions as used
// today).
const xdndMinVersion = 3

// dropSession is an XDND drag of another client over a window.
type dropSession struct {
	source   xproto.Window
	version  int
	targets  []string // the offered target names, in the source's order
	types    []string // normalized MIME types
	entered  bool     // DragEnter was queued
	pos      platform.Point
	mods     platform.Modifiers
	allowed  platform.DropEffect
	dropped  bool // XdndDrop arrived: XdndLeave is ignored
	finished bool // XdndFinished was sent
}

// windowDnD is a window's drag and drop state.
type windowDnD struct {
	session  *dropSession // an XDND drag of another client over it
	answer   dropAnswer   // the latest SetDropEffect of the session
	dropping bool         // a DragDrop is being handled: SetDropEffect sets the outcome
	appOver  bool         // the app's own drag is over it (dragsource.go)
}

// dropAnswer is a window's SetDropEffect.
type dropAnswer struct {
	effect platform.DropEffect
	types  []string
}

// SetDropEffect records the window's answer to the drag over it; it does
// nothing without one. XDND sources get it with the XdndStatus that
// follows each position.
func (w *Window) SetDropEffect(effect platform.DropEffect, types ...string) {
	platform.CheckSetDropEffect(effect, types)
	d := &w.dnd
	if w.Closed() || d.session == nil && !d.dropping && !d.appOver {
		return
	}
	d.answer = dropAnswer{effect: effect, types: slices.Clone(types)}
}

// accepted returns the effect the window's answer gives a drag allowing
// allowed: DropNone when it refuses or is outside allowed.
func (d *windowDnD) accepted(allowed platform.DropEffect) platform.DropEffect {
	if allowed.Has(d.answer.effect) {
		return d.answer.effect
	}
	return platform.DropNone
}

// xdndMessage handles an XDND client message sent to w; it reports
// whether ev was one.
func (b *Backend) xdndMessage(w *Window, ev xproto.ClientMessageEvent) bool {
	a := &b.atoms
	if ev.Format != 32 || len(ev.Data.Data32) < 5 {
		return false
	}
	d := ev.Data.Data32
	switch ev.Type {
	case a.xdndEnter:
		b.xdndEnter(w, xproto.Window(d[0]), d[1], d[2:5])
	case a.xdndPosition:
		b.xdndPosition(w, xproto.Window(d[0]), int16(d[2]>>16), int16(d[2]&0xffff), xproto.Atom(d[4]))
	case a.xdndLeave:
		b.xdndLeave(w, xproto.Window(d[0]))
	case a.xdndDrop:
		b.xdndDrop(w, xproto.Window(d[0]), d[2])
	case a.xdndStatus:
		b.dragStatus(w, xproto.Window(d[0]), d[1], xproto.Atom(d[4]))
	case a.xdndFinished:
		b.dragFinished(w, xproto.Window(d[0]), d[1], xproto.Atom(d[2]))
	default:
		return false
	}
	return true
}

// xdndEnter starts a session over w.
func (b *Backend) xdndEnter(w *Window, source xproto.Window, flags uint32, inline []uint32) {
	version := int(flags >> 24)
	if version < xdndMinVersion {
		return
	}
	if s := w.dnd.session; s != nil && s.entered {
		// A new drag without the old one's leave (a source that died):
		// end the old session for the app.
		b.loop.QueueDrag(w, b.sessionEvent(w, s, platform.DragLeave))
	}
	var atoms []xproto.Atom
	if flags&1 != 0 {
		if p, ok := b.xs.property(source, b.atoms.xdndTypeList, false); ok && p.format == 32 {
			for i := 0; i+4 <= len(p.data); i += 4 {
				atoms = append(atoms, xproto.Atom(le.Uint32(p.data[i:])))
			}
		}
	}
	if len(atoms) == 0 {
		for _, t := range inline {
			if t != 0 {
				atoms = append(atoms, xproto.Atom(t))
			}
		}
	}
	targets := b.atomNamesOf(atoms)
	w.dnd.session = &dropSession{source: source, version: min(version, xdndVersion), targets: targets, types: normalizeTargets(targets)}
	w.dnd.answer = dropAnswer{}
}

// xdndPosition is the source's pointer position over w (root coordinates).
func (b *Backend) xdndPosition(w *Window, source xproto.Window, x, y int16, action xproto.Atom) {
	s := w.dnd.session
	if s == nil || s.source != source || s.dropped {
		return
	}
	mods := s.mods
	pos := s.pos
	if qp, ok := b.xs.queryPointer(w.id); ok {
		mods = b.kbd.mods(uint32(qp.mask))
		// The window's origin in root coordinates is the pointer's root
		// position minus its window position.
		ox, oy := float64(qp.rootX)-float64(qp.winX), float64(qp.rootY)-float64(qp.winY)
		pos = w.posf(float64(x)-ox, float64(y)-oy)
	}
	allowed := b.xdndAllowed(s, source, action)
	switch {
	case !s.entered:
		s.entered = true
		s.pos, s.mods, s.allowed = pos, mods, allowed
		if w.inside {
			// The pointer that was over the window is the drag's now.
			b.leave(w, 0, mods)
		}
		b.loop.QueueDrag(w, b.sessionEvent(w, s, platform.DragEnter))
	case pos != s.pos || mods != s.mods || allowed != s.allowed:
		s.pos, s.mods, s.allowed = pos, mods, allowed
		b.loop.QueueDrag(w, b.sessionEvent(w, s, platform.DragOver))
	}
	// Answer once the app handled the event (or at once, behind the
	// queue, when nothing changed): the source waits for this status
	// before it sends the next position.
	b.loop.Post(func() { b.sendStatus(w, s) })
}

// xdndAllowed maps the action of an XdndPosition to the allowed effects.
func (b *Backend) xdndAllowed(s *dropSession, source xproto.Window, action xproto.Atom) platform.DropEffect {
	if s.version < 2 || action == 0 {
		return platform.DropCopy
	}
	if action == b.atoms.xdndActionAsk {
		var e platform.DropEffect
		if p, ok := b.xs.property(source, b.atoms.xdndActionList, false); ok && p.format == 32 {
			for i := 0; i+4 <= len(p.data); i += 4 {
				e |= b.effectOf(xproto.Atom(le.Uint32(p.data[i:])))
			}
		}
		if e == platform.DropNone {
			e = platform.DropCopy
		}
		return e
	}
	if e := b.effectOf(action); e != platform.DropNone {
		return e
	}
	return platform.DropCopy // XdndActionPrivate, an unknown action
}

// effectOf maps an XDND action to a drop effect (DropNone for others).
func (b *Backend) effectOf(action xproto.Atom) platform.DropEffect {
	switch action {
	case b.atoms.xdndActionCopy:
		return platform.DropCopy
	case b.atoms.xdndActionMove:
		return platform.DropMove
	case b.atoms.xdndActionLink:
		return platform.DropLink
	}
	return platform.DropNone
}

// actionOf maps a drop effect to its XDND action (None for DropNone).
func (b *Backend) actionOf(e platform.DropEffect) xproto.Atom {
	switch e {
	case platform.DropCopy:
		return b.atoms.xdndActionCopy
	case platform.DropMove:
		return b.atoms.xdndActionMove
	case platform.DropLink:
		return b.atoms.xdndActionLink
	}
	return 0
}

// sessionEvent builds a DragEvent of session s over w.
func (b *Backend) sessionEvent(w *Window, s *dropSession, action platform.DragAction) platform.DragEvent {
	return platform.DragEvent{
		Action:  action,
		Pos:     s.pos,
		Types:   slices.Clone(s.types),
		Allowed: s.allowed,
		Mods:    s.mods,
		FromApp: b.integ.drag != nil || b.windows[s.source] != nil,
		Time:    b.Now(),
	}
}

// sendStatus answers the latest position of session s over w.
func (b *Backend) sendStatus(w *Window, s *dropSession) {
	if w.Closed() || w.dnd.session != s || s.dropped {
		return
	}
	effect := w.dnd.accepted(s.allowed)
	flags := uint32(2) // send positions even inside the (empty) rectangle
	if effect != platform.DropNone {
		flags |= 1
	}
	var action uint32
	if s.version >= 2 {
		action = uint32(b.actionOf(effect))
	}
	b.sendXdnd(s.source, b.atoms.xdndStatus, uint32(w.id), flags, 0, 0, action)
}

// sendXdnd sends an XDND client message to win.
func (b *Backend) sendXdnd(win xproto.Window, typ xproto.Atom, data ...uint32) {
	var d [5]uint32
	copy(d[:], data)
	ev := xproto.ClientMessageEvent{Format: 32, Window: win, Type: typ, Data: xproto.ClientMessageDataUnionData32New(d[:])}
	b.xs.sendEvent(win, 0, ev.Bytes())
}

// xdndLeave ends the session over w.
func (b *Backend) xdndLeave(w *Window, source xproto.Window) {
	s := w.dnd.session
	if s == nil || s.source != source || s.dropped {
		return
	}
	w.dnd.session = nil
	w.dnd.answer = dropAnswer{}
	if s.entered {
		b.loop.QueueDrag(w, b.sessionEvent(w, s, platform.DragLeave))
	}
}

// xdndDrop is the release over w. It is handled behind the queued events,
// so the answer is the one the app gave to the last position.
func (b *Backend) xdndDrop(w *Window, source xproto.Window, t uint32) {
	s := w.dnd.session
	if s == nil || s.source != source || s.dropped {
		return
	}
	s.dropped = true
	b.loop.Post(func() {
		if w.Closed() || w.dnd.session != s {
			b.sendFinished(w, s, platform.DropNone)
			return
		}
		effect := w.dnd.accepted(s.allowed)
		if !s.entered || effect == platform.DropNone {
			w.dnd.session, w.dnd.answer = nil, dropAnswer{}
			if s.entered {
				b.loop.QueueDrag(w, b.sessionEvent(w, s, platform.DragLeave))
			}
			b.sendFinished(w, s, platform.DropNone)
			return
		}
		types := platform.DropTypes(s.types, w.dnd.answer.types)
		b.readDrop(w, s, t, types, nil)
	})
}

// readDrop reads the wanted types one after the other, then delivers the
// drop.
func (b *Backend) readDrop(w *Window, s *dropSession, t uint32, types []string, data platform.Data) {
	if len(types) == 0 {
		b.deliverDrop(w, s, data)
		return
	}
	mime := types[0]
	b.readType(b.atoms.xdndSelection, t, s.targets, mime, func(bytes []byte, ok bool, _ error) {
		if ok {
			data = append(data, platform.DataEntry{Type: mime, Bytes: bytes})
		}
		b.readDrop(w, s, t, types[1:], data)
	})
}

// deliverDrop queues the DragDrop, and the XdndFinished with the effect
// the window holds once the handler returned.
func (b *Backend) deliverDrop(w *Window, s *dropSession, data platform.Data) {
	if w.Closed() || w.dnd.session != s {
		b.sendFinished(w, s, platform.DropNone)
		return
	}
	w.dnd.session = nil
	ev := b.sessionEvent(w, s, platform.DragDrop)
	ev.Data = data
	b.loop.QueueDrag(w, ev)
	w.dnd.dropping = true
	b.loop.Post(func() {
		effect := w.dnd.accepted(s.allowed)
		w.dnd.dropping, w.dnd.answer = false, dropAnswer{}
		b.sendFinished(w, s, effect)
	})
	b.signal()
}

// sendFinished tells the source the drop is over: accepted with the
// performed action, or not (version 5 fields; older sources read only
// the target window).
func (b *Backend) sendFinished(w *Window, s *dropSession, effect platform.DropEffect) {
	if s.finished {
		return
	}
	s.finished = true
	var flags, action uint32
	if effect != platform.DropNone {
		flags, action = 1, uint32(b.actionOf(effect))
	}
	b.sendXdnd(s.source, b.atoms.xdndFinished, uint32(w.id), flags, action)
}

// endSessions ends the XDND session over a closing window: a source
// waiting for the drop is told it did not happen.
func (b *Backend) endSessions(w *Window) {
	if s := w.dnd.session; s != nil {
		w.dnd.session = nil
		if s.dropped {
			b.sendFinished(w, s, platform.DropNone)
		}
	}
}
