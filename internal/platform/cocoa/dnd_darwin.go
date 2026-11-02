//go:build darwin

package cocoa

import (
	"errors"
	"image"
	"slices"
	"time"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/keyboard"
)

// Drag and drop (phase 15, ADR-224): TelaView is the NSDraggingDestination
// of its window and the NSDraggingSource of the app's drags. See "Drag and
// drop" in the package documentation; the pure rules are in dnd.go and the
// Data mapping is the clipboard's (pasteboard.go, clipboard_darwin.go).
//
// Quirks handled, after Chromium (web_drag_dest_mac.mm,
// web_contents_view_mac.mm, web_drag_source_mac.mm):
//
//   - AppKit asks for the operation from inside draggingEntered:,
//     draggingUpdated: and performDragOperation:, so those methods deliver
//     their event synchronously (a delivery pass inside the call, as the
//     Win32 backend does inside IDropTarget) and return the window's
//     sticky answer. Inside a pass (a method called from the handler) the
//     event waits for the follow-up and the previous answer is returned.
//   - A view gets the destination methods only for a drag whose pasteboard
//     offers one of its registered types: the well-known types of the
//     clipboard mapping, plus the custom types of this app's own drags,
//     registered on every window before such a drag starts (another app's
//     custom types cannot be listed in advance; a drag that offers only
//     those is not seen, as in Chromium, which registers its own custom
//     type only).
//   - draggingSourceOperationMask is already filtered by the modifiers
//     (Option leaves copy, Command generic, Control link), so Allowed
//     follows them; generic counts as move. The answer is given back in
//     the mask's own bits (move as generic when only generic is offered).
//   - A release over a destination whose last answer was none is
//     draggingExited: (no performDragOperation:). draggingEnded: ends a
//     session nothing else ended; after a drop both find no session, so
//     no DragLeave follows a DragDrop whatever AppKit's order.
//   - performDragOperation: only says yes or no: the source learns the
//     operation of the last draggingUpdated:. For this app's own drags the
//     effect the target held after the drop is reported instead.
//   - beginDraggingSessionWithItems:event:source: needs the mouse event the
//     drag starts from (the press or its latest drag, kept while a button
//     is held) and returns at once; the session runs from the event loop,
//     and its end is draggingSession:endedAtPoint:operation:. The source
//     view is retained until then (its window may close meanwhile).
//   - Files are one dragging item per URI, the clipboard's items (Finder
//     reads every item's public.file-url); every item needs a frame and
//     contents, so the items after the first get the same frame with a
//     transparent image.
//   - wantsPeriodicDraggingUpdates is NO: draggingUpdated: comes for moves
//     and modifier changes only.

// dndSel holds the selectors of drag and drop.
var dndSel = struct {
	registerForDraggedTypes, unregisterDraggedTypes, draggingPasteboard    darwin.SEL
	draggingLocation, draggingSourceOperationMask, draggingSource          darwin.SEL
	beginDraggingSession, initWithPasteboardWriter, setDraggingFrame       darwin.SEL
	initWithCGImageSize, setDraggingFormation, setAnimatesToStartPositions darwin.SEL
}{
	registerForDraggedTypes:     darwin.Sel("registerForDraggedTypes:"),
	unregisterDraggedTypes:      darwin.Sel("unregisterDraggedTypes"),
	draggingPasteboard:          darwin.Sel("draggingPasteboard"),
	draggingLocation:            darwin.Sel("draggingLocation"),
	draggingSourceOperationMask: darwin.Sel("draggingSourceOperationMask"),
	draggingSource:              darwin.Sel("draggingSource"),
	beginDraggingSession:        darwin.Sel("beginDraggingSessionWithItems:event:source:"),
	initWithPasteboardWriter:    darwin.Sel("initWithPasteboardWriter:"),
	setDraggingFrame:            darwin.Sel("setDraggingFrame:contents:"),
	initWithCGImageSize:         darwin.Sel("initWithCGImage:size:"),
	setDraggingFormation:        darwin.Sel("setDraggingFormation:"),
	setAnimatesToStartPositions: darwin.Sel("setAnimatesToStartingPositionsOnCancelOrFail:"),
}

