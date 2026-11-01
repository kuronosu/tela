package headless

import (
	"fmt"
	"image"
	"image/draw"
	"slices"

	"github.com/kuronosu/tela/internal/platform"
)

// dragState is the drag in progress, guarded by b.mu.
type dragState struct {
	data    platform.Data
	types   []string
	allowed platform.DropEffect
	source  *Window // nil for another application's drag
	image   *image.RGBA
	hotspot image.Point
	over    *Window // the window the drag is over, or nil
	pos     platform.Point
	mods    platform.Modifiers
}

// dropAnswer is a window's latest SetDropEffect in the current session.
type dropAnswer struct {
	effect platform.DropEffect
	types  []string
}

// DragInfo describes the drag in progress (Backend.Drag).
type DragInfo struct {
	Data    platform.Data
	Allowed platform.DropEffect
	// Source is the window that started it, nil for another
	// application's drag (StartExternalDrag).
	Source *Window
	// Image and Hotspot are the source's drag image (a copy), or nil.
	Image   *image.RGBA
	Hotspot image.Point
	// Over is the window the drag is over, or nil.
	Over *Window
	Pos  platform.Point
}

// Drag returns the drag in progress.
func (b *Backend) Drag() (DragInfo, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	d := b.drag
	if d == nil {
		return DragInfo{}, false
	}
	return DragInfo{Data: d.data.Clone(), Allowed: d.allowed, Source: d.source, Image: d.image, Hotspot: d.hotspot, Over: d.over, Pos: d.pos}, true
}

// StartExternalDrag starts another application's drag of data, allowing
// the effects allowed: it is over no window until a pointer event is
// injected into one (see the package documentation). It panics while a
// drag is in progress, or for empty or invalid data. Safe from any
// goroutine.
func (b *Backend) StartExternalDrag(data platform.Data, allowed platform.DropEffect) {
	platform.CheckDragOptions(platform.DragOptions{Data: data, Allowed: allowed})
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.drag != nil {
		panic("headless: StartExternalDrag while a drag is in progress; end it first (a release, CancelDrag, DropOutside)")
	}
	data = data.Clone()
	b.drag = &dragState{data: data, types: data.Types(), allowed: allowed}
}

// CancelDrag ends the drag in progress as the user pressing Escape would:
// a DragLeave for the window it is over, and DragDoneEvent with DropNone
// for an app drag's source. It panics without a drag. Safe from any
// goroutine.
func (b *Backend) CancelDrag() { b.endDrag("CancelDrag", platform.DropNone) }

// DropOutside ends an app drag over another application, which performed
// effect (one of the allowed effects, or DropNone for a refusal): a
// DragLeave for the window it is over, and the source's DragDoneEvent with
// effect. It panics without an app drag. Safe from any goroutine.
func (b *Backend) DropOutside(effect platform.DropEffect) {
	platform.CheckDropEffect("DropOutside", effect, true)
	b.endDrag("DropOutside", effect)
}

func (b *Backend) endDrag(what string, effect platform.DropEffect) {
	b.mu.Lock()
	defer b.mu.Unlock()
	d := b.drag
	switch {
	case d == nil:
		panic("headless: " + what + " without a drag in progress")
	case what == "DropOutside" && d.source == nil:
		panic("headless: DropOutside of another application's drag; use CancelDrag")
	case effect != platform.DropNone && !d.allowed.Has(effect):
		panic(fmt.Sprintf("headless: DropOutside(%v): the drag allows only %v", effect, d.allowed))
	}
	if d.over != nil {
		b.dragLeave(d.over)
	}
	b.finishDrag(effect)
	b.signal()
}

// finishDrag ends the drag and queues the source's outcome; b.mu must be
// held.
func (b *Backend) finishDrag(effect platform.DropEffect) {
	if src := b.drag.source; src != nil {
		b.loop.Enqueue(src, platform.DragDoneEvent{Effect: effect})
	}
	b.drag = nil
}

// dragEvent builds a DragEvent of the drag in progress; b.mu must be held.
func (b *Backend) dragEvent(action platform.DragAction) platform.DragEvent {
	d := b.drag
	return platform.DragEvent{Action: action, Pos: d.pos, Types: slices.Clone(d.types), Allowed: d.allowed,
		Mods: d.mods, FromApp: d.source != nil, Time: b.now()}
}

// dragLeave queues a DragLeave for w, which the drag is over; b.mu must
// be held.
func (b *Backend) dragLeave(w *Window) {
	b.loop.QueueDrag(w, b.dragEvent(platform.DragLeave))
	b.drag.over = nil
	w.drop = dropAnswer{}
}

// dragTo moves the drag over w at pos (entering it if needed); b.mu must
// be held.
func (b *Backend) dragTo(w *Window, pos platform.Point, mods platform.Modifiers) {
	d := b.drag
	if d.over != w {
		if d.over != nil {
			b.dragLeave(d.over)
		}
		d.over, d.pos, d.mods = w, pos, mods
		w.drop = dropAnswer{}
		b.loop.QueueDrag(w, b.dragEvent(platform.DragEnter))
		return
	}
	if d.pos == pos && d.mods == mods {
		return // nothing changed: no DragOver (the contract's rule)
	}
	d.pos, d.mods = pos, mods
	b.loop.QueueDrag(w, b.dragEvent(platform.DragOver))
}

