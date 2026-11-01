//go:build linux

package wayland

import (
	"fmt"
	"image"
	"math"
	"slices"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/wayland/protocols"
)

// Drag and drop on wl_data_device, the clipboard's device (clipboard.go).
// See "Drag and drop and the clipboard" in package platform for the
// contract; how it maps onto the protocol, and the quirks handled:
//
//   - Target. data_offer and its offer events come before enter, and
//     (version 3) source_actions too; enter, motion, leave and drop are
//     the session's DragEnter, DragOver, DragLeave and DragDrop. The app's
//     answer (SetDropEffect) is sent after the handlers of a delivery pass
//     ran (syncDrops, before the loop flushes): wl_data_offer.accept with
//     the enter's serial and the first wanted target, or null to refuse,
//     and set_actions(effect, effect). Answers go out only when they
//     change, so a burst of motion coalesced by QueueDrag costs one
//     request at most.
//   - Compositors drop only on a target that accepted a type and agreed
//     on an action (wlroots, Mutter); otherwise the release is a leave. A
//     drop that races with a refusal still in flight is turned into a
//     DragLeave here, as the contract says.
//   - Every compositor sends leave right after drop (the drag's focus
//     goes away); it is ignored, so a drop is never followed by a leave.
//     The offer lives on after drop while the wanted types are read
//     through pipes (the clipboard's transfers), then DragDrop is
//     delivered, then finish (version 3, when the drop was accepted and
//     the compositor sent an action: Mutter rejects a finish before it)
//     and destroy. A drop the app refuses while handling it is destroyed
//     without finish, so the source sees cancelled and keeps its data.
//   - Source. start_drag needs the serial of the button press that holds
//     the implicit grab (compositors check it against the button still
//     held), kept per seat (seat.pressSerial). wl_data_source.set_actions
//     must precede start_drag; Wayland has no link action, so a drag that
//     allows only DropLink offers no action and can only be cancelled.
//   - The compositor takes the pointer (wl_pointer.leave follows
//     start_drag on wlroots and Mutter, but not in a fixed order with the
//     data device's events), so StartDrag ends the app's pointer sequence
//     itself: PointerCancel and PointerLeave, then the seat's pointer
//     events are ignored until the compositor's leave or next enter. A
//     drag the compositor refuses at once (the button was released
//     before start_drag arrived) sends no leave: the pointer is given back
//     with a PointerEnter when the drag ends.
//   - Our own drags are recognized on the target side by the source's
//     marker type (FromApp), and their data is read from memory.

// dropAnswer is the app's answer to a drag session (SetDropEffect).
type dropAnswer struct {
	effect platform.DropEffect
	types  []string
}

// dndSession is a drag over one of our windows, from its enter to its
// leave or drop (target side).
type dndSession struct {
	offer   *dataOffer
	serial  uint32 // the enter's, for accept
	w       *Window
	pos     platform.Point
	mods    platform.Modifiers
	allowed platform.DropEffect
	types   []string // normalized (offerTypes)
	fromApp bool
	answer  dropAnswer

	// The answer as last sent to the compositor: the accepted target ("":
	// null) and the action; sent is false before the first.
	sent       bool
	sentTarget string
	sentAction protocols.WlDataDeviceManagerDndAction
}

func (s *dndSession) event(a platform.DragAction, t time.Duration) platform.DragEvent {
	return platform.DragEvent{Action: a, Pos: s.pos, Types: slices.Clone(s.types), Allowed: s.allowed,
		Mods: s.mods, FromApp: s.fromApp, Time: t}
}

// effect is the app's answer as an effect the source allows (DropNone
// otherwise).
func (s *dndSession) effect() platform.DropEffect {
	if s.allowed.Has(s.answer.effect) {
		return s.answer.effect
	}
	return platform.DropNone
}

// acceptTarget is the target to accept: the best target of the first type
// the app wants that is offered (with no types named, the first
// well-known type offered); when nothing wanted is offered, the first
// offered type, so the drop still happens (with no data). "" for an offer
// without types.
func (s *dndSession) acceptTarget() string {
	for _, t := range s.answer.types {
		if slices.Contains(s.types, t) {
			return targetFor(s.offer.mimes, t)
		}
	}
	if len(s.answer.types) == 0 {
		if want := platform.DropTypes(s.types, nil); len(want) > 0 {
			return targetFor(s.offer.mimes, want[0])
		}
	}
	if len(s.types) > 0 {
		return targetFor(s.offer.mimes, s.types[0])
	}
	return ""
}

