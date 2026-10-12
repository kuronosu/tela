package headless

import (
	"fmt"
	"image"
	"slices"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

// Window is a headless window. Besides platform.Window it offers an
// injection API (what the user and the window manager would do) and an
// inspection API (what was presented). Both are safe from any goroutine.
type Window struct {
	b    *Backend
	opts platform.WindowOptions

	// Guarded by b.mu.
	title   string
	cursor  platform.Cursor
	os      platform.Metrics // the "OS" state: latest size/scale, maybe not delivered yet
	metrics platform.Metrics // delivered by the latest ConfigureEvent
	focused bool
	closed  bool

	frameRequested bool          // RequestFrame called since the last FrameEvent
	forcedFrame    bool          // a ConfigureEvent was delivered; frame due now
	hasFramed      bool          // at least one FrameEvent delivered
	lastFrame      time.Duration // Time of the last FrameEvent

	needsFull bool // next Present is treated as full (after a ConfigureEvent)
	frame     *image.RGBA
	damage    []image.Rectangle
	presents  int
}

var _ platform.Window = (*Window)(nil)

// --- platform.Window ---

// Metrics returns the metrics of the latest delivered ConfigureEvent.
func (w *Window) Metrics() platform.Metrics {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.metrics
}

// RequestFrame requests a FrameEvent; see platform.Window.
func (w *Window) RequestFrame() {
	w.b.mu.Lock()
	if w.closed {
		w.b.mu.Unlock()
		return
	}
	w.frameRequested = true
	w.b.mu.Unlock()
	w.b.signal()
}

// Present stores a copy of the damaged pixels of fb into the window's
// retained frame; see platform.Window.
func (w *Window) Present(fb *image.RGBA, damage []image.Rectangle) error {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	if w.closed {
		return nil
	}
	if w.metrics.Scale == 0 {
		panic("headless: Present called before the window's first ConfigureEvent was delivered; present from the FrameEvent that follows it")
	}
	platform.CheckFramebuffer(fb, w.metrics)
	full := w.needsFull || w.frame == nil
	rects := platform.ClipDamage(damage, fb.Rect, full)
	if len(rects) == 0 {
		// Every damage rectangle was clipped away: nothing changed, so
		// nothing is presented (like the Wayland and X11 backends). Damage
		// and Presents keep describing the last real Present; recording
		// nil damage here would read as "full frame".
		return nil
	}
	if full {
		w.frame = image.NewRGBA(fb.Rect)
		w.needsFull = false
	}
	for _, r := range rects {
		platform.CopyRGBA(w.frame, fb, r)
	}
	w.damage = rects
	w.presents++
	return nil
}

// SetTitle sets the title.
func (w *Window) SetTitle(title string) {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	if !w.closed {
		w.title = title
	}
}

// SetCursor sets the cursor.
func (w *Window) SetCursor(c platform.Cursor) {
	if !c.Valid() {
		panic(fmt.Sprintf("headless: SetCursor(%d): invalid cursor; use one of the platform.Cursor constants", c))
	}
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	if !w.closed {
		w.cursor = c
	}
}

// SetSize resizes the window. The headless "window manager" grants every
// request, clamped to MinSize; a ConfigureEvent follows if the size changed.
func (w *Window) SetSize(size platform.Size) {
	platform.CheckSize("SetSize size", size)
	w.reconfigure(func(m *platform.Metrics) { m.Size = w.opts.ClampSize(size) })
}

// Close closes the window. Queued events for it are dropped.
func (w *Window) Close() {
	b := w.b
	b.mu.Lock()
	defer b.mu.Unlock()
	if w.closed {
		return
	}
	w.closed = true
	w.frameRequested, w.forcedFrame = false, false
	b.windows = slices.DeleteFunc(b.windows, func(x *Window) bool { return x == w })
	b.queue = slices.DeleteFunc(b.queue, func(it item) bool { return it.w == w })
}

// --- injection (user / window manager actions) ---

// Inject queues an input or window event for this window, as if the OS
// produced it. Accepted: PointerEvent, ScrollEvent, FocusEvent,
// CloseRequestedEvent. A zero Time in pointer and scroll events is set to
// the current clock. Other events have dedicated APIs (Resize, SetScale,
// RequestFrame, Backend.Wake, Backend.SetSettings) and panic here. Events
// injected into a closed window are dropped.
func (w *Window) Inject(e platform.Event) {
	switch ev := e.(type) {
	case platform.PointerEvent:
		if ev.Time == 0 {
			ev.Time = w.b.Now()
		}
		e = ev
	case platform.ScrollEvent:
		if ev.Time == 0 {
			ev.Time = w.b.Now()
		}
		e = ev
	case platform.FocusEvent:
		w.b.mu.Lock()
		w.focused = ev.Focused
		w.b.mu.Unlock()
	case platform.CloseRequestedEvent:
	case nil:
		panic("headless: Inject called with a nil event")
	default:
		panic(fmt.Sprintf("headless: Inject does not accept %T; use Resize/SetScale for ConfigureEvent, RequestFrame and Backend.Advance for FrameEvent, Backend.Wake for WakeEvent, Backend.SetSettings for SettingsEvent", e))
	}
	w.b.mu.Lock()
	if w.closed {
		w.b.mu.Unlock()
		return
	}
	w.b.queue = append(w.b.queue, item{w: w, ev: e})
	w.b.mu.Unlock()
	w.b.signal()
}

// Resize simulates the user or window manager resizing the window to size
// (logical pixels), clamped to MinSize. A ConfigureEvent is queued if the
// size changed.
func (w *Window) Resize(size platform.Size) {
	platform.CheckSize("Resize size", size)
	w.reconfigure(func(m *platform.Metrics) { m.Size = w.opts.ClampSize(size) })
}

// SetScale simulates moving the window to a display with another scale. The
// logical size is kept; a ConfigureEvent is queued if the scale changed.
func (w *Window) SetScale(scale float64) {
	platform.PhysicalSize(platform.Size{}, scale) // validates the scale
	w.reconfigure(func(m *platform.Metrics) { m.Scale = scale })
}

// RequestClose simulates the user clicking the close button: it queues a
// CloseRequestedEvent.
func (w *Window) RequestClose() { w.Inject(platform.CloseRequestedEvent{}) }

// SetFocused simulates the window gaining or losing focus: it queues a
// FocusEvent if the focus state changes.
func (w *Window) SetFocused(focused bool) {
	w.b.mu.Lock()
	same := w.focused == focused
	w.b.mu.Unlock()
	if !same {
		w.Inject(platform.FocusEvent{Focused: focused})
	}
}

func (w *Window) reconfigure(change func(*platform.Metrics)) {
	b := w.b
	b.mu.Lock()
	if w.closed {
		b.mu.Unlock()
		return
	}
	old := w.os
	change(&w.os)
	if w.os == old {
		b.mu.Unlock()
		return
	}
	b.queue = append(b.queue, item{w: w, ev: platform.ConfigureEvent{Metrics: w.os}})
	b.mu.Unlock()
	b.signal()
}

// --- inspection ---

// Frame returns a copy of the window's current content (the result of
// every Present so far), or nil if nothing was presented.
func (w *Window) Frame() *image.RGBA {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	if w.frame == nil {
		return nil
	}
	c := image.NewRGBA(w.frame.Rect)
	copy(c.Pix, w.frame.Pix)
	return c
}

// Damage returns the effective damage of the last Present: the rectangles
// copied, clipped to the frame, with nil or empty input expanded to the full
// frame (and any Present after a ConfigureEvent reported as full).
func (w *Window) Damage() []image.Rectangle {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return slices.Clone(w.damage)
}

// Presents returns how many times Present stored a frame. A Present whose
// damage lies entirely outside the frame stores nothing and is not counted.
func (w *Window) Presents() int {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.presents
}

// Title returns the current title.
func (w *Window) Title() string {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.title
}

// Cursor returns the current cursor.
func (w *Window) Cursor() platform.Cursor {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.cursor
}

// Options returns the resolved options the window was created with.
func (w *Window) Options() platform.WindowOptions { return w.opts }

// Closed reports whether Close was called.
func (w *Window) Closed() bool {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.closed
}

// FrameRequested reports whether a frame request is waiting to be
// delivered.
func (w *Window) FrameRequested() bool {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.frameRequested || w.forcedFrame
}

// frameDueLocked reports whether a FrameEvent should be delivered at now;
// b.mu held.
func (w *Window) frameDueLocked(now time.Duration) bool {
	if w.closed || !w.visibleLocked() {
		return false
	}
	if w.forcedFrame {
		return true
	}
	return w.frameRequested && (!w.hasFramed || now >= w.lastFrame+w.b.opts.FrameInterval)
}

// visibleLocked reports whether the delivered metrics have a non-empty
// physical size; b.mu held.
func (w *Window) visibleLocked() bool {
	if w.metrics.Scale == 0 {
		return false
	}
	p := w.metrics.Physical()
	return p.X > 0 && p.Y > 0
}
