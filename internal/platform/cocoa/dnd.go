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

// conformanceRoots are the root types of the uniform type identifier
// hierarchy. A view registers for them too, in case AppKit matches a
// drag's types to the registered ones by conformance (it is not
// documented to; the learned types below do not depend on it).
var conformanceRoots = []string{"public.item", "public.data", "public.content"}

// draggedTypes returns the pasteboard types a view registers for
// (registerForDraggedTypes:): the well-known ones of the clipboard's
// mapping (text in every encoding AppKit knows, HTML, PNG and TIFF, file
// URLs and URLs), the file promise types (promiseTypes, then promises:
// NSFilePromiseReceiver's readableDraggedTypes), the conformance roots,
// then extra (the custom types of this app's own drags and the types
// learned from other apps' drags), without duplicates. AppKit sends a
// view the destination methods only for a drag whose pasteboard offers
// one of its registered types (see dnd_darwin.go).
func draggedTypes(promises, extra []string) []string {
	out := make([]string, 0, len(wellKnownTypes)+len(promiseTypes)+len(promises)+len(conformanceRoots)+len(extra))
	add := func(t string) {
		if t != "" && !slices.Contains(out, t) {
			out = append(out, t)
		}
	}
	for _, w := range wellKnownTypes {
		add(w.uti)
	}
	for _, t := range promiseTypes {
		add(t)
	}
	for _, t := range promises {
		add(t)
	}
	for _, t := range conformanceRoots {
		add(t)
	}
	for _, t := range extra {
		add(t)
	}
	return out
}

// maxLearnedTypes bounds the pasteboard types learned from other apps'
// drags: the registration of every window grows with them.
const maxLearnedTypes = 256

