package headless

import (
	"fmt"
	"image"
	"slices"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/internal/keyboard"
	"github.com/kuronosu/tela/internal/platform/internal/loop"
)

// Window is a headless window. Besides platform.Window it offers an
// injection API (what the user and the window manager would do) and an
// inspection API (what was presented). Both are safe from any goroutine.
type Window struct {
	b    *Backend
	opts platform.WindowOptions

	// Guarded by b.mu, like the embedded loop state (delivered metrics,
	// closed, frame requests, full-Present flag).
	loop.WindowState
	title    string
	cursor   platform.Cursor
	os       platform.Metrics // the "OS" state: latest size/scale, maybe not delivered yet
	focused  bool
	held     keyboard.Held // keys pressed and not released, for synthetic releases
	ime      imeState
	frame    *image.RGBA
	damage   []image.Rectangle
	presents int
	ctl      windowCtl // window control state (windowctl.go)
}

var _ platform.Window = (*Window)(nil)

// --- platform.Window ---

// Metrics returns the metrics of the latest delivered ConfigureEvent.
func (w *Window) Metrics() platform.Metrics {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.WindowState.Metrics()
}

// RequestFrame requests a FrameEvent; see platform.Window.
func (w *Window) RequestFrame() {
	w.b.mu.Lock()
	if w.WindowState.Closed() {
		w.b.mu.Unlock()
		return
	}
	w.WindowState.RequestFrame()
	w.b.mu.Unlock()
	w.b.signal()
}

