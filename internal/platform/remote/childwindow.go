package remote

import (
	"errors"
	"fmt"
	"image"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/loop"
)

// Buffering
//
// Each window has one shared segment per framebuffer size, split into
// slotsPerWindow slots of one full frame each, and a generation number
// that changes with the segment. Present copies the damaged rectangles of
// the framebuffer into a free slot and sends the slot, the generation, the
// window's metrics and the damage; the slot is then busy. The host's
// reader goroutine copies the same rectangles out of the slot into its
// own retained copy of the frame, under the lock its UI thread presents
// from, and releases the slot. So:
//
//   - the child never writes a slot the host may be reading (it is busy
//     until released);
//   - the host never shows a torn frame (it applies a Present's damage as
//     one step under the lock), and never reads pixels outside the damage,
//     so a slot only needs valid pixels inside the damage of its own
//     Present: no buffer-age repair as in the Wayland swapchain;
//   - Present returns as soon as its pixels are in shared memory; it waits
//     only when both slots are still busy, which takes a host that has not
//     read the previous two frames yet (the read is one memcpy on a
//     goroutine of its own, never behind the host's UI thread).
//
// A size change allocates a new segment (new generation) and sends it
// with a buffers message before the first Present of that size. The old
// segment is unmapped once the host acknowledges the new generation: on
// Windows the mapping lives by name and must outlive the host's open, and
// the acknowledgment proves the host processed every message before it.
// For the same reason a window closed before the host acknowledged its
// latest segment keeps it until then, and Backend.Close waits (at most
// closeTimeout) for the acknowledgments still due before it closes the
// connection and unmaps everything. Releases of an older generation are
// ignored.

// slotsPerWindow is the number of frame slots of a window's segment.
const slotsPerWindow = 2

// releaseTimeout bounds how long Present waits for a busy slot.
const releaseTimeout = 5 * time.Second

// Window is a window of a remote Backend: a proxy for a window of the
// host. Methods follow platform.Window.
type Window struct {
	b  *Backend
	id uint32

	// Guarded by b.mu. The embedded loop state is updated when the loop
	// delivers a ConfigureEvent (metrics, full Present). The window is
	// never registered with the loop (Loop.Add): its FrameEvents come from
	// the host, so the loop never paces it.
	loop.WindowState
	frameAsked bool                     // a RequestFrame was sent and no FrameEvent received since
	textInput  *platform.TextInputState // latest state sent

	seg      *segment
	gen      uint32
	ackedGen uint32 // the newest generation the host acknowledged
	size     image.Point
	busy     []bool
	retired  []retiredSegment
	released chan struct{} // a slot was released; buffer 1
}

type retiredSegment struct {
	gen uint32
	seg *segment
}

var _ platform.Window = (*Window)(nil)

// NewWindow asks the host for a window: on a swap, the host's existing
// window with the same ID (IDs count from 1 in creation order); otherwise a
// new window of the host's backend. Its first event is a ConfigureEvent
// with the host window's metrics. An empty AppID is replaced by this
// executable's DefaultAppID, so the host's window is named after the app.
func (b *Backend) NewWindow(opts platform.WindowOptions) (platform.Window, error) {
	opts = opts.Resolve()
	if opts.AppID == "" {
		opts.AppID = platform.DefaultAppID()
	}
	b.mu.Lock()
	if b.lost != nil {
		err := b.lost
		b.mu.Unlock()
		return nil, err
	}
	b.nextWin++
	w := &Window{b: b, id: b.nextWin, released: make(chan struct{}, 1)}
	b.windows[w.id] = w
	b.mu.Unlock()
	e := newMsg(msgNewWindow)
	e.u32(w.id)
	e.windowOptions(opts)
	if err := b.c.send(e.bytes()); err != nil {
		b.mu.Lock()
		delete(b.windows, w.id)
		b.mu.Unlock()
		return nil, err
	}
	return w, nil
}

