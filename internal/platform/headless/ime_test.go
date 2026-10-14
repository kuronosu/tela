package headless

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// imeLog records IME, key and focus events as compact strings.
type imeLog struct{ events []string }

func (l *imeLog) HandleEvent(_ platform.Window, e platform.Event) {
	switch e := e.(type) {
	case platform.IMEEvent:
		l.events = append(l.events, fmt.Sprintf("ime del=%d,%d commit=%q preedit=%q cursor=%d,%d",
			e.DeleteBefore, e.DeleteAfter, e.Commit, e.Preedit, e.CursorStart, e.CursorEnd))
	case platform.IMESessionEvent:
		l.events = append(l.events, fmt.Sprintf("session %v", e.Active))
	case platform.FocusEvent:
		l.events = append(l.events, fmt.Sprintf("focus %v", e.Focused))
	case platform.KeyEvent:
		l.events = append(l.events, fmt.Sprintf("key %v %v", e.Action, e.Code))
	}
}

func (l *imeLog) take() string {
	s := strings.Join(l.events, "; ")
	l.events = nil
	return s
}

func TestTextInputSession(t *testing.T) {
	b := New(Options{})
	w := newWindow(t, b, platform.WindowOptions{})
	b.Drain(&recorder{})
	log := &imeLog{}

	// A state without focus: recorded, no session.
	st := &platform.TextInputState{CursorArea: platform.Rect{X: 4, Y: 8, H: 20}, Surrounding: &platform.SurroundingText{Text: "ab", Cursor: 2, Anchor: 2}}
	w.SetTextInput(st)
	st.CursorArea.X = 99 // the backend keeps a copy
	if got := w.TextInput(); got == nil || got.CursorArea.X != 4 || got.Surrounding.Text != "ab" {
		t.Errorf("TextInput() = %+v", got)
	}
	b.Drain(log)
	if got := log.take(); got != "" {
		t.Errorf("events without focus: %s", got)
	}

	// Focus starts the session after the FocusEvent.
	w.SetFocused(true)
	b.Drain(log)
	if got, want := log.take(), "focus true; session true"; got != want {
		t.Errorf("focus in: %s, want %s", got, want)
	}
	if !w.IMEActive() {
		t.Errorf("IMEActive() = false")
	}

	// Equal states are free; changes are counted.
	changes := w.TextInputChanges()
	w.SetTextInput(w.TextInput())
	if w.TextInputChanges() != changes {
		t.Errorf("an equal state counted as a change")
	}
	w.SetTextInput(&platform.TextInputState{Purpose: platform.TextPurposeURL})
	if w.TextInputChanges() != changes+1 {
		t.Errorf("TextInputChanges() = %d, want %d", w.TextInputChanges(), changes+1)
	}

	// Moving to another field restarts the session.
	w.SetTextInput(nil)
	w.SetTextInput(&platform.TextInputState{Multiline: true})
	b.Drain(log)
	if got, want := log.take(), "session false; session true"; got != want {
		t.Errorf("field change: %s, want %s", got, want)
	}

	// A composition: preedit updates, commit, delete surrounding.
	inject := func(e platform.IMEEvent) { w.Inject(e) }
	inject(platform.IMEEvent{Preedit: "k", CursorStart: 1, CursorEnd: 1})
	inject(platform.IMEEvent{Preedit: "か", CursorStart: 3, CursorEnd: 3})
	inject(platform.IMEEvent{Preedit: "漢字", CursorStart: 0, CursorEnd: 6})
	inject(platform.IMEEvent{Commit: "漢字"})
	inject(platform.IMEEvent{DeleteBefore: 1, Commit: "é", Preedit: "x", CursorStart: -1, CursorEnd: -1})
	b.Drain(log)
	want := strings.Join([]string{
		`ime del=0,0 commit="" preedit="k" cursor=1,1`,
		`ime del=0,0 commit="" preedit="か" cursor=3,3`,
		`ime del=0,0 commit="" preedit="漢字" cursor=0,6`,
		`ime del=0,0 commit="漢字" preedit="" cursor=0,0`,
		`ime del=1,0 commit="é" preedit="x" cursor=-1,-1`,
	}, "; ")
	if got := log.take(); got != want {
		t.Errorf("composition:\n got %s\nwant %s", got, want)
	}

	// A reset forgets the preedit: the same preedit may come again.
	w.ResetTextInput()
	if w.TextInputResets() != 1 {
		t.Errorf("TextInputResets() = %d", w.TextInputResets())
	}
	inject(platform.IMEEvent{Preedit: "x", CursorStart: -1, CursorEnd: -1})

	// Losing focus ends the session after the synthetic releases.
	w.Inject(key(platform.KeyPress, platform.CodeKeyA, "a", "a"))
	w.SetFocused(false)
	b.Drain(log)
	if got, want := log.take(), `ime del=0,0 commit="" preedit="x" cursor=-1,-1; key press KeyA; key release KeyA; session false; focus false`; got != want {
		t.Errorf("focus out:\n got %s\nwant %s", got, want)
	}

	// Nil state: no session at focus.
	w.SetTextInput(nil)
	w.SetFocused(true)
	w.ResetTextInput() // no state: not counted
	b.Drain(log)
	if got := log.take(); got != "focus true" {
		t.Errorf("focus without a state: %s", got)
	}
	if w.TextInputResets() != 1 || w.TextInput() != nil {
		t.Errorf("TextInputResets() = %d, TextInput() = %+v", w.TextInputResets(), w.TextInput())
	}
}

func TestInjectIMEContract(t *testing.T) {
	b := New(Options{})
	w := newWindow(t, b, platform.WindowOptions{})
	b.Drain(&recorder{})

	expectPanic(t, "no active IME session", func() { w.Inject(platform.IMEEvent{Commit: "a"}) })
	w.SetTextInput(&platform.TextInputState{})
	expectPanic(t, "no active IME session", func() { w.Inject(platform.IMEEvent{Commit: "a"}) })
	w.SetFocused(true)
	w.Inject(platform.IMEEvent{Commit: "a"})
	expectPanic(t, "changes nothing", func() { w.Inject(platform.IMEEvent{}) })
	w.Inject(platform.IMEEvent{Preedit: "ab", CursorStart: 1, CursorEnd: 2})
	expectPanic(t, "changes nothing", func() { w.Inject(platform.IMEEvent{Preedit: "ab", CursorStart: 1, CursorEnd: 2}) })
	w.Inject(platform.IMEEvent{Preedit: "ab", CursorStart: 2, CursorEnd: 2})
	w.Inject(platform.IMEEvent{}) // ends the composition
	expectPanic(t, "negative", func() { w.Inject(platform.IMEEvent{DeleteAfter: -2}) })
	expectPanic(t, "ordered", func() { w.Inject(platform.IMEEvent{Preedit: "ab", CursorStart: 2, CursorEnd: 1}) })
	expectPanic(t, "hidden cursor", func() { w.Inject(platform.IMEEvent{Preedit: "ab", CursorStart: -1}) })
	expectPanic(t, "UTF-8", func() { w.Inject(platform.IMEEvent{Commit: "\xc3"}) })
	expectPanic(t, "SetTextInput for IMESessionEvent", func() { w.Inject(platform.IMESessionEvent{Active: true}) })
	expectPanic(t, "TextInputState.Purpose", func() { w.SetTextInput(&platform.TextInputState{Purpose: 50}) })

	// Closed windows drop everything.
	w.Close()
	w.SetTextInput(&platform.TextInputState{Multiline: true})
	w.Inject(platform.IMEEvent{Commit: "x"})
	w.ResetTextInput()
}