// sync sends the app's answer if it changed since it was last sent.
func (s *dndSession) sync() {
	if s.offer.wl.Destroyed() {
		return
	}
	effect, target := s.effect(), ""
	if effect != platform.DropNone {
		target = s.acceptTarget()
	}
	if target == "" {
		effect = platform.DropNone
	}
	action := dndActions(effect)
	if s.sent && target == s.sentTarget && action == s.sentAction {
		return
	}
	s.sent, s.sentTarget, s.sentAction = true, target, action
	if target == "" {
		s.offer.wl.Accept(s.serial, nil)
	} else {
		s.offer.wl.Accept(s.serial, &target)
	}
	if s.offer.wl.Version() >= 3 {
		s.offer.wl.SetActions(action, action)
	}
}

// offerAllowed is the set of effects the source of a drag offer allows:
// its source_actions (version 3), copy before version 3 (no actions).
func offerAllowed(o *dataOffer) platform.DropEffect {
	if o.wl.Version() < 3 {
		return platform.DropCopy
	}
	return dropEffects(o.actions)
}

// dropEffects maps Wayland dnd actions to effects (ask is ignored).
func dropEffects(a protocols.WlDataDeviceManagerDndAction) platform.DropEffect {
	var e platform.DropEffect
	if a&protocols.WlDataDeviceManagerDndActionCopy != 0 {
		e |= platform.DropCopy
	}
	if a&protocols.WlDataDeviceManagerDndActionMove != 0 {
		e |= platform.DropMove
	}
	return e
}

// dndActions maps effects to Wayland dnd actions (link has none).
func dndActions(e platform.DropEffect) protocols.WlDataDeviceManagerDndAction {
	var a protocols.WlDataDeviceManagerDndAction
	if e&platform.DropCopy != 0 {
		a |= protocols.WlDataDeviceManagerDndActionCopy
	}
	if e&platform.DropMove != 0 {
		a |= protocols.WlDataDeviceManagerDndActionMove
	}
	return a
}

// --- target side ---

func (d *dataDevice) onEnter(e protocols.WlDataDeviceEnterEvent) {
	d.leaveDrag() // a missed leave
	o := d.take(e.Id)
	if o == nil {
		return // a drag without data (a client's own drag with a null source)
	}
	b := d.s.b
	w := b.surfaces[e.Surface]
	if w == nil {
		o.destroy() // not ours (a drag icon), or already destroyed
		return
	}
	s := &dndSession{
		offer:   o,
		serial:  e.Serial,
		w:       w,
		pos:     platform.Point{X: float32(e.X), Y: float32(e.Y)},
		mods:    d.s.ptr.mods,
		allowed: offerAllowed(o),
		types:   offerTypes(o.mimes),
	}
	s.fromApp = b.drag != nil && o.has(b.drag.src.marker)
	d.dnd = s
	b.loop.QueueDrag(w, s.event(platform.DragEnter, b.Now()))
}

func (d *dataDevice) onMotion(e protocols.WlDataDeviceMotionEvent) {
	s := d.dnd
	if s == nil {
		return
	}
	p := platform.Point{X: float32(e.X), Y: float32(e.Y)}
	if p == s.pos {
		return // nothing changed: no DragOver (the contract's rule)
	}
	s.pos = p
	b := d.s.b
	b.loop.QueueDrag(s.w, s.event(platform.DragOver, b.eventTime(e.Time)))
}

// sourceActionsChanged reports a change of the allowed effects of the
// drag over a window as a DragOver.
func (d *dataDevice) sourceActionsChanged(o *dataOffer) {
	s := d.dnd
	if s == nil || s.offer != o {
		return // not entered yet: enter reads them
	}
	if a := offerAllowed(o); a != s.allowed {
		s.allowed = a
		b := d.s.b
		b.loop.QueueDrag(s.w, s.event(platform.DragOver, b.Now()))
	}
}