// windowDnD is a window's drag and drop state.
type windowDnD struct {
	session dropSession
	own     bool // the session's drag is this app's (its Data is read as it is)
	// press is the NSEvent of the held press, or of its latest drag
	// (retained), which beginDraggingSessionWithItems: starts from.
	press darwin.ID
	// swallow drops the rest of the pointer sequence a drag of this
	// window took (StartDrag), until its release or a move without
	// buttons.
	swallow bool
}

// backendDnD is the backend's drag and drop state.
type backendDnD struct {
	drag *outgoingDrag // the drag StartDrag started, until its session ends
	// custom are the custom pasteboard types the views are registered
	// for besides the well-known ones: those of the app's own drags.
	custom []string
}

// outgoingDrag is a drag of this app.
type outgoingDrag struct {
	w       *Window
	view    darwin.ID // the source view (retained until the session ends)
	data    platform.Data
	allowed platform.DropEffect
	// dropped and effect are the outcome of a drop on one of the app's
	// own windows: the effect the target held after its DragDrop.
	dropped bool
	effect  platform.DropEffect
}

// --- target ---

// registerDrop makes the window's view a destination for the dragged
// types.
func (w *Window) registerDrop() {
	types := draggedTypes(w.b.dnd.custom)
	ids := make([]darwin.ID, len(types))
	for i, t := range types {
		ids[i] = darwin.NSString(t)
	}
	darwin.Send(w.view, dndSel.registerForDraggedTypes, darwin.NSArray(ids...))
	for _, id := range ids {
		darwin.Release(id) // the array keeps them
	}
}

// registerDragTypes adds custom pasteboard types to the registration of
// every window (a drag of this app that offers only custom types must
// reach its own windows).
func (b *Backend) registerDragTypes(types []string) {
	added := false
	for _, t := range types {
		if !slices.Contains(b.dnd.custom, t) {
			b.dnd.custom = append(b.dnd.custom, t)
			added = true
		}
	}
	if !added {
		return
	}
	for _, w := range b.loop.Windows() {
		w.registerDrop()
	}
}

// closeDnD runs when the window closes: no more drags reach it, and the
// kept press is released. A drag it started goes on (its session ends it)
// without a DragDoneEvent.
func (w *Window) closeDnD() {
	darwin.Send(w.view, dndSel.unregisterDraggedTypes)
	w.dnd.session = dropSession{}
	w.dnd.own, w.dnd.swallow = false, false
	w.keepPress(0)
}

// cocoaDropHost delivers a window's drag events synchronously.
type cocoaDropHost struct{ w *Window }

func (h cocoaDropHost) deliverDrag(e platform.DragEvent) bool {
	b := h.w.b
	b.loop.QueueDrag(h.w, e)
	if b.pumping || b.handler == nil || !b.running {
		b.notify()
		return false
	}
	b.pump()
	return true
}

func (h cocoaDropHost) now() time.Duration { return h.w.b.Now() }

// currentMods returns the modifiers held now (+[NSEvent modifierFlags]).
func currentMods() platform.Modifiers {
	return keyboard.MacModifiers(darwin.SendUint(darwin.ID(app.event), isel.modifierFlags))
}

// dragEmitter queues the pointer events the drag rules make (a cancel, a
// leave), with the current modifiers and time.
func (w *Window) dragEmitter() func(platform.PointerEvent) {
	mods, t := currentMods(), w.b.Now()
	return func(pe platform.PointerEvent) {
		pe.Kind = platform.PointerMouse
		pe.Mods = mods
		pe.Time = t
		w.b.enqueue(w, pe)
	}
}

// dragPos returns the drag's position in the view (points, top-left
// origin): draggingLocation is in the window's base coordinates.
func (w *Window) dragPos(info darwin.ID) platform.Point {
	loc := darwin.SendPoint(info, dndSel.draggingLocation)
	p := darwin.SendPoint(w.view, isel.convertPointFromView, loc, darwin.ID(0))
	return platform.Point{X: float32(p.X), Y: float32(p.Y)}
}

// ownDrag returns the app's drag when source (draggingSource: the source
// object of a drag of this process, nil for another app's) is its view.
func (b *Backend) ownDrag(source darwin.ID) *outgoingDrag {
	if d := b.dnd.drag; d != nil && source != 0 && source == d.view {
		return d
	}
	return nil
}

