package core

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// A quit request of the OS runs Options.OnQuit, which decides; no window
// is asked to close.
func TestQuitRequestRunsOnQuit(t *testing.T) {
	quits, closes := 0, 0
	x := newWinHarness(t, Options{
		Root:    div(Text("x")),
		OnClose: func(WindowHandle) { closes++ },
		OnQuit:  func() { quits++ },
	})
	x.b.RequestQuit()
	x.drain()
	if quits != 1 || closes != 0 || x.b.Quitting() {
		t.Errorf("after a quit request: OnQuit ran %d times, OnClose %d, quitting %v; want OnQuit once only", quits, closes, x.b.Quitting())
	}
}

// Without OnQuit every window gets a close request: an app whose main
// window has no OnClose quits.
func TestQuitRequestWithoutOnQuitQuits(t *testing.T) {
	childClosed := 0
	root := Component(func() Node {
		return div(win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "C"}, OnClose: func() { childClosed++ }, Root: div(Text("c"))}))
	})
	x := newWinHarness(t, Options{Root: root})
	main := x.windows()[0]
	x.b.RequestQuit()
	x.drain()
	if !x.b.Quitting() || !main.Closed() {
		t.Errorf("after a quit request without OnQuit: quitting %v, main window closed %v; want the app to quit", x.b.Quitting(), main.Closed())
	}
	if childClosed != 0 {
		t.Errorf("the child window's OnClose ran %d times after the main window closed the app", childClosed)
	}
}

// Without OnQuit, a close-to-tray app's windows each get their close
// request: the main window hides (its OnClose), the child window closes
// (its OnClose), and the app keeps running.
func TestQuitRequestWithoutOnQuitAsksEveryWindow(t *testing.T) {
	var hidden []WindowHandle
	root := Component(func() Node {
		open, setOpen := UseState(true)
		return div(If(open, win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "C"}, OnClose: func() { setOpen.Set(false) }, Root: div(Text("c"))})))
	})
	x := newWinHarness(t, Options{Root: root, OnClose: func(w WindowHandle) { hidden = append(hidden, w); w.Hide() }})
	main, c := x.windows()[0], x.child(0)
	x.b.RequestQuit()
	x.drain()
	if len(hidden) != 1 || main.Visible() || main.Closed() {
		t.Errorf("main window: OnClose ran %d times, visible %v, closed %v; want it hidden by its OnClose", len(hidden), main.Visible(), main.Closed())
	}
	if !c.Closed() || x.b.Quitting() {
		t.Errorf("child window closed %v, quitting %v; want the child closed by its OnClose and the app running", c.Closed(), x.b.Quitting())
	}
}

// A modal window over the main window takes the main window's close
// request, as with the close button: it comes forward, and its own close
// request runs its OnClose.
func TestQuitRequestWithModal(t *testing.T) {
	closes, modalCloses := 0, 0
	root := Component(func() Node {
		return div(win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "M"}, Modal: true, OnClose: func() { modalCloses++ }, Root: div(Text("m"))}))
	})
	x := newWinHarness(t, Options{Root: root, OnClose: func(WindowHandle) { closes++ }})
	m := x.child(0)
	before := m.Activations()
	x.b.RequestQuit()
	x.drain()
	if closes != 0 || modalCloses != 1 || m.Activations() != before+1 {
		t.Errorf("main OnClose %d, modal OnClose %d, modal activations +%d; want 0, 1, +1", closes, modalCloses, m.Activations()-before)
	}
}

// A TitleBarOverlay window has client-side decorations from the start and
// reports the OS's controls' rectangle in its state.
func TestTitleBarOverlayState(t *testing.T) {
	controls := platform.Rect{X: 12, Y: 10, W: 54, H: 16}
	b := headless.New(headless.Options{Size: platform.Size{W: 400, H: 300}, TitleBarControls: controls})
	h, err := NewHost(b, Options{Fonts: testFonts, Root: div(Text("x")), Window: WindowSpec{TitleBarOverlay: true}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.stopTimer)
	main := b.Windows()[0]
	if !main.Options().TitleBarOverlay || main.Options().Frameless {
		t.Errorf("platform options %+v, want TitleBarOverlay", main.Options())
	}
	handle := h.Handle(main)
	if st := handle.State(); !st.ClientSideDecorations {
		t.Errorf("state before any event %+v, want client-side decorations", st)
	}
	b.Drain(h)
	want := Rect{X: 12, Y: 10, Width: 54, Height: 16}
	if st := handle.State(); !st.ClientSideDecorations || st.TitleBarControls != want {
		t.Errorf("state %+v, want client-side decorations and controls %v", st, want)
	}
	main.SetTitleBarControls(platform.Rect{X: 20, Y: 18, W: 54, H: 16})
	b.Drain(h)
	if st := handle.State(); st.TitleBarControls != (Rect{X: 20, Y: 18, Width: 54, Height: 16}) {
		t.Errorf("state after the controls moved %+v", st)
	}
}

func TestFramelessTitleBarOverlayPanics(t *testing.T) {
	b := headless.New(headless.Options{})
	defer func() {
		r := recover()
		if s, _ := r.(string); !strings.Contains(s, "both Frameless and TitleBarOverlay; set one") {
			t.Errorf("panic %v, want one that says to set one of Frameless and TitleBarOverlay", r)
		}
	}()
	_, _ = NewHost(b, Options{Fonts: testFonts, Root: div(Text("x")), Window: WindowSpec{Frameless: true, TitleBarOverlay: true}})
}