// modsChanged reports a change of the seat's modifiers during a drag over
// a window as a DragOver (kbd.updatePointerMods).
func (d *dataDevice) modsChanged() {
	s := d.dnd
	if s == nil || s.mods == d.s.ptr.mods {
		return
	}
	s.mods = d.s.ptr.mods
	b := d.s.b
	b.loop.QueueDrag(s.w, s.event(platform.DragOver, b.Now()))
}

// leaveDrag ends the session with DragLeave: the drag left, was cancelled,
// or the release was refused. A leave after a drop finds no session.
func (d *dataDevice) leaveDrag() {
	s := d.dnd
	if s == nil {
		return
	}
	d.dnd = nil
	s.offer.destroy()
	b := d.s.b
	b.loop.QueueDrag(s.w, s.event(platform.DragLeave, b.Now()))
}

// onDrop reads the types the app wants, then delivers DragDrop
// (deliverDrop). The session ends here: the leave that follows is
// ignored.
func (d *dataDevice) onDrop() {
	s := d.dnd
	if s == nil {
		return
	}
	b := d.s.b
	effect := s.effect()
	if effect == platform.DropNone || s.acceptTarget() == "" {
		// The app refused after the compositor saw an acceptance: the
		// drop is a leave (no finish, so the source sees cancelled).
		d.leaveDrag()
		return
	}
	d.dnd = nil
	want := platform.DropTypes(s.types, s.answer.types)
	if s.fromApp && b.drag != nil && s.offer.has(b.drag.src.marker) {
		b.deliverDrop(s, effect, b.drag.src.data.Select(want).Clone())
		return
	}
	if len(want) == 0 {
		b.deliverDrop(s, effect, nil)
		return
	}
	got := make([][]byte, len(want))
	ok := make([]bool, len(want))
	pending := len(want)
	for i, t := range want {
		target := targetFor(s.offer.mimes, t)
		b.readOffer(s.offer, target, func(data []byte, err error) {
			if err == nil {
				got[i], ok[i] = convertRead(t, target, data), true
			}
			// A type the source failed to deliver (an error, the
			// timeout) is left out.
			if pending--; pending > 0 {
				return
			}
			var out platform.Data
			for j, t := range want {
				if ok[j] {
					out = append(out, platform.DataEntry{Type: t, Bytes: got[j]})
				}
			}
			b.deliverDrop(s, effect, out)
		})
	}
}

// deliverDrop queues the DragDrop with its data, between two callbacks:
// the first makes SetDropEffect calls of its handler set the session's
// final effect, the second ends the offer with that effect (finishDrop).
func (b *Backend) deliverDrop(s *dndSession, effect platform.DropEffect, data platform.Data) {
	w := s.w
	if w.Closed() {
		s.offer.destroy()
		return
	}
	ev := s.event(platform.DragDrop, b.Now())
	ev.Data = data
	b.loop.Post(func() { w.dropping = s })
	b.loop.QueueDrag(w, ev)
	b.loop.Post(func() {
		if w.dropping == s {
			w.dropping = nil
		}
		b.finishDrop(s, effect)
	})
}

// finishDrop tells the source the outcome, the effect the window holds
// once it handled the drop: finish with that action (set_actions first if
// the handler changed it), or, for DropNone, a destroy without finish.
func (b *Backend) finishDrop(s *dndSession, atDrop platform.DropEffect) {
	o := s.offer
	if o.wl.Destroyed() {
		return
	}
	final := s.effect()
	if s.w.Closed() {
		final = platform.DropNone
	}
	if o.wl.Version() >= 3 && final != platform.DropNone && o.action != 0 {
		if a := dndActions(final); a != dndActions(atDrop) {
			o.wl.SetActions(a, a)
		}
		o.wl.Finish()
	}
	o.destroy()
}

// syncDrops sends the answers of the drags over our windows (run after
// each delivery pass).
func (b *Backend) syncDrops() {
	for _, s := range b.seats {
		if s.data != nil && s.data.dnd != nil {
			s.data.dnd.sync()
		}
	}
}

// closeDrags ends the drag sessions over a window that is closing, without
// events (the window gets none from now on).
func (w *Window) closeDrags() {
	for _, s := range w.b.seats {
		if d := s.data; d != nil && d.dnd != nil && d.dnd.w == w {
			d.dnd.offer.destroy()
			d.dnd = nil
		}
	}
	w.dropping = nil
}