// dragEntered handles draggingEntered: and returns the operation.
func (w *Window) dragEntered(info darwin.ID) uint64 {
	mask := darwin.SendUint(info, dndSel.draggingSourceOperationMask)
	own := w.b.ownDrag(darwin.Send(info, dndSel.draggingSource))
	var types []string
	if own != nil {
		types = own.data.Types()
	} else if pb := darwin.Send(info, dndSel.draggingPasteboard); pb != 0 {
		types = typesOf(pb)
	}
	w.dnd.own = own != nil
	// The window gets no pointer events during the drag: a pointer the
	// app believes over it leaves first (this app's drag cancelled its
	// sequence already).
	emit := w.dragEmitter()
	w.ptr.cancel(emit)
	if w.ptr.inside {
		w.ptr.leave(emit)
	}
	w.updateCursorHidden()
	e := w.dnd.session.enter(cocoaDropHost{w}, types, dropEffects(mask), currentMods(), w.dragPos(info), own != nil)
	return targetOperation(e, mask)
}

// dragUpdated handles draggingUpdated:.
func (w *Window) dragUpdated(info darwin.ID) uint64 {
	mask := darwin.SendUint(info, dndSel.draggingSourceOperationMask)
	e := w.dnd.session.over(cocoaDropHost{w}, dropEffects(mask), currentMods(), w.dragPos(info))
	return targetOperation(e, mask)
}

// dragExited handles draggingExited: (and draggingEnded:, which finds no
// session after an exit or a drop).
func (w *Window) dragExited() {
	w.dnd.own = false
	w.dnd.session.leave(cocoaDropHost{w})
}

// dragPerform handles performDragOperation:: the drop's data is read from
// the dragging pasteboard (or the app's own Data) before DragDrop is
// delivered, and the answer is whether the final effect accepts.
func (w *Window) dragPerform(info darwin.ID) bool {
	mask := darwin.SendUint(info, dndSel.draggingSourceOperationMask)
	pb := darwin.Send(info, dndSel.draggingPasteboard)
	var own *outgoingDrag
	if w.dnd.own {
		own = w.b.ownDrag(darwin.Send(info, dndSel.draggingSource))
	}
	w.dnd.own = false
	read := func(types []string) platform.Data {
		if own != nil {
			return own.data.Select(types).Clone()
		}
		return readDrop(pb, types)
	}
	effect := w.dnd.session.drop(cocoaDropHost{w}, dropEffects(mask), currentMods(), w.dragPos(info), read)
	if own != nil && w.b.dnd.drag == own {
		own.dropped, own.effect = true, effect
	}
	return effect != platform.DropNone
}

// readDrop reads types from a dragging pasteboard with the clipboard's
// readers; a type that cannot be read (gone, or an image that does not
// decode) is left out.
func readDrop(pb darwin.ID, types []string) platform.Data {
	if pb == 0 {
		return nil
	}
	var d platform.Data
	for _, t := range types {
		b, err := readFrom(pb, t)
		if err != nil || b == nil {
			continue
		}
		d = append(d, platform.DataEntry{Type: t, Bytes: b})
	}
	return d
}

// SetDropEffect answers the drag over the window; AppKit gets the answer
// when the destination method that delivered the event returns, and at
// every later draggingUpdated: until it changes or the session ends.
func (w *Window) SetDropEffect(effect platform.DropEffect, types ...string) {
	platform.CheckSetDropEffect(effect, types)
	if !w.Closed() {
		w.dnd.session.setAnswer(effect, types)
	}
}

// --- pointer ---

// keepPress keeps ev, a press or a drag while a button is held, for
// StartDrag; without buttons held (or with ev 0) the kept event is
// released.
func (w *Window) keepPress(ev darwin.ID) {
	if ev == 0 || w.ptr.held == 0 {
		if w.dnd.press != 0 {
			darwin.Release(w.dnd.press)
			w.dnd.press = 0
		}
		return
	}
	switch darwin.SendUint(ev, isel.typ) {
	case darwin.NSEventTypeLeftMouseDown, darwin.NSEventTypeRightMouseDown, darwin.NSEventTypeOtherMouseDown,
		darwin.NSEventTypeLeftMouseDragged, darwin.NSEventTypeRightMouseDragged, darwin.NSEventTypeOtherMouseDragged:
		if ev != w.dnd.press {
			darwin.Retain(ev)
			darwin.Release(w.dnd.press)
			w.dnd.press = ev
		}
	}
}

