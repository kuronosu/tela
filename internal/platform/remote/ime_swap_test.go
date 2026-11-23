package remote

import (
	"slices"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// An IME composition in progress when tela dev swaps children (ADR-309):
// the platform's IME holds the preedit in the real window's session, so the
// host keeps that session when the new build's window shows the same field
// (the same purpose, multiline flag and surrounding text: its state was
// restored) and the new child joins it; for any other field the session
// restarts, and the new child is told that the session it took over ended,
// which finishes a composition it restored.

// imeSwap is a real window with child A's focused field composing, and
// child B pending with its first frame shown offscreen.
type imeSwap struct {
	p     *pair
	a, b  *app
	cb    *Child
	hw    *headless.Window
	wb    platform.Window
	field platform.TextInputState
}

func newIMESwap(t *testing.T) *imeSwap {
	p := newPair(t, HostOptions{}, headless.Options{})
	a := p.dial("gen-1").start()
	wa := a.newWindow(platform.WindowOptions{Size: platform.Size{W: 80, H: 60}})
	a.configured()
	hw := p.window(0)
	waitFrame(t, hw, "A's frame", a.presentNow(wa, 1, nil))
	hw.SetFocused(true)
	a.waitEvent("A's focus", func(e platform.Event) bool { return e == platform.FocusEvent{Focused: true} })
	field := platform.TextInputState{CursorArea: platform.Rect{X: 10, Y: 2, W: 20, H: 10},
		Surrounding: &platform.SurroundingText{Text: "ab", Cursor: 2, Anchor: 2}}
	a.ui(func() { wa.SetTextInput(field.Clone()) })
	a.waitEvent("A's IME session", func(e platform.Event) bool { return e == platform.IMESessionEvent{Active: true} })
	pre := platform.IMEEvent{Preedit: "にほ", CursorStart: 6, CursorEnd: 6}
	hw.Inject(pre)
	a.waitEvent("A's preedit", func(e platform.Event) bool { ev, ok := e.(platform.IMEEvent); return ok && ev.Preedit == pre.Preedit })

	b := p.dial("gen-2").start()
	wb := b.newWindow(platform.WindowOptions{Size: platform.Size{W: 80, H: 60}})
	cb := p.child(1)
	b.configured()
	b.waitEvent("B's focus", func(e platform.Event) bool { return e == platform.FocusEvent{Focused: true} })
	b.waitEvent("B's frame", isType[platform.FrameEvent])
	b.presentNow(wb, 2, nil)
	<-cb.FirstFrame()
	return &imeSwap{p: p, a: a, b: b, cb: cb, hw: hw, wb: wb, field: field}
}

// promote gives B the text input state st (as its first frame did), then
// freezes and promotes B as tela dev does, and returns how many times the
// real window's text input state changed.
func (s *imeSwap) promote(t *testing.T, st *platform.TextInputState) int {
	t.Helper()
	if st != nil {
		s.b.ui(func() { s.wb.SetTextInput(st.Clone()) })
	}
	s.p.roundTrip(s.b, s.cb)
	before := s.hw.TextInputChanges()
	s.p.ui(s.p.host.Freeze)
	s.b.take()
	var err error
	s.p.ui(func() { err = s.p.host.Promote(s.cb) })
	if err != nil {
		t.Fatalf("Promote: %v", err)
	}
	s.p.roundTrip(s.b, s.cb)
	return s.hw.TextInputChanges() - before
}

// sessions returns the IME session events B received since the promotion,
// in order.
func (s *imeSwap) sessions() []bool {
	s.b.mu.Lock()
	defer s.b.mu.Unlock()
	var out []bool
	for _, e := range s.b.events {
		if ev, ok := e.e.(platform.IMESessionEvent); ok {
			out = append(out, ev.Active)
		}
	}
	return out
}

// expectSessions waits until B's IME session events are want, and checks
// that nothing follows once the host and B are settled.
func (s *imeSwap) expectSessions(t *testing.T, want ...bool) {
	t.Helper()
	deadline := time.Now().Add(waitTimeout)
	for !slices.Equal(s.sessions(), want) && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	time.Sleep(20 * time.Millisecond) // a late session event of the real window
	s.p.roundTrip(s.b, s.cb)
	if got := s.sessions(); !slices.Equal(got, want) {
		t.Errorf("B's IME sessions %v, want %v", got, want)
	}
}

func TestSwapKeepsIMESession(t *testing.T) {
	s := newIMESwap(t)
	// B shows the same field, laid out a little to the right.
	moved := s.field
	moved.CursorArea.X = 14
	if n := s.promote(t, &moved); n != 1 {
		t.Errorf("the real window's text input changed %d times at the swap, want 1 (the caret area): the IME session restarted", n)
	}
	if got := s.hw.TextInput(); !got.Equal(&moved) || !s.hw.IMEActive() {
		t.Errorf("real window: text input %+v, session active %v; want B's state in the same session", got, s.hw.IMEActive())
	}
	s.expectSessions(t, true) // it joins the session
	// The IME goes on with its preedit: its next update is B's.
	commit := platform.IMEEvent{Commit: "日本"}
	s.hw.Inject(commit)
	if got := s.b.waitEvent("B's commit", isType[platform.IMEEvent]); got != commit {
		t.Errorf("B got %+v, want %+v", got, commit)
	}
	for _, e := range s.a.take() {
		if isType[platform.IMEEvent](e.e) {
			t.Errorf("the detached child got %+v", e.e)
		}
	}
}

// A new build promoted before its restored field was laid out (its state
// has no caret area yet, under load) keeps the session's caret area: the
// real window's state does not change, and nor does it when the new build
// reports the area of the same layout.
func TestSwapKeepsIMESessionBeforeLayout(t *testing.T) {
	s := newIMESwap(t)
	unknown := s.field
	unknown.CursorArea = platform.Rect{}
	if n := s.promote(t, &unknown); n != 0 {
		t.Errorf("the real window's text input changed %d times at the swap, want 0: B's field is the same, its caret area not known yet", n)
	}
	if got := s.hw.TextInput(); !got.Equal(&s.field) || !s.hw.IMEActive() {
		t.Errorf("real window: text input %+v, session active %v; want A's caret area in the same session", got, s.hw.IMEActive())
	}
	s.expectSessions(t, true)
	before := s.hw.TextInputChanges()
	s.b.ui(func() { s.wb.SetTextInput(s.field.Clone()) })
	s.p.roundTrip(s.b, s.cb)
	if n := s.hw.TextInputChanges() - before; n != 0 {
		t.Errorf("B's laid-out state, the same as A's, changed the real window's text input %d times", n)
	}
}

func TestSwapRestartsIMESessionForAnotherField(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*platform.TextInputState)
	}{
		{"another purpose", func(st *platform.TextInputState) { st.Purpose = platform.TextPurposeEmail }},
		{"another text", func(st *platform.TextInputState) {
			st.Surrounding = &platform.SurroundingText{Text: "abc", Cursor: 3, Anchor: 3}
		}},
		{"multiline", func(st *platform.TextInputState) { st.Multiline = true }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := newIMESwap(t)
			other := s.field
			other.Surrounding = new(*s.field.Surrounding)
			tc.edit(&other)
			if n := s.promote(t, &other); n != 2 {
				t.Errorf("the real window's text input changed %d times, want 2: a new session (nil, then B's state)", n)
			}
			// B was told the session it took over ended, which finishes a
			// composition it restored, then the new session started.
			waitFor(t, "the new session", func() bool { return s.hw.IMEActive() })
			s.expectSessions(t, true, false, true)
		})
	}
}

// A new build with no field (its window has no text input state) ends the
// session, for the real window and for the new child.
func TestSwapEndsIMESessionWithoutAField(t *testing.T) {
	s := newIMESwap(t)
	s.promote(t, nil)
	if s.hw.TextInput() != nil {
		t.Errorf("the real window keeps the text input %+v of a field the new build does not have", s.hw.TextInput())
	}
	s.expectSessions(t, true, false) // the session it took over ended
}