// SetDropEffect records the window's answer to the drag over it, sent to
// the compositor after the handler returns (syncDrops); while the window
// handles a DragDrop it sets the drop's final effect (finishDrop). Without
// a drag over the window it does nothing.
func (w *Window) SetDropEffect(effect platform.DropEffect, types ...string) {
	platform.CheckSetDropEffect(effect, types)
	if w.Closed() {
		return
	}
	a := dropAnswer{effect: effect, types: slices.Clone(types)}
	if w.dropping != nil {
		w.dropping.answer = a
		return
	}
	for _, s := range w.b.seats {
		if d := s.data; d != nil && d.dnd != nil && d.dnd.w == w {
			d.dnd.answer = a
		}
	}
}

// --- source side ---

// dragSource is the drag this app started (StartDrag); one at a time.
type dragSource struct {
	src  *dataSource
	w    *Window
	seat *seat
	// action is the latest action the compositor negotiated (the
	// source's action event), reported by dnd_finished.
	action protocols.WlDataDeviceManagerDndAction

	// The icon: its surface, viewport (fractional scales) and buffer,
	// the surface's logical size and the hotspot in surface coordinates.
	icon        *protocols.WlSurface
	iconVP      *protocols.WpViewport
	iconBuf     *shmBuffer
	iconLogical image.Point
	iconHot     image.Point
}

// StartDrag starts a drag of opts.Data with wl_data_device.start_drag
// from the press that the window's seat holds (else platform.ErrNoGrab).
// The window gets PointerCancel and PointerLeave at once; DragDoneEvent
// follows when the compositor ends the drag. Without a data device of
// version 3 it returns an error wrapping platform.ErrUnsupported.
func (w *Window) StartDrag(opts platform.DragOptions) error {
	platform.CheckDragOptions(opts)
	b := w.b
	if b.dataManager == nil {
		return errNoDataDevice
	}
	if w.Closed() || b.drag != nil {
		return platform.ErrNoGrab
	}
	var s *seat
	for _, x := range b.seats {
		if x.pointer != nil && x.ptr.focus == w && x.ptr.buttons != 0 && !x.ptr.taken {
			s = x
			break
		}
	}
	if s == nil {
		return platform.ErrNoGrab
	}
	d := s.data
	if d == nil {
		return errNoDataDevice
	}
	if d.wl.Version() < 3 {
		return fmt.Errorf("wayland: drag and drop needs wl_data_device_manager version 3, the compositor has %d: %w", d.wl.Version(), platform.ErrUnsupported)
	}
	ds := &dragSource{w: w, seat: s}
	ds.src = b.newDataSource(opts.Data.Clone())
	ds.src.wl.SetActions(dndActions(opts.Allowed)) // before start_drag (the protocol's rule)
	ds.src.wl.SetActionHandler(func(e protocols.WlDataSourceActionEvent) { ds.action = e.DndAction })
	ds.src.wl.SetCancelledHandler(func(protocols.WlDataSourceCancelledEvent) { b.endDrag(ds, platform.DropNone) })
	ds.src.wl.SetDndFinishedHandler(func(protocols.WlDataSourceDndFinishedEvent) {
		b.endDrag(ds, dropEffects(ds.action))
	})
	// dnd_drop_performed needs nothing: the source stays alive until
	// dnd_finished or cancelled, which follow it.
	icon := ds.makeIcon(opts.Image, opts.Hotspot)
	d.wl.StartDrag(ds.src.wl, w.surface, icon, s.pressSerial)
	ds.commitIcon()
	b.drag = ds
	s.takePointer()
	return nil
}

// endDrag ends our drag: the source and icon are destroyed, the source
// window gets DragDoneEvent, and a pointer still taken (no leave came) is
// given back with PointerEnter.
func (b *Backend) endDrag(ds *dragSource, effect platform.DropEffect) {
	if b.drag != ds {
		return
	}
	b.drag = nil
	if !ds.src.wl.Destroyed() {
		ds.src.wl.Destroy()
	}
	ds.destroyIcon()
	b.enqueue(ds.w, platform.DragDoneEvent{Effect: effect})
	if p := &ds.seat.ptr; p.taken && p.focus == ds.w {
		p.taken = false
		p.emit(platform.PointerEnter, platform.ButtonNone, b.Now())
		ds.seat.flushPointer()
	}
}