// dragTakes reports whether the pointer event ev belongs to a drag and is
// not delivered: while a drag is over the window, and the rest of the
// sequence a drag of this window started from (until its release, or a
// move without buttons, which is delivered: the pointer is back).
func (w *Window) dragTakes(ev darwin.ID) bool {
	if w.dnd.session.active {
		return true
	}
	if !w.dnd.swallow {
		return false
	}
	switch darwin.SendUint(ev, isel.typ) {
	case darwin.NSEventTypeMouseMoved:
		w.dnd.swallow = false
		return false
	case darwin.NSEventTypeLeftMouseUp, darwin.NSEventTypeRightMouseUp, darwin.NSEventTypeOtherMouseUp:
		w.dnd.swallow = false
	}
	return true
}

// --- source ---

// StartDrag starts a drag of opts.Data from the held press: an
// NSDraggingSession with one dragging item per URI of a URI list and the
// other types on the first, the image at the hotspot under the pointer.
// It needs a pointer button pressed in the window and still held, and no
// other drag of the app in progress, else ErrNoGrab. The window's pointer
// sequence ends with a PointerCancel, queued before any drag event; one
// DragDoneEvent follows when the session ends.
func (w *Window) StartDrag(opts platform.DragOptions) error {
	platform.CheckDragOptions(opts)
	b := w.b
	if w.Closed() || b.closed || b.dnd.drag != nil || w.ptr.held == 0 || w.dnd.press == 0 {
		return platform.ErrNoGrab
	}
	pool := darwin.PoolPush()
	defer darwin.PoolPop(pool)
	data := opts.Data.Clone()
	items := pasteboardItems(data, utiForMIME)
	if len(items) == 0 {
		return errors.New("cocoa: StartDrag: the data puts nothing on a pasteboard (a URI list without URIs?)")
	}
	b.registerDragTypes(customDragTypes(data, utiForMIME))

	scale := w.Metrics().Scale
	pos := w.eventPos(w.dnd.press)
	frame := dragFrame{X: float64(pos.X), Y: float64(pos.Y), W: 1, H: 1}
	var picture darwin.ID
	if opts.Image != nil {
		picture = b.dragImage(opts.Image, scale)
		frame = imageFrame(pos, opts.Image.Rect.Size(), opts.Hotspot, scale)
	}
	blank := b.dragImage(image.NewRGBA(image.Rect(0, 0, 1, 1)), 1)
	defer darwin.Release(blank)
	if picture == 0 {
		picture = darwin.Retain(blank)
	}
	defer darwin.Release(picture)

	array := darwin.SendClass(darwin.GetClass("NSMutableArray"), pbSel.array)
	itemClass := darwin.GetClass("NSDraggingItem")
	rect := darwin.CGRect{Origin: darwin.CGPoint{X: frame.X, Y: frame.Y}, Size: darwin.CGSize{Width: frame.W, Height: frame.H}}
	for i, it := range items {
		writer, err := newPasteboardItem(it)
		if err != nil {
			return err
		}
		item := darwin.Send(darwin.SendClass(itemClass, sel.alloc), dndSel.initWithPasteboardWriter, writer)
		darwin.Release(writer) // the dragging item keeps it
		if item == 0 {
			return errors.New("cocoa: StartDrag: creating an NSDraggingItem failed")
		}
		contents := picture
		if i > 0 {
			contents = blank
		}
		darwin.Send(item, dndSel.setDraggingFrame, rect, contents)
		darwin.Send(array, pbSel.addObject, item)
		darwin.Release(item) // the array keeps it
	}

	d := &outgoingDrag{w: w, view: darwin.Retain(w.view), data: data, allowed: opts.Allowed}
	// Set before the session begins: AppKit may ask for the operation
	// mask, or call the destination methods of this view, at once.
	b.dnd.drag = d
	session := darwin.Send(w.view, dndSel.beginDraggingSession, array, w.dnd.press, w.view)
	if session == 0 {
		b.dnd.drag = nil
		darwin.Release(d.view)
		return errors.New("cocoa: beginDraggingSessionWithItems:event:source: returned nil")
	}
	darwin.Send(session, dndSel.setDraggingFormation, uint(darwin.NSDraggingFormationNone))
	darwin.Send(session, dndSel.setAnimatesToStartPositions, true)
	// The OS takes the pointer from here on.
	w.ptr.cancel(w.dragEmitter())
	w.dnd.swallow = true
	w.keepPress(0)
	w.updateCursorHidden()
	return nil
}