// learnTypes adds the pasteboard types of a drag that the views are not
// registered for (registered: what draggedTypes gave) to learned, keeping
// at most maxLearnedTypes (the oldest go first), and reports whether
// learned changed. Types that are not data a drop could carry (dynamic
// identifiers, names with spaces) are learned too: a drag of another app
// may offer nothing else, and AppKit matches them by name.
func learnTypes(learned, registered, offered []string) ([]string, bool) {
	changed := false
	for _, t := range offered {
		if t == "" || slices.Contains(registered, t) || slices.Contains(learned, t) {
			continue
		}
		learned = append(learned, t)
		changed = true
	}
	if n := len(learned) - maxLearnedTypes; n > 0 {
		learned = slices.Delete(learned, 0, n)
	}
	return learned, changed
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

// dragIconPoints is the side of the icon a drag without an image shows,
// in points (Finder drags 32-point icons in list views).
const dragIconPoints = 32

// dragIcon is what a dragging item shows when the app gives no image: the
// icon of a file (path), else the icon of a content type (uti).
type dragIcon struct{ path, uti string }

// dragIcons returns the icon of each pasteboard item of a drag without an
// image: a file: URL's file (NSWorkspace iconForFile:), else the type of
// the item's first entry (iconForContentType:: text, HTML, a URL, a PNG,
// a custom type), public.data for an item without entries.
func dragIcons(items []pbItem) []dragIcon {
	out := make([]dragIcon, len(items))
	for i, it := range items {
		out[i] = dragIcon{uti: "public.data"}
		for _, e := range it {
			if e.uti == utiFileURL {
				if p, err := platform.FilePath(string(e.data)); err == nil {
					out[i] = dragIcon{path: p}
					break
				}
			}
		}
		if out[i].path == "" && len(it) > 0 && it[0].uti != "" {
			out[i].uti = it[0].uti
		}
	}
	return out
}

// iconFrame returns the dragging frame of a drag icon of side points:
// centered under the pointer at pos (view points).
func iconFrame(pos platform.Point, side float64) dragFrame {
	return dragFrame{X: float64(pos.X) - side/2, Y: float64(pos.Y) - side/2, W: side, H: side}
}

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

// heldDrop is a drop whose DragDrop waits for data that arrives later
// (file promises, dnd_darwin.go): the event without that data, and the
// answer the session held at the release.
type heldDrop struct {
	event  platform.DragEvent
	effect platform.DropEffect
	want   []string
}

// hold ends the session at a release like drop, but delivers nothing: it
// returns the DragDrop to deliver later with deliverHeld and the types
// the drop reads. The effect performed now (what AppKit is told) is
// held.effect: the handler's final effect cannot reach the source later.
// A refusing answer is a DragLeave, as in drop, and ok is false.
func (s *dropSession) hold(h dropHost, allowed platform.DropEffect, mods platform.Modifiers, pos platform.Point) (held heldDrop, types []string, ok bool) {
	if !s.active {
		return heldDrop{}, nil, false
	}
	s.pos, s.mods, s.allowed = pos, mods, allowed&platform.DropAll
	effect := s.answer()
	if effect == platform.DropNone {
		s.leave(h)
		return heldDrop{}, nil, false
	}
	held = heldDrop{event: s.event(h, platform.DragDrop), effect: effect, want: slices.Clone(s.want)}
	types = platform.DropTypes(s.types, s.want)
	*s = dropSession{}
	return held, types, true
}

// deliverHeld delivers a held drop with its data: the session is active
// again while the handler runs (SetDropEffect works there as in a drop),
// and ends with it. It returns the final effect. An active session (a
// drag that entered meanwhile) is left first.
func (s *dropSession) deliverHeld(h dropHost, d heldDrop, data platform.Data) platform.DropEffect {
	if s.active {
		s.leave(h)
	}
	e := d.event
	e.Data = data
	e.Time = h.now()
	*s = dropSession{active: true, types: slices.Clone(e.Types), allowed: e.Allowed, fromApp: e.FromApp,
		pos: e.Pos, mods: e.Mods, effect: d.effect, want: d.want}
	h.deliverDrag(e)
	effect := s.answer()
	*s = dropSession{}
	return effect
}

// promisedOnly reports whether a drag's items (each a list of pasteboard
// types) hold a file promise (promiseTypes, or one of promises) on an
// item without a file URL: a file the drop must receive. An item that has
// both is read from its file URL, so its file does not arrive twice.
func promisedOnly(items [][]string, promises []string) bool {
	for _, types := range items {
		if slices.Contains(types, utiFileURL) {
			continue
		}
		if slices.ContainsFunc(types, func(t string) bool { return isPromiseType(t) || slices.Contains(promises, t) }) {
			return true
		}
	}
	return false
}

// promiseDrop counts the files of a drop's file promises
// (NSFilePromiseReceiver): the files expected, and those that arrived
// (paths) or failed, in arrival order.
type promiseDrop struct {
	expected int
	paths    []string
	failed   []string
}

// arrived records one file's outcome (a path, or the error's text) and
// reports whether every expected file is in.
func (p *promiseDrop) arrived(path, failure string) bool {
	if path != "" {
		p.paths = append(p.paths, path)
	} else {
		p.failed = append(p.failed, failure)
	}
	return p.done()
}

func (p *promiseDrop) done() bool { return len(p.paths)+len(p.failed) >= p.expected }

// withFiles returns d with the files at paths added to its URI list as
// file: URIs, after the list's own URIs; without one, a URI list entry is
// added at the place of MIMEURIList in order (the types the drop read,
// best first). d is not modified.
func withFiles(d platform.Data, order []string, paths []string) platform.Data {
	var uris []string
	for _, p := range paths {
		if u, err := platform.FileURI(p); err == nil {
			uris = append(uris, u)
		}
	}
	if len(uris) == 0 {
		return d
	}
	out := d.Clone()
	for i, e := range out {
		if e.Type == platform.MIMEURIList {
			out[i].Bytes = platform.URIList(append(platform.ParseURIList(e.Bytes), uris...))
			return out
		}
	}
	entry := platform.DataEntry{Type: platform.MIMEURIList, Bytes: platform.URIList(uris)}
	at := len(out)
	if k := slices.Index(order, platform.MIMEURIList); k >= 0 {
		for i, e := range out {
			if j := slices.Index(order, e.Type); j > k {
				at = i
				break
			}
		}
	}
	return slices.Insert(out, at, entry)
}

// setAnswer records SetDropEffect; it does nothing without a session.
func (s *dropSession) setAnswer(effect platform.DropEffect, types []string) {
	if s.active {
		s.effect, s.want = effect, slices.Clone(types)
	}
}

// dragLookGap is the shortest time between two looks at the drag
// pasteboard within one drag sequence (dragWatch).
const dragLookGap = 50 * time.Millisecond

// dragWatch decides when the global monitor of left mouse drags looks at
// the drag pasteboard's changeCount (ADR-237), which is a call to the
// pasteboard server, for the drag events of every app. Within one drag
// sequence (from its first LeftMouseDragged to the next LeftMouseUp or
// LeftMouseDown of another app) it
// looks at the first event and then at most once per dragLookGap, since
// the source writes the pasteboard only once its own drag threshold is
// crossed, a few events in; once the sequence has shown a new drag it
// looks no more until the mouse goes up. A text selection or a window
// move in another app costs a look per dragLookGap instead of one per
// event, and a drag one look at most dragLookGap after it starts, long
// before the pointer leaves the source's window for one of ours.
type dragWatch struct {
	seen bool      // this sequence has shown a new drag
	last time.Time // the latest look of this sequence; zero before the first
}

// dragged reports whether to look at the pasteboard for a drag event at
// now.
func (w *dragWatch) dragged(now time.Time) bool {
	if w.seen || !w.last.IsZero() && now.Sub(w.last) < dragLookGap {
		return false
	}
	w.last = now
	return true
}

// changed notes that a look found a new drag: none more this sequence.
func (w *dragWatch) changed() { w.seen = true }

// up ends the sequence (LeftMouseUp, LeftMouseDown).
func (w *dragWatch) up() { *w = dragWatch{} }