// ID returns the window's ID, the key the host maps to its real window.
func (w *Window) ID() uint32 { return w.id }

// Metrics returns the metrics of the latest delivered ConfigureEvent.
func (w *Window) Metrics() platform.Metrics {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.WindowState.Metrics()
}

// RequestFrame asks the host for a FrameEvent; requests coalesce until it
// arrives.
func (w *Window) RequestFrame() {
	w.b.mu.Lock()
	if w.Closed() || w.frameAsked {
		w.b.mu.Unlock()
		return
	}
	w.frameAsked = true
	w.b.mu.Unlock()
	w.send(newMsg(msgRequestFrame))
}

// send sends a window operation that starts with the window ID (e was
// made by newMsg; the ID is inserted here).
func (w *Window) send(e *enc, fill ...func(*enc)) {
	e.u32(w.id)
	for _, f := range fill {
		f(e)
	}
	w.b.c.send(e.bytes())
}

// open reports whether the window is open; it takes b.mu briefly.
func (w *Window) open() bool {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return !w.Closed()
}

// SetTitle sets the title of the host window (shown while this process's
// windows are the host's active ones).
func (w *Window) SetTitle(title string) {
	if w.open() {
		w.send(newMsg(msgSetTitle), func(e *enc) { e.str(title) })
	}
}

// SetCursor sets the cursor. Invalid values panic.
func (w *Window) SetCursor(c platform.Cursor) {
	if !c.Valid() {
		panic(fmt.Sprintf("remote: SetCursor(%d): invalid cursor; use one of the platform.Cursor constants", c))
	}
	if w.open() {
		w.send(newMsg(msgSetCursor), func(e *enc) { e.u8(uint8(c)) })
	}
}

// SetSize asks the host to resize its window.
func (w *Window) SetSize(size platform.Size) {
	platform.CheckSize("SetSize size", size)
	if w.open() {
		w.send(newMsg(msgSetSize), func(e *enc) { e.size(size) })
	}
}

// SetTextInput forwards the focused field's state to the host's IME.
// Equal states are not resent.
func (w *Window) SetTextInput(state *platform.TextInputState) {
	platform.CheckTextInputState(state)
	b := w.b
	b.mu.Lock()
	if w.Closed() || w.textInput.Equal(state) {
		b.mu.Unlock()
		return
	}
	w.textInput = state.Clone()
	b.mu.Unlock()
	w.send(newMsg(msgSetTextInput), func(e *enc) { e.textInput(state) })
}

// ResetTextInput forwards a composition reset. It does nothing without a
// text input state.
func (w *Window) ResetTextInput() {
	b := w.b
	b.mu.Lock()
	skip := w.Closed() || w.textInput == nil
	b.mu.Unlock()
	if !skip {
		w.send(newMsg(msgResetTextInput))
	}
}

// Close closes the window; the host closes its window if this process's
// windows are the active ones.
func (w *Window) Close() {
	b := w.b
	b.mu.Lock()
	if w.Closed() {
		b.mu.Unlock()
		return
	}
	b.loop.Remove(w)
	delete(b.windows, w.id)
	if w.seg != nil && w.ackedGen < w.gen {
		// The host may not have mapped the latest segment yet (on
		// Windows it opens it by name, which needs it alive): keep it
		// until the acknowledgment.
		b.closing[w.id] = w
	} else {
		w.releaseBuffers()
	}
	b.mu.Unlock()
	w.send(newMsg(msgCloseWindow))
}

// releaseBuffers unmaps every segment; b.mu must be held.
func (w *Window) releaseBuffers() {
	w.seg.close()
	w.seg = nil
	for _, r := range w.retired {
		r.seg.close()
	}
	w.retired = nil
	w.size = image.Point{}
	w.busy = nil
}