// Present stores a copy of the damaged pixels of fb into the window's
// retained frame; see platform.Window.
func (w *Window) Present(fb *image.RGBA, damage []image.Rectangle) error {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	if w.WindowState.Closed() {
		return nil
	}
	m := w.WindowState.Metrics()
	if m.Scale == 0 {
		panic("headless: Present called before the window's first ConfigureEvent was delivered; present from the FrameEvent that follows it")
	}
	platform.CheckFramebuffer(fb, m)
	full := w.NeedsFull() || w.frame == nil
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
		w.ClearNeedsFull()
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
	if !w.WindowState.Closed() {
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
	if !w.WindowState.Closed() {
		w.cursor = c
	}
}

// SetSize resizes the window. The headless "window manager" grants every
// request, clamped to the size limits; a ConfigureEvent follows if the size
// changed. While the window is maximized or fullscreen, the size is the one
// it returns to (like GTK's default size).
func (w *Window) SetSize(size platform.Size) {
	platform.CheckSize("SetSize size", size)
	b := w.b
	b.mu.Lock()
	big := w.ctl.big()
	if big && !w.WindowState.Closed() {
		w.ctl.restore = w.ctl.clamp(size)
	}
	b.mu.Unlock()
	if !big {
		w.reconfigure(func(m *platform.Metrics) { m.Size = w.ctl.resized(size) })
	}
}

// Close closes the window, after its child windows (in creation order).
// Queued events for them are dropped.
func (w *Window) Close() {
	b := w.b
	b.mu.Lock()
	defer b.mu.Unlock()
	w.closeLocked()
}

// closeLocked closes w and its children; b.mu must be held.
func (w *Window) closeLocked() {
	if w.WindowState.Closed() {
		return
	}
	for _, c := range slices.Clone(w.b.loop.Windows()) {
		if c.ctl.parent == w {
			c.closeLocked()
		}
	}
	w.b.loop.Remove(w)
}

// --- injection (user / window manager actions) ---

// Inject queues an input or window event for this window, as if the OS
// produced it. Accepted: PointerEvent, ScrollEvent, KeyEvent, IMEEvent,
// FocusEvent, CloseRequestedEvent. A zero Time in pointer, scroll and key
// events is set to the current clock. Other events have dedicated APIs
// (Resize, SetScale, SetState for WindowStateEvent, RequestFrame,
// Backend.Wake, Backend.SetSettings, Backend.InjectApp, SetTextInput for
// IMESessionEvent) and panic here. Events injected into a closed window are
// dropped.
//
// Pointer presses in a caption hit region (SetHitRegions) are the "OS's",
// as on a real backend: the press and every pointer event until its
// release are not delivered; a primary press starts a move (counted by
// CaptionDrags), two primary presses within the double-click interval
// toggle maximize, and a secondary press opens the window menu (counted by
// WindowMenus).
//
// Key events follow the platform keyboard contract, and Inject enforces it
// so that tests describe sequences a real backend can produce: the window
// must have focus (SetFocused(true) first); a press of a held key must be
// a Repeat, and a repeat or a release must be for a held key; releases
// carry no Text and no Repeat; Synthetic is reserved for the backend.
// Inject does not derive any field: Code, Key, Text, Location and Mods are
// delivered as given (Code.Location() gives the Location of a physical
// key). Losing focus (SetFocused(false) or an injected FocusEvent) queues a
// synthetic release for every held key before the FocusEvent.
//
// IME events follow the platform text input contract: the window must
// have an active IME session (focus and a state set with SetTextInput),
// the event must pass platform.CheckIMEEvent, and it must change
// something (a deletion, a commit, or a preedit or preedit cursor other
// than the previous event's in the session). The session's start and end
// are queued as IMESessionEvents by SetTextInput and focus changes: losing
// focus ends it after the synthetic key releases and before the
// FocusEvent; gaining focus with a state starts it after the FocusEvent.
func (w *Window) Inject(e platform.Event) {
	switch ev := e.(type) {
	case platform.PointerEvent:
		if ev.Time == 0 {
			ev.Time = w.b.Now()
		}
		if w.captionPointer(ev) {
			return
		}
		e = ev
	case platform.ScrollEvent:
		if ev.Time == 0 {
			ev.Time = w.b.Now()
		}
		e = ev
	case platform.KeyEvent:
		w.injectKey(ev)
		return
	case platform.IMEEvent:
		w.injectIME(ev)
		return
	case platform.FocusEvent:
		w.injectFocus(ev)
		return
	case platform.CloseRequestedEvent:
	case nil:
		panic("headless: Inject called with a nil event")
	default:
		panic(fmt.Sprintf("headless: Inject does not accept %T; use Resize/SetScale for ConfigureEvent, SetState for WindowStateEvent, RequestFrame and Backend.Advance for FrameEvent, Backend.Wake for WakeEvent, Backend.SetSettings for SettingsEvent, Backend.InjectApp for NotificationEvent and TrayEvent, SetFocused and SetTextInput for IMESessionEvent", e))
	}
	w.b.mu.Lock()
	if w.WindowState.Closed() {
		w.b.mu.Unlock()
		return
	}
	w.b.loop.Enqueue(w, e)
	w.b.mu.Unlock()
	w.b.signal()
}

// injectKey validates and queues a KeyEvent; see Inject.
func (w *Window) injectKey(e platform.KeyEvent) {
	switch {
	case e.Action != platform.KeyPress && e.Action != platform.KeyRelease:
		panic(fmt.Sprintf("headless: Inject(KeyEvent) with invalid Action %d", e.Action))
	case e.Synthetic:
		panic("headless: Inject(KeyEvent) with Synthetic set; synthetic releases are generated by the backend when the window loses focus (SetFocused(false))")
	case e.Action == platform.KeyRelease && (e.Repeat || e.Text != ""):
		panic(fmt.Sprintf("headless: Inject(KeyEvent) of a %v release with Repeat %v and Text %q; releases never repeat and produce no text", e.Code, e.Repeat, e.Text))
	}
	b := w.b
	b.mu.Lock()
	if w.WindowState.Closed() {
		b.mu.Unlock()
		return
	}
	var problem string
	held := w.held.IsHeld(e.Code)
	switch {
	case !w.focused:
		problem = "the window does not have keyboard focus; call SetFocused(true) first (backends deliver key events only to the focused window)"
	case e.Action == platform.KeyPress && !e.Repeat && held:
		problem = "the key is already held; inject a release first, or set Repeat for key repeat"
	case e.Action == platform.KeyPress && e.Repeat && !held:
		problem = "a repeat of a key that is not held; inject its press first"
	case e.Action == platform.KeyRelease && !held:
		problem = "a release of a key that is not held; inject its press first"
	}
	if problem != "" {
		b.mu.Unlock()
		panic(fmt.Sprintf("headless: Inject(KeyEvent %v %v): %s", e.Action, e.Code, problem))
	}
	if e.Time == 0 {
		e.Time = b.now()
	}
	w.held.Accept(e)
	b.loop.Enqueue(w, e)
	b.mu.Unlock()
	b.signal()
}

// injectFocus records the focus state and queues a FocusEvent, preceded by
// the synthetic releases of the held keys when the focus is lost.
func (w *Window) injectFocus(e platform.FocusEvent) {
	b := w.b
	b.mu.Lock()
	if w.WindowState.Closed() {
		b.mu.Unlock()
		return
	}
	w.queueFocus(e.Focused)
	b.mu.Unlock()
	b.signal()
}

// queueFocus records the focus state and queues its FocusEvent (see
// injectFocus); b.mu must be held and the window open.
func (w *Window) queueFocus(focused bool) {
	b := w.b
	w.focused = focused
	if !focused {
		for _, r := range w.held.ReleaseAll(b.now()) {
			b.loop.Enqueue(w, r)
		}
		w.setSession(false)
	}
	b.loop.Enqueue(w, platform.FocusEvent{Focused: focused})
	if focused {
		w.setSession(w.ime.state != nil)
	}
}

// Resize simulates the user or window manager resizing the window to size
// (logical pixels), clamped to the size limits. A ConfigureEvent is queued
// if the size changed.
func (w *Window) Resize(size platform.Size) {
	platform.CheckSize("Resize size", size)
	w.reconfigure(func(m *platform.Metrics) { m.Size = w.ctl.resized(size) })
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
// FocusEvent if the focus state changes (after synthetic releases of the
// held keys when the focus is lost).
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
	if w.WindowState.Closed() {
		b.mu.Unlock()
		return
	}
	old := w.os
	change(&w.os)
	if w.os == old {
		b.mu.Unlock()
		return
	}
	b.loop.Enqueue(w, platform.ConfigureEvent{Metrics: w.os})
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

// Options returns the resolved options the window was created with (the
// current size limits are MinSize and MaxSize).
func (w *Window) Options() platform.WindowOptions { return w.opts }

// Closed reports whether Close was called.
func (w *Window) Closed() bool {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.WindowState.Closed()
}

// FrameRequested reports whether a frame request is waiting to be
// delivered.
func (w *Window) FrameRequested() bool {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.FramePending()
}