// makeIcon creates the drag icon's surface holding img (premultiplied
// RGBA in physical pixels of the window's scale) as an ARGB8888 buffer:
// with a viewport at a scale other than 1 when the compositor has one,
// else with an integer buffer scale when the image divides by it, else at
// scale 1. It returns nil without an image, or if the buffer cannot be
// made (the image is a hint).
func (ds *dragSource) makeIcon(img *image.RGBA, hot image.Point) *protocols.WlSurface {
	if img == nil {
		return nil
	}
	w := ds.w
	b := w.b
	size := img.Rect.Size()
	buf, err := allocShm(b.shm, protocols.WlShmFormatArgb8888, size)
	if err != nil {
		return nil
	}
	src := &image.RGBA{Pix: img.Pix[img.PixOffset(img.Rect.Min.X, img.Rect.Min.Y):], Stride: img.Stride, Rect: image.Rectangle{Max: size}}
	platform.CopyRGBAToBGRA(buf.data, buf.stride, src, src.Rect)
	surface := b.compositor.CreateSurface()
	scale := float64(w.Metrics().Scale)
	logical, hotspot := size, hot
	div := func(v int, s float64) int { return int(math.Round(float64(v) / s)) }
	switch n := int(scale); {
	case scale <= 0 || scale == 1:
	case b.viewporter != nil:
		ds.iconVP = b.viewporter.GetViewport(surface)
		logical = image.Pt(max(1, div(size.X, scale)), max(1, div(size.Y, scale)))
		ds.iconVP.SetDestination(int32(logical.X), int32(logical.Y))
		hotspot = image.Pt(div(hot.X, scale), div(hot.Y, scale))
	case float64(n) == scale && surface.Version() >= 3 && size.X%n == 0 && size.Y%n == 0:
		surface.SetBufferScale(int32(n))
		logical = size.Div(n)
		hotspot = image.Pt(div(hot.X, scale), div(hot.Y, scale))
	}
	ds.icon, ds.iconBuf, ds.iconLogical, ds.iconHot = surface, buf, logical, hotspot
	return surface
}

// commitIcon shows the icon's buffer, its top-left at -hotspot from the
// pointer (the dnd icon role places the surface's origin at the pointer):
// wl_surface.offset from version 5, the attach offset before.
func (ds *dragSource) commitIcon() {
	s := ds.icon
	if s == nil {
		return
	}
	hx, hy := int32(-ds.iconHot.X), int32(-ds.iconHot.Y)
	if s.Version() >= 5 {
		s.Attach(ds.iconBuf.wl, 0, 0)
		if hx != 0 || hy != 0 {
			s.Offset(hx, hy)
		}
	} else {
		s.Attach(ds.iconBuf.wl, hx, hy)
	}
	if s.Version() >= 4 {
		s.DamageBuffer(0, 0, int32(ds.iconBuf.size.X), int32(ds.iconBuf.size.Y))
	} else {
		s.Damage(0, 0, int32(ds.iconLogical.X), int32(ds.iconLogical.Y))
	}
	s.Commit()
}

func (ds *dragSource) destroyIcon() {
	if ds.icon == nil {
		return
	}
	if ds.iconVP != nil {
		ds.iconVP.Destroy()
		ds.iconVP = nil
	}
	ds.icon.Destroy()
	ds.icon = nil
	ds.iconBuf.destroy()
	ds.iconBuf = nil
}

// takePointer ends the app's pointer sequence on the seat's focused window
// when a drag takes the pointer: pointer events of the window still
// queued are dropped, PointerCancel and PointerLeave are queued, and the
// seat's pointer events are ignored until the compositor's leave or next
// enter (pointerState.taken).
func (s *seat) takePointer() {
	s.flushPointer()
	p := &s.ptr
	w := p.focus
	s.b.loop.Extract(w, func(e platform.Event) bool { _, ok := e.(platform.PointerEvent); return ok })
	now := s.b.Now()
	p.osHeld, p.captionPending, p.edge, p.finger = 0, false, 0, false
	p.buttons = 0
	p.emit(platform.PointerCancel, platform.ButtonNone, now)
	p.emit(platform.PointerLeave, platform.ButtonNone, now)
	p.taken = true
	s.flushPointer()
}