// acked handles the host's acknowledgment of generation gen: older
// segments are no longer needed by the host. b.mu must be held.
func (w *Window) acked(gen uint32) {
	w.ackedGen = max(w.ackedGen, gen)
	keep := w.retired[:0]
	for _, r := range w.retired {
		if r.gen < gen {
			r.seg.close()
		} else {
			keep = append(keep, r)
		}
	}
	clear(w.retired[len(keep):])
	w.retired = keep
}

// Present copies the damaged pixels of fb into a free shared slot and
// tells the host; see platform.Window and the buffering notes above.
func (w *Window) Present(fb *image.RGBA, damage []image.Rectangle) error {
	b := w.b
	b.mu.Lock()
	defer b.mu.Unlock()
	if w.Closed() {
		return nil
	}
	m := w.WindowState.Metrics()
	if m.Scale == 0 {
		panic("remote: Present called before the window's first ConfigureEvent was delivered; present from the FrameEvent that follows it")
	}
	platform.CheckFramebuffer(fb, m)
	full := w.NeedsFull() || w.size != fb.Rect.Max
	rects := platform.ClipDamage(damage, fb.Rect, full)
	if len(rects) == 0 {
		return nil
	}
	if b.lost != nil {
		return b.lost
	}
	if err := w.ensureBuffers(fb.Rect.Max); err != nil {
		return err
	}
	slot, err := w.freeSlot()
	if err != nil || slot < 0 {
		return err
	}
	stride := 4 * w.size.X
	base := slot * stride * w.size.Y
	dst := w.seg.data[base : base+stride*w.size.Y]
	for _, r := range rects {
		n := 4 * r.Dx()
		for y := r.Min.Y; y < r.Max.Y; y++ {
			so := fb.PixOffset(r.Min.X, y)
			do := y*stride + 4*r.Min.X
			copy(dst[do:do+n], fb.Pix[so:so+n])
		}
	}
	w.busy[slot] = true
	e := newMsg(msgPresent)
	e.u32(w.id)
	e.u32(w.gen)
	e.u8(uint8(slot))
	e.metrics(m)
	e.rects(rects)
	if err := b.c.send(e.bytes()); err != nil {
		return err
	}
	w.ClearNeedsFull()
	return nil
}

// ensureBuffers makes the current segment fit frames of size p; b.mu must
// be held.
func (w *Window) ensureBuffers(p image.Point) error {
	if w.seg != nil && w.size == p {
		return nil
	}
	stride := 4 * p.X
	seg, f, name, err := createSegment(slotsPerWindow * stride * p.Y)
	if err != nil {
		return err
	}
	if w.seg != nil {
		w.retired = append(w.retired, retiredSegment{gen: w.gen, seg: w.seg})
	}
	w.gen++
	w.seg, w.size, w.busy = seg, p, make([]bool, slotsPerWindow)
	e := newMsg(msgBuffers)
	e.u32(w.id)
	e.u32(w.gen)
	e.u32(uint32(p.X))
	e.u32(uint32(p.Y))
	e.u32(uint32(stride))
	e.u8(slotsPerWindow)
	e.str(name)
	if f != nil {
		return w.b.c.sendFile(e.bytes(), f)
	}
	return w.b.c.send(e.bytes())
}

// freeSlot returns a slot the host is not reading, waiting for a release
// if both are busy. b.mu must be held; it is released while waiting. It
// returns -1 and no error if the window closed meanwhile.
func (w *Window) freeSlot() (int, error) {
	b := w.b
	deadline := time.Now().Add(releaseTimeout)
	for {
		for i, busy := range w.busy {
			if !busy {
				return i, nil
			}
		}
		b.mu.Unlock()
		t := time.NewTimer(time.Until(deadline))
		var err error
		select {
		case <-w.released:
		case <-b.c.done:
			err = fmt.Errorf("remote: connection to the host lost: %w", b.c.Err())
		case <-t.C:
			err = errors.New("remote: the host did not release a frame buffer within 5s")
		}
		t.Stop()
		b.mu.Lock()
		if err != nil {
			return -1, err
		}
		if w.Closed() || w.busy == nil {
			return -1, nil
		}
	}
}