// dragPointer handles a pointer event injected into w while a drag is in
// progress: the "OS" moves, drops or cancels the drag, and the app gets
// no pointer event. b.mu must be held and w open.
func (w *Window) dragPointer(e platform.PointerEvent) {
	b := w.b
	d := b.drag
	switch e.Action {
	case platform.PointerMove, platform.PointerEnter:
		b.dragTo(w, e.Pos, e.Mods)
	case platform.PointerLeave:
		if d.over == w {
			b.dragLeave(w)
		}
	case platform.PointerUp:
		b.dragTo(w, e.Pos, e.Mods)
		b.dropOn(w)
	case platform.PointerCancel:
		if d.over != nil {
			b.dragLeave(d.over)
		}
		b.finishDrag(platform.DropNone)
	}
	b.signal()
}

// dropOn releases the drag over w: a DragDrop with the data w asked for
// when its answer accepts the drag (an allowed effect), else a DragLeave;
// either ends the drag. The outcome, the effect w holds once it handled
// the drop, goes to w's Drops and the source's DragDoneEvent. b.mu must be
// held.
func (b *Backend) dropOn(w *Window) {
	d := b.drag
	if !d.allowed.Has(w.drop.effect) {
		b.dragLeave(w)
		b.finishDrag(platform.DropNone)
		return
	}
	ev := b.dragEvent(platform.DragDrop)
	ev.Data = d.data.Select(platform.DropTypes(d.types, w.drop.types)).Clone()
	b.loop.QueueDrag(w, ev)
	src, allowed := d.source, d.allowed
	b.drag = nil
	w.dropping = true // SetDropEffect sets the final effect while the drop is handled
	b.loop.Post(func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		w.dropping = false
		effect := w.drop.effect
		if !allowed.Has(effect) {
			effect = platform.DropNone
		}
		w.drop = dropAnswer{}
		w.drops = append(w.drops, effect)
		if src != nil {
			b.loop.Enqueue(src, platform.DragDoneEvent{Effect: effect})
			b.signal()
		}
	})
}

// --- platform.Window ---

// SetDropEffect records the window's answer to the drag over it
// (DropAnswer); it does nothing without one.
func (w *Window) SetDropEffect(effect platform.DropEffect, types ...string) {
	platform.CheckSetDropEffect(effect, types)
	b := w.b
	b.mu.Lock()
	defer b.mu.Unlock()
	if !w.dropping && (b.drag == nil || b.drag.over != w) {
		return
	}
	w.drop = dropAnswer{effect: effect, types: slices.Clone(types)}
}

// StartDrag starts an app drag from the window while a pointer button
// pressed in it is held, as the delivered pointer events say (else
// platform.ErrNoGrab): the window gets a PointerCancel and the drag enters
// it at the pointer. Pointer events of the window injected but not yet
// delivered move the drag instead (a release queued behind the move that
// started the drag drops it). The data and image are copied (Drag).
// ServiceDragDrop unsupported returns platform.ErrUnsupported.
func (w *Window) StartDrag(opts platform.DragOptions) error {
	platform.CheckDragOptions(opts)
	if !w.b.supports(ServiceDragDrop) {
		return platform.ErrUnsupported
	}
	b := w.b
	b.mu.Lock()
	defer b.mu.Unlock()
	if w.WindowState.Closed() || b.drag != nil || w.buttons == 0 {
		return platform.ErrNoGrab
	}
	var img *image.RGBA
	if opts.Image != nil {
		img = image.NewRGBA(image.Rect(0, 0, opts.Image.Rect.Dx(), opts.Image.Rect.Dy()))
		draw.Draw(img, img.Rect, opts.Image, opts.Image.Rect.Min, draw.Src)
	}
	data := opts.Data.Clone()
	// The rest of the sequence a test injected already belongs to the
	// drag, as it would on a real OS (which owns the pointer from now on).
	rest := b.loop.Extract(w, func(e platform.Event) bool { _, ok := e.(platform.PointerEvent); return ok })
	b.loop.Enqueue(w, platform.PointerEvent{Action: platform.PointerCancel, Pos: w.pointer, Mods: w.mods, Time: b.now()})
	w.buttons = 0
	b.drag = &dragState{data: data, types: data.Types(), allowed: opts.Allowed, source: w, image: img, hotspot: opts.Hotspot}
	b.dragTo(w, w.pointer, w.mods)
	w.replayDrag(b.drag, rest)
	b.signal()
	return nil
}

// replayDrag feeds pointer events the test injected before the drag
// started to the drag, one per pass, each after the app handled the drag
// event of the previous one (the user releases after the app answered the
// enter). Once the drag ends, the rest is a new sequence and goes to the
// app again. b.mu must be held.
func (w *Window) replayDrag(d *dragState, rest []platform.Event) {
	if len(rest) == 0 {
		return
	}
	b := w.b
	b.loop.Post(func() {
		b.mu.Lock()
		defer b.mu.Unlock()
		if b.drag != d {
			for _, e := range rest {
				b.loop.Enqueue(w, e) // dropped if w closed
			}
			b.signal()
			return
		}
		if !w.WindowState.Closed() {
			w.dragPointer(rest[0].(platform.PointerEvent))
		}
		w.replayDrag(d, rest[1:])
	})
}

// --- inspection ---

// DropAnswer returns the window's latest SetDropEffect while a drag is
// over it (DropNone and no types before one).
func (w *Window) DropAnswer() (platform.DropEffect, []string) {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.drop.effect, slices.Clone(w.drop.types)
}

// Drops returns the final effect of every drop on the window, in order:
// the effect it accepted, as it stood once it had handled the DragDrop.
func (w *Window) Drops() []platform.DropEffect {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return slices.Clone(w.drops)
}

// draggedPointer reports whether a drag in progress took e (dragPointer).
func (w *Window) draggedPointer(e platform.PointerEvent) bool {
	b := w.b
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.drag == nil || w.WindowState.Closed() {
		return false
	}
	w.dragPointer(e)
	return true
}
