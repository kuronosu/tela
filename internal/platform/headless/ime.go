package headless

import (
	"fmt"

	"github.com/kuronosu/tela/internal/platform"
)

// imeState is a window's text input and IME session state, guarded by
// b.mu.
type imeState struct {
	state   *platform.TextInputState // latest SetTextInput (a clone); nil: none
	changes int                      // SetTextInput calls that changed state
	resets  int                      // ResetTextInput calls with a state
	active  bool                     // the IME session reported to the app
	// preedit and cursor of the latest injected IMEEvent of the session,
	// to reject events that change nothing.
	preedit   string
	preCursor [2]int
}

// SetTextInput records the text input state; see platform.Window. The
// session is active while the window has focus and a state; its changes
// are queued as IMESessionEvents.
func (w *Window) SetTextInput(state *platform.TextInputState) {
	platform.CheckTextInputState(state)
	b := w.b
	b.mu.Lock()
	defer b.mu.Unlock()
	if w.WindowState.Closed() {
		return
	}
	if w.ime.state.Equal(state) {
		return
	}
	w.ime.state = state.Clone()
	w.ime.changes++
	w.setSession(w.focused && w.ime.state != nil)
}

// ResetTextInput records a reset and forgets the injected preedit; see
// platform.Window.
func (w *Window) ResetTextInput() {
	b := w.b
	b.mu.Lock()
	defer b.mu.Unlock()
	if w.WindowState.Closed() || w.ime.state == nil {
		return
	}
	w.ime.resets++
	w.ime.preedit, w.ime.preCursor = "", [2]int{}
}

// setSession queues an IMESessionEvent if the session state changes; b.mu
// must be held.
func (w *Window) setSession(active bool) {
	if w.ime.active == active {
		return
	}
	w.ime.active = active
	w.ime.preedit, w.ime.preCursor = "", [2]int{}
	w.b.loop.Enqueue(w, platform.IMESessionEvent{Active: active})
	w.b.signal()
}

// injectIME validates and queues an IMEEvent; see Inject.
func (w *Window) injectIME(e platform.IMEEvent) {
	b := w.b
	b.mu.Lock()
	if w.WindowState.Closed() {
		b.mu.Unlock()
		return
	}
	problem := platform.CheckIMEEvent(e)
	switch {
	case problem != "":
	case !w.ime.active:
		problem = "the window has no active IME session; give it focus (SetFocused(true)) and a text input state (SetTextInput) first"
	case e.DeleteBefore == 0 && e.DeleteAfter == 0 && e.Commit == "" &&
		e.Preedit == w.ime.preedit && [2]int{e.CursorStart, e.CursorEnd} == w.ime.preCursor:
		problem = fmt.Sprintf("the event changes nothing (the preedit is already %q); backends deliver only changes", e.Preedit)
	}
	if problem != "" {
		b.mu.Unlock()
		panic(fmt.Sprintf("headless: Inject(%+v): %s", e, problem))
	}
	w.ime.preedit, w.ime.preCursor = e.Preedit, [2]int{e.CursorStart, e.CursorEnd}
	b.loop.Enqueue(w, e)
	b.mu.Unlock()
	b.signal()
}

// TextInput returns a copy of the latest text input state set by the app
// (nil if none, or if it was cleared).
func (w *Window) TextInput() *platform.TextInputState {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.ime.state.Clone()
}

// TextInputChanges returns how many SetTextInput calls changed the state:
// what a real backend would have sent to the OS at most, which tests use
// to check that the app does not resend equal states.
func (w *Window) TextInputChanges() int {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.ime.changes
}

// TextInputResets returns how many ResetTextInput calls were made while a
// text input state was set.
func (w *Window) TextInputResets() int {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.ime.resets
}

// IMEActive reports whether the window's IME session is active (the state
// of the latest queued IMESessionEvent).
func (w *Window) IMEActive() bool {
	w.b.mu.Lock()
	defer w.b.mu.Unlock()
	return w.ime.active
}