// dragImage returns an NSImage (+1) of img (premultiplied RGBA, physical
// pixels) sized in points of scale, or 0 when it cannot be made.
func (b *Backend) dragImage(img *image.RGBA, scale float64) darwin.ID {
	if !(scale > 0) {
		scale = 1
	}
	// A copy at the origin with its own rows: newImage reads Pix as rows
	// of Stride bytes from the image's first pixel.
	size := img.Rect.Size()
	c := image.NewRGBA(image.Rectangle{Max: size})
	for y := range size.Y {
		copy(c.Pix[y*c.Stride:], img.Pix[img.PixOffset(img.Rect.Min.X, img.Rect.Min.Y+y):][:4*size.X])
	}
	cg, err := b.newImage(c)
	if err != nil {
		return 0
	}
	defer darwin.CGImageRelease(cg)
	ns := darwin.Send(darwin.SendClass(darwin.GetClass("NSImage"), sel.alloc), dndSel.initWithCGImageSize,
		cg, darwin.CGSize{Width: float64(size.X) / scale, Height: float64(size.Y) / scale})
	return ns
}

// dragOf returns the backend whose drag has the source view view.
func dragOf(view darwin.ID) (*Backend, *outgoingDrag) {
	registry.Lock()
	defer registry.Unlock()
	for _, b := range registry.backends {
		if d := b.dnd.drag; d != nil && d.view == view {
			return b, d
		}
	}
	return nil, nil
}

// dragEnded handles draggingSession:endedAtPoint:operation:: one
// DragDoneEvent with the effect, unless the source window closed.
func dragEnded(view darwin.ID, op uint64) {
	b, d := dragOf(view)
	if d == nil {
		return
	}
	b.guard(func() {
		b.dnd.drag = nil
		effect := doneEffect(op, d.allowed)
		if d.dropped {
			effect = d.effect
		}
		b.releaseLater(d.view) // not inside its own method
		w := d.w
		if w.Closed() {
			return
		}
		w.dnd.swallow = false
		b.enqueue(w, platform.DragDoneEvent{Effect: effect})
		b.pump()
	})
}

// dndMethods are the NSDraggingDestination and NSDraggingSource methods
// of TelaView.
func dndMethods() []darwin.Method {
	window := func(self darwin.ID) *Window {
		if w := windowFor(self); w != nil && !w.Closed() {
			return w
		}
		return nil
	}
	operation := func(f func(w *Window, info darwin.ID) uint64) func(darwin.ID, darwin.SEL, darwin.ID) uint {
		return func(self darwin.ID, _ darwin.SEL, info darwin.ID) uint {
			var op uint64
			if w := window(self); w != nil {
				w.b.guard(func() { op = f(w, info) })
			}
			return uint(op)
		}
	}
	exited := func(self darwin.ID, _ darwin.SEL, _ darwin.ID) {
		if w := window(self); w != nil {
			w.b.guard(w.dragExited)
		}
	}
	return []darwin.Method{
		{Sel: darwin.Sel("draggingEntered:"), Fn: operation((*Window).dragEntered)},
		{Sel: darwin.Sel("draggingUpdated:"), Fn: operation((*Window).dragUpdated)},
		{Sel: darwin.Sel("draggingExited:"), Fn: exited},
		{Sel: darwin.Sel("draggingEnded:"), Fn: exited},
		{Sel: darwin.Sel("wantsPeriodicDraggingUpdates"), Fn: func(darwin.ID, darwin.SEL) bool { return false }},
		{Sel: darwin.Sel("performDragOperation:"), Fn: func(self darwin.ID, _ darwin.SEL, info darwin.ID) bool {
			ok := false
			if w := window(self); w != nil {
				w.b.guard(func() { ok = w.dragPerform(info) })
			}
			return ok
		}},
		{Sel: darwin.Sel("draggingSession:sourceOperationMaskForDraggingContext:"), Fn: func(self darwin.ID, _ darwin.SEL, _ darwin.ID, _ int) uint {
			if _, d := dragOf(self); d != nil {
				return uint(sourceMask(d.allowed))
			}
			return 0
		}},
		{Sel: darwin.Sel("draggingSession:endedAtPoint:operation:"), Fn: func(self darwin.ID, _ darwin.SEL, _ darwin.ID, _ darwin.CGPoint, op uint) {
			dragEnded(self, uint64(op))
		}},
	}
}
