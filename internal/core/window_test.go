package core

import (
	"image"
	"strconv"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// win is a Window component (tela.Window's build) for these tests.
func win(spec ChildWindowSpec) Node {
	return Component(func() Node {
		UseWindowSpec(spec)
		return Node{}
	})
}

// winHarness is an app on the headless backend whose windows the tests
// drive (ADR-180).
type winHarness struct {
	t *testing.T
	b *headless.Backend
	h *Host
}

// newWinHarness mounts opts.Root in a 400x300 headless window and delivers
// what is due (the windows' configures and frames).
func newWinHarness(t *testing.T, opts Options) *winHarness {
	t.Helper()
	b := headless.New(headless.Options{Size: platform.Size{W: 400, H: 300}})
	if opts.Fonts == nil {
		opts.Fonts = testFonts
	}
	h, err := NewHost(b, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.stopTimer)
	x := &winHarness{t: t, b: b, h: h}
	x.drain()
	return x
}

// drain delivers what is due, then a few refreshes of the clock, so the
// frames requested meanwhile (paced one refresh after the previous one)
// run too, and so do the host's timer deadlines (a paced commit of a
// hidden window, paused.go), which wake the loop as the timer would.
func (x *winHarness) drain() {
	x.b.Drain(x.h)
	for range 4 {
		x.b.Advance(x.b.FrameInterval())
		if at, ok := x.h.TimerArmed(); ok && at <= x.b.Now() {
			x.b.Wake()
		}
		x.b.Drain(x.h)
	}
}

// windows returns the open headless windows, the main one first.
func (x *winHarness) windows() []*headless.Window { return x.b.Windows() }

// child returns the i-th open window after the main one.
func (x *winHarness) child(i int) *headless.Window {
	x.t.Helper()
	ws := x.windows()
	if len(ws) <= i+1 {
		x.t.Fatalf("%d windows open, want at least %d", len(ws), i+2)
	}
	return ws[i+1]
}

// snapshot returns the aria snapshot of the tree shown in w.
func (x *winHarness) snapshot(w *headless.Window) string {
	x.t.Helper()
	tr := x.h.TreeOf(w)
	if tr == nil {
		x.t.Fatalf("no tree for window %q", w.Title())
	}
	return tr.AriaSnapshot()
}

func (x *winHarness) click(w *headless.Window, px, py float32) {
	w.Inject(ptr(platform.PointerMove, px, py, platform.ButtonNone, 0))
	w.Inject(down(px, py))
	w.Inject(up(px, py))
	x.drain()
}

// A Window opens a child window of the window it is declared in while it
// is mounted, shows its Root there, and closes it (running the cleanups of
// its tree's effects) when it unmounts.
func TestWindowOpensWhileMounted(t *testing.T) {
	var setOpen Setter[bool]
	mounts, cleanups := 0, 0
	content := Component(func() Node {
		UseMount(func() func() {
			mounts++
			return func() { cleanups++ }
		})
		return div(Text("child content"))
	})
	root := Component(func() Node {
		open, set := UseState(true)
		setOpen = set
		return div(Text("main content"), If(open, win(ChildWindowSpec{
			WindowSpec: WindowSpec{Title: "Child", Width: 200, Height: 100},
			OnClose:    func() {}, Root: content,
		})))
	})
	x := newWinHarness(t, Options{Window: WindowSpec{Title: "Main"}, Root: root})
	ws := x.windows()
	if len(ws) != 2 || len(x.h.Windows()) != 2 {
		t.Fatalf("%d headless windows, %d host windows; want 2 and 2", len(ws), len(x.h.Windows()))
	}
	c := ws[1]
	if c.Title() != "Child" || c.Parent() != ws[0] || c.Options().Size != (platform.Size{W: 200, H: 100}) || c.Options().Modal {
		t.Errorf("child window: title %q, parent %v, options %+v", c.Title(), c.Parent(), c.Options())
	}
	if s := x.snapshot(c); !strings.Contains(s, "child content") || strings.Contains(s, "main content") {
		t.Errorf("child window shows %q", s)
	}
	if s := x.snapshot(ws[0]); strings.Contains(s, "child content") {
		t.Errorf("main window shows the child's content: %q", s)
	}
	if c.Presents() == 0 || mounts != 1 {
		t.Errorf("child window: %d presents, %d effect runs; want a frame and 1 run", c.Presents(), mounts)
	}

	setOpen.Set(false)
	x.drain()
	if !c.Closed() || len(x.windows()) != 1 || len(x.h.Windows()) != 1 {
		t.Errorf("after unmounting the Window: closed %v, %d windows", c.Closed(), len(x.windows()))
	}
	if cleanups != 1 {
		t.Errorf("%d cleanups after the window closed, want 1", cleanups)
	}
	if x.h.TreeOf(c) != nil {
		t.Error("the host still knows the closed window")
	}

	setOpen.Set(true)
	x.drain()
	if len(x.windows()) != 2 || mounts != 2 {
		t.Errorf("mounted again: %d windows, %d effect runs; want 2 and 2", len(x.windows()), mounts)
	}
}

// One element tree: contexts above the Window reach its content, and a
// Value of the main window's tree read in the child window builds it
// again when it changes.
func TestWindowContextAndValues(t *testing.T) {
	ctx := CreateContext("default")
	var setTheme Setter[string]
	var val *Value[int]
	content := Component(func() Node {
		return div(Textf("theme=%s n=%d", UseContext(ctx), val.Get()))
	})
	root := Component(func() Node {
		th, set := UseState("light")
		setTheme = set
		val = UseValue(1)
		return ctx.Provider(th, div(win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "C"}, OnClose: func() {}, Root: content})))
	})
	x := newWinHarness(t, Options{Root: root})
	c := x.child(0)
	if s := x.snapshot(c); !strings.Contains(s, "theme=light n=1") {
		t.Fatalf("child window shows %q, want the provider's value and the Value", s)
	}
	setTheme.Set("dark")
	x.drain()
	if s := x.snapshot(c); !strings.Contains(s, "theme=dark n=1") {
		t.Errorf("after the provider changed, the child window shows %q", s)
	}
	val.Set(5)
	x.drain()
	if s := x.snapshot(c); !strings.Contains(s, "theme=dark n=5") {
		t.Errorf("after the Value changed, the child window shows %q", s)
	}
}

// The fields of an open Window update its OS window when they change.
func TestWindowUpdatesOptions(t *testing.T) {
	var setStep Setter[int]
	icon := []image.Image{image.NewRGBA(image.Rect(0, 0, 16, 16))}
	root := Component(func() Node {
		step, set := UseState(0)
		setStep = set
		spec := ChildWindowSpec{WindowSpec: WindowSpec{Title: "A", Width: 300, Height: 200}, OnClose: func() {}, Root: div(Text("x"))}
		if step == 1 {
			spec.Title = "B"
			spec.Width, spec.Height = 320, 240
			spec.MinWidth, spec.MinHeight = 100, 80
			spec.AlwaysOnTop = true
			spec.Hidden = true
			spec.Icon = icon
		}
		return div(win(spec))
	})
	x := newWinHarness(t, Options{Root: root})
	c := x.child(0)
	h := x.h.Handle(c)
	setStep.Set(1)
	x.drain()
	min, _ := c.SizeLimits()
	if c.Title() != "B" || c.RestoreSize() != (platform.Size{W: 320, H: 240}) || min != (platform.Size{W: 100, H: 80}) ||
		!c.AlwaysOnTop() || c.Visible() || len(c.Icon()) != 1 {
		t.Errorf("updated window: title %q, size %v, min %v, on top %v, visible %v, %d icons",
			c.Title(), c.RestoreSize(), min, c.AlwaysOnTop(), c.Visible(), len(c.Icon()))
	}
	if !h.State().Hidden {
		t.Error("State().Hidden is false for a hidden window")
	}
	setStep.Set(0)
	x.drain()
	if c.Title() != "A" || !c.Visible() || c.Closed() || len(c.Icon()) != 0 {
		t.Errorf("back: title %q, visible %v, closed %v, %d icons", c.Title(), c.Visible(), c.Closed(), len(c.Icon()))
	}
}

// A visible modal window blocks the input of the window it is declared
// in: core drops it whatever the backend does (headless delivers it), a
// press activates the modal window, and a close request is refused.
func TestModalWindowBlocksParent(t *testing.T) {
	clicks := 0
	var setModal Setter[bool]
	root := Component(func() Node {
		modal, set := UseState(true)
		setModal = set
		return div(
			button(Class("block w-40 h-10"), On("click", func(MouseEvent) { clicks++ }), Text("press")),
			If(modal, win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "M"}, Modal: true, OnClose: func() {}, Root: div(Text("modal"))})),
		)
	})
	x := newWinHarness(t, Options{Root: root})
	main, m := x.windows()[0], x.child(0)
	if !m.Options().Modal || m.Parent() != main || !main.Blocked() {
		t.Fatalf("modal window: options %+v, blocked %v", m.Options(), main.Blocked())
	}
	x.click(main, 20, 20)
	if clicks != 0 {
		t.Errorf("a click in the blocked window ran its handler (%d clicks)", clicks)
	}
	if m.Activations() == 0 {
		t.Error("a press in the blocked window did not activate the modal window")
	}
	main.Inject(platform.ScrollEvent{Pos: platform.Point{X: 20, Y: 20}, Steps: platform.Point{Y: 1}})
	main.RequestClose()
	x.drain()
	if x.b.Quitting() || main.Closed() {
		t.Error("a close request of the blocked main window quit the app")
	}

	setModal.Set(false)
	x.drain()
	if m.Closed() == false || main.Blocked() {
		t.Fatalf("modal closed %v, main blocked %v", m.Closed(), main.Blocked())
	}
	x.click(main, 20, 20)
	if clicks != 1 {
		t.Errorf("%d clicks after the modal window closed, want 1", clicks)
	}
}

// A handler of a child window that drives its parent synchronously (an
// ElementRef.Click on the parent's element), whose build changes the
// Window, does not rebuild the child window's tree under the child's own
// dispatch: the click finishes with the handlers it started with, and the
// window and its content are updated after it (as closing waits,
// closeWindow).
func TestWindowUpdateWaitsForChildDispatch(t *testing.T) {
	var parentBtn ElementRef
	var calls []string
	root := Component(func() Node {
		n, setN := UseState(0)
		parentBtn = UseElementRef()
		child := []Child{Class("block w-40 h-10"), On("click", func(MouseEvent) {
			calls = append(calls, "first")
			parentBtn.Click()
		})}
		if n == 0 {
			child = append(child, On("click", func(MouseEvent) { calls = append(calls, "second") }))
		}
		child = append(child, Text("child "+strconv.Itoa(n)))
		return div(
			button(AttachRef(parentBtn), Class("block w-40 h-10"), On("click", func(MouseEvent) {
				calls = append(calls, "parent")
				setN.Set(n + 1)
			}), Text("parent")),
			win(ChildWindowSpec{
				WindowSpec: WindowSpec{Title: "Child " + strconv.Itoa(n), Width: 200, Height: 100},
				OnClose:    func() {}, Root: button(child...),
			}),
		)
	})
	x := newWinHarness(t, Options{Root: root})
	c := x.child(0)
	x.click(c, 20, 20)
	if got := strings.Join(calls, " "); got != "first parent second" {
		t.Errorf("handlers ran %q, want %q", got, "first parent second")
	}
	if c.Title() != "Child 1" || !strings.Contains(x.snapshot(c), "child 1") {
		t.Errorf("child window after the click: title %q, content %q", c.Title(), x.snapshot(c))
	}
	calls = nil
	x.click(c, 20, 20)
	if got := strings.Join(calls, " "); got != "first parent" {
		t.Errorf("second click ran %q, want %q", got, "first parent")
	}
	if c.Title() != "Child 2" || !strings.Contains(x.snapshot(c), "child 2") {
		t.Errorf("child window after the second click: title %q, content %q", c.Title(), x.snapshot(c))
	}
}

// The same when the child window's frame drives the parent (a layout
// effect of the child clicks the parent's element): the child's tree is
// not rebuilt in the middle of its own frame.
func TestWindowUpdateWaitsForChildFrame(t *testing.T) {
	var parentBtn ElementRef
	builds, during := map[int]int{}, -1
	content := func(n int) Node {
		builds[n]++
		UseLayoutEffect(func() func() {
			before := builds[1]
			parentBtn.Click()
			during = builds[1] - before
			return nil
		}, Once)
		return div(Text("child " + strconv.Itoa(n)))
	}
	root := Component(func() Node {
		n, setN := UseState(0)
		parentBtn = UseElementRef()
		return div(
			button(AttachRef(parentBtn), Class("block w-40 h-10"), On("click", func(MouseEvent) { setN.Set(n + 1) }), Text("parent")),
			win(ChildWindowSpec{
				WindowSpec: WindowSpec{Title: "Child " + strconv.Itoa(n), Width: 200, Height: 100},
				OnClose:    func() {}, Root: ComponentProps(content, n),
			}),
		)
	})
	x := newWinHarness(t, Options{Root: root})
	c := x.child(0)
	if during != 0 {
		t.Errorf("the child window's content built %d times inside its own frame, want 0", during)
	}
	if c.Title() != "Child 1" || !strings.Contains(x.snapshot(c), "child 1") || builds[1] != 1 {
		t.Errorf("child window after its frame: title %q, content %q, builds %v", c.Title(), x.snapshot(c), builds)
	}
}

// A hidden modal window blocks nothing.
func TestHiddenModalWindowDoesNotBlock(t *testing.T) {
	clicks := 0
	root := div(
		button(Class("block w-40 h-10"), On("click", func(MouseEvent) { clicks++ }), Text("press")),
		win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "M", Hidden: true}, Modal: true, OnClose: func() {}, Root: div(Text("modal"))}),
	)
	x := newWinHarness(t, Options{Root: root})
	x.click(x.windows()[0], 20, 20)
	if clicks != 1 {
		t.Errorf("%d clicks with a hidden modal window, want 1", clicks)
	}
}

// Each window has its own focus state, and UseWindow's handle follows it.
func TestWindowFocusPerWindow(t *testing.T) {
	var mainH, childH WindowHandle
	content := Component(func() Node {
		childH = UseWindow()
		return div(Text("child"))
	})
	root := Component(func() Node {
		mainH = UseWindow()
		return div(win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "C"}, OnClose: func() {}, Root: content}))
	})
	x := newWinHarness(t, Options{Root: root})
	main, c := x.windows()[0], x.child(0)
	if mainH == childH || mainH != x.h.Handle(main) || childH != x.h.Handle(c) {
		t.Fatal("UseWindow does not return the handle of the component's window")
	}
	main.SetFocused(true)
	x.drain()
	if !mainH.State().Focused || childH.State().Focused || !x.h.Tree().foc.window || x.h.TreeOf(c).foc.window {
		t.Errorf("main focused: main %+v, child %+v", mainH.State(), childH.State())
	}
	childH.Focus() // Activate: the headless window manager moves the focus
	x.drain()
	if mainH.State().Focused || !childH.State().Focused || x.h.Tree().foc.window || !x.h.TreeOf(c).foc.window {
		t.Errorf("child focused: main %+v, child %+v", mainH.State(), childH.State())
	}
}

// UseWindow's State is reactive, its requests reach the OS window, and
// Placement keeps the size to return to.
func TestUseWindowState(t *testing.T) {
	var w WindowHandle
	builds := 0
	root := Component(func() Node {
		builds++
		w = UseWindow()
		st := w.State()
		return div(Textf("max=%v full=%v", st.Maximized, st.Fullscreen))
	})
	x := newWinHarness(t, Options{Root: root})
	main := x.windows()[0]
	if s := x.snapshot(main); !strings.Contains(s, "max=false") {
		t.Fatalf("snapshot %q", s)
	}
	w.Maximize()
	x.drain()
	if !main.OSState().Maximized || !w.State().Maximized {
		t.Fatalf("Maximize: OS state %v, State %+v", main.OSState(), w.State())
	}
	if s := x.snapshot(main); !strings.Contains(s, "max=true") {
		t.Errorf("after Maximize the component shows %q", s)
	}
	if p := w.Placement(); !p.Maximized || p.Width != 400 || p.Height != 300 {
		t.Errorf("placement of the maximized window %+v, want 400x300 maximized", p)
	}
	w.ToggleMaximize()
	x.drain()
	if main.OSState().Maximized || main.Metrics().Size != (platform.Size{W: 400, H: 300}) {
		t.Errorf("ToggleMaximize: OS state %v, size %v", main.OSState(), main.Metrics().Size)
	}
	w.SetFullscreen(true)
	x.drain()
	if s := x.snapshot(main); !strings.Contains(s, "full=true") {
		t.Errorf("after SetFullscreen the component shows %q", s)
	}
	w.SetFullscreen(false)
	w.Minimize()
	x.drain()
	if !main.OSState().Minimized || !w.State().Minimized {
		t.Errorf("Minimize: OS state %v", main.OSState())
	}
	w.Focus()
	x.drain()
	if main.OSState().Minimized {
		t.Error("Focus did not bring the window back from minimized")
	}
	w.Hide()
	x.drain()
	if main.Visible() || !w.State().Hidden {
		t.Error("Hide did not hide the window")
	}
	w.Show()
	x.drain()
	if !main.Visible() || w.State().Hidden || !w.State().Focused {
		t.Errorf("Show: visible %v, state %+v", main.Visible(), w.State())
	}
}

// The initial placement opens the window where it was, and Placement
// reports it back.
func TestWindowPlacement(t *testing.T) {
	place := WindowPlacement{Width: 500, Height: 350, X: 10, Y: 20, HasPosition: true, RestoreToken: "headless:7"}
	x := newWinHarness(t, Options{Window: WindowSpec{Width: 640, Height: 480, Placement: place}, Root: div(Text("x"))})
	main := x.windows()[0]
	o := main.Options()
	if o.Size != (platform.Size{W: 500, H: 350}) || !o.HasPosition || o.Position != (image.Point{X: 10, Y: 20}) || o.RestoreToken != "headless:7" {
		t.Errorf("window options %+v", o)
	}
	if got := x.h.Handle(main).Placement(); got != place {
		t.Errorf("Placement() = %+v, want %+v", got, place)
	}
	main.Resize(platform.Size{W: 450, H: 320})
	main.SetState(platform.WindowState{HasPosition: true, Position: image.Point{X: 30, Y: 40}})
	x.drain()
	want := WindowPlacement{Width: 450, Height: 320, X: 30, Y: 40, HasPosition: true, RestoreToken: "headless:7"}
	if got := x.h.Handle(main).Placement(); got != want {
		t.Errorf("after a move and a resize Placement() = %+v, want %+v", got, want)
	}
}

// A window opened maximized keeps the size it returns to in its
// placement, although its first configure has the maximized size.
func TestWindowPlacementMaximized(t *testing.T) {
	place := WindowPlacement{Width: 500, Height: 350, Maximized: true}
	x := newWinHarness(t, Options{Window: WindowSpec{Placement: place}, Root: div(Text("x"))})
	main := x.windows()[0]
	if !main.OSState().Maximized || main.Metrics().Size == (platform.Size{W: 500, H: 350}) {
		t.Fatalf("window not opened maximized: %v %v", main.OSState(), main.Metrics().Size)
	}
	if got := x.h.Handle(main).Placement(); got.Width != 500 || got.Height != 350 || !got.Maximized {
		t.Errorf("Placement() = %+v, want 500x350 maximized", got)
	}
}

// Minimizing a maximized window keeps its placement maximized: a
// minimized window reports the state it returns to (platform.WindowState;
// Win32 reads WPF_RESTORETOMAXIMIZED, since IsZoomed is false while it is
// iconic), so an app that quits while minimized reopens maximized.
func TestWindowPlacementMinimizedMaximized(t *testing.T) {
	x := newWinHarness(t, Options{Root: div(Text("x"))})
	main := x.windows()[0]
	w := x.h.Handle(main)
	w.Maximize()
	x.drain()
	w.Minimize()
	x.drain()
	if st := w.State(); !st.Minimized || !st.Maximized {
		t.Fatalf("state of the minimized maximized window %+v", st)
	}
	if p := w.Placement(); !p.Maximized || p.Width != 400 || p.Height != 300 {
		t.Errorf("Placement() of the minimized maximized window = %+v, want 400x300 maximized", p)
	}
	w.Focus()
	x.drain()
	if p := w.Placement(); !p.Maximized || p.Width != 400 || p.Height != 300 {
		t.Errorf("Placement() after restoring = %+v, want 400x300 maximized", p)
	}
}

// Without Options.OnClose, closing the main window quits; with it, the
// app decides (here: close to tray) and keeps running.
func TestMainWindowClosePolicy(t *testing.T) {
	var got []WindowHandle
	x := newWinHarness(t, Options{Root: div(Text("x")), OnClose: func(w WindowHandle) {
		got = append(got, w)
		w.Hide()
	}})
	main := x.windows()[0]
	main.RequestClose()
	x.drain()
	if len(got) != 1 || got[0] != x.h.Handle(main) {
		t.Fatalf("OnClose calls %v", got)
	}
	if x.b.Quitting() || main.Closed() || main.Visible() {
		t.Errorf("close to tray: quitting %v, closed %v, visible %v", x.b.Quitting(), main.Closed(), main.Visible())
	}
	// Close on the handle asks the same thing.
	got[0].Close()
	x.drain()
	if len(got) != 2 {
		t.Errorf("WindowHandle.Close ran OnClose %d times in all, want 2", len(got))
	}
}

// A child window's close request runs its OnClose, with the state of the
// component that declared it; the state unmounts it.
func TestChildWindowCloseRequest(t *testing.T) {
	root := Component(func() Node {
		open, setOpen := UseState(true)
		return div(If(open, win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "C"}, OnClose: func() { setOpen.Set(false) }, Root: div(Text("c"))})))
	})
	x := newWinHarness(t, Options{Root: root})
	c := x.child(0)
	c.RequestClose()
	x.drain()
	if !c.Closed() || len(x.windows()) != 1 || x.b.Quitting() {
		t.Errorf("after the close request: closed %v, %d windows, quitting %v", c.Closed(), len(x.windows()), x.b.Quitting())
	}
}

// A Window without OnClose cannot be closed by the user; dev mode warns
// when it opens.
func TestWindowWithoutOnClose(t *testing.T) {
	var warns []string
	x := newWinHarness(t, Options{
		Config: Config{Dev: true, Warn: func(m string) { warns = append(warns, m) }},
		Root:   div(win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "Stuck"}, Root: div(Text("c"))})),
	})
	c := x.child(0)
	c.RequestClose()
	x.drain()
	if c.Closed() {
		t.Error("a Window without OnClose closed")
	}
	found := false
	for _, w := range warns {
		if strings.Contains(w, `"Stuck" has no OnClose`) {
			found = true
		}
	}
	if !found {
		t.Errorf("no warning about the missing OnClose: %q", warns)
	}
}

// A nil Root panics in dev mode with the fix.
func TestWindowNeedsRoot(t *testing.T) {
	tr := NewTree(Config{Fonts: testFonts, Dev: true})
	msg := panicMessage(func() { tr.Mount(div(win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "Empty"}}))) })
	if !strings.Contains(msg, `"Empty" has no Root`) {
		t.Errorf("panic %q", msg)
	}
}

// Quit ends the app from any goroutine, through the tree that receives
// posts.
func TestQuit(t *testing.T) {
	x := newWinHarness(t, Options{Root: div(Text("x"))})
	stop := x.h.Tree().ReceivePosts()
	defer stop()
	done := make(chan struct{})
	go func() {
		Quit()
		close(done)
	}()
	<-done
	x.drain()
	if !x.b.Quitting() {
		t.Error("Quit did not quit")
	}
}

// Closing a window closes the windows opened from it first: nested
// Windows are children of the window they are declared in.
func TestNestedWindows(t *testing.T) {
	var setOpen Setter[bool]
	inner := win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "Inner"}, OnClose: func() {}, Root: div(Text("inner"))})
	root := Component(func() Node {
		open, set := UseState(true)
		setOpen = set
		return div(If(open, win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "Outer"}, OnClose: func() {}, Root: div(Text("outer"), inner)})))
	})
	x := newWinHarness(t, Options{Root: root})
	ws := x.windows()
	if len(ws) != 3 || ws[1].Title() != "Outer" || ws[2].Title() != "Inner" || ws[2].Parent() != ws[1] {
		t.Fatalf("windows %d, want main, Outer and Inner (child of Outer)", len(ws))
	}
	setOpen.Set(false)
	x.drain()
	if !ws[1].Closed() || !ws[2].Closed() || len(x.h.Windows()) != 1 {
		t.Errorf("after closing Outer: outer closed %v, inner closed %v", ws[1].Closed(), ws[2].Closed())
	}
}

// The color scheme and the theme are the app's; the scale is each
// window's.
func TestWindowSchemeThemeAndScale(t *testing.T) {
	x := newWinHarness(t, Options{Root: div(win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "C"}, OnClose: func() {}, Root: div(Text("c"))}))})
	c := x.child(0)
	ct := x.h.TreeOf(c)
	if ct.parser != x.h.Tree().parser {
		t.Error("the child window does not share the app's parser")
	}
	x.h.Tree().SetColorScheme(ColorSchemeDark)
	if ct.ColorScheme() != ColorSchemeDark {
		t.Error("the child window does not follow the app's color scheme")
	}
	x.h.Tree().SetTheme(Theme{Radius: 4})
	if ct.parser != x.h.Tree().parser {
		t.Error("the child window does not follow SetTheme")
	}
	c.SetScale(2)
	x.drain()
	if ct.Scale() != 2 || x.h.Tree().Scale() != 1 {
		t.Errorf("scales: child %v, main %v", ct.Scale(), x.h.Tree().Scale())
	}
}

// A tree without a window (Render, tela snapshot) opens no window, and
// UseWindow's state is a focused window.
func TestWindowWithoutHost(t *testing.T) {
	var st WindowState
	var w WindowHandle
	root := Component(func() Node {
		w = UseWindow()
		st = w.State()
		return div(Text("main"), win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "C"}, OnClose: func() {}, Root: div(Text("c"))}))
	})
	Render(root, RenderOptions{Config: Config{Fonts: testFonts}, Width: 200, Height: 100})
	if !st.Focused || w.IsZero() {
		t.Errorf("state %+v", st)
	}
	w.Maximize() // nothing to do, no panic
	if (WindowHandle{}).State() != (WindowState{}) || (WindowHandle{}).Placement() != (WindowPlacement{}) {
		t.Error("the zero handle is not empty")
	}
}

// A custom title bar: the drag region, its interactive holes, the window
// controls, recomputed when the layout changes and sent only then.
func TestWindowHitRegions(t *testing.T) {
	var closes int
	bar := div(Class("flex h-10"), WindowDrag(),
		div(Class("w-20 h-10"), Text("Title")),
		button(Class("w-10 h-10"), Text("M")),
		div(Class("w-10 h-10"), WindowNoDrag()),
		button(Class("w-10 h-10"), WindowControlProp(WindowMinimize), Text("_")),
		button(Class("w-10 h-10"), WindowControlProp(WindowMaximize), Text("o")),
		button(Class("w-10 h-10"), WindowControlProp(WindowClose), Text("x")),
	)
	x := newWinHarness(t, Options{Window: WindowSpec{Frameless: true}, Root: bar, OnClose: func(WindowHandle) { closes++ }})
	main := x.windows()[0]
	want := []platform.HitRegion{
		{Kind: platform.HitCaption, Rect: platform.Rect{X: 0, Y: 0, W: 400, H: 40}},
		{Kind: platform.HitClient, Rect: platform.Rect{X: 80, Y: 0, W: 40, H: 40}},
		{Kind: platform.HitClient, Rect: platform.Rect{X: 120, Y: 0, W: 40, H: 40}},
		{Kind: platform.HitMinimize, Rect: platform.Rect{X: 160, Y: 0, W: 40, H: 40}},
		{Kind: platform.HitMaximize, Rect: platform.Rect{X: 200, Y: 0, W: 40, H: 40}},
		{Kind: platform.HitClose, Rect: platform.Rect{X: 240, Y: 0, W: 40, H: 40}},
	}
	if got := main.HitRegions(); !platform.EqualHitRegions(got, want) {
		t.Fatalf("hit regions\n got %v\nwant %v", got, want)
	}
	if n := main.HitRegionChanges(); n != 1 {
		t.Errorf("%d SetHitRegions changes, want 1", n)
	}
	if !x.h.Handle(main).State().ClientSideDecorations {
		t.Error("a frameless window does not report client-side decorations")
	}
	// A press in the caption is the OS's: a move.
	main.Inject(down(20, 20))
	main.Inject(up(20, 20))
	x.drain()
	if main.CaptionDrags() != 1 {
		t.Errorf("%d caption drags, want 1", main.CaptionDrags())
	}
	// The controls act by themselves.
	x.click(main, 210, 20)
	if !main.OSState().Maximized {
		t.Error("the maximize control did not maximize")
	}
	want[0].Rect.W = 1920 // the caption follows the maximized width
	if got := main.HitRegions(); !platform.EqualHitRegions(got, want) {
		t.Errorf("maximized hit regions\n got %v\nwant %v", got, want)
	}
	x.click(main, 210, 20)
	if main.OSState().Maximized {
		t.Error("the maximize control did not restore")
	}
	x.click(main, 250, 20)
	if closes != 1 {
		t.Errorf("the close control ran OnClose %d times, want 1", closes)
	}
	// A frame that changes nothing sends nothing.
	n, frames := main.HitRegionChanges(), x.h.Tree().Stats().Frames
	x.h.Tree().markStyle(x.h.Tree().root)
	x.drain()
	if x.h.Tree().Stats().Frames == frames {
		t.Fatal("no frame ran")
	}
	if main.HitRegionChanges() != n {
		t.Error("an unchanged frame sent the hit regions again")
	}
	x.click(main, 170, 20)
	if !main.OSState().Minimized {
		t.Error("the minimize control did not minimize")
	}
}

// A window without title bar props never sends hit regions.
func TestNoHitRegionsWithoutProps(t *testing.T) {
	x := newWinHarness(t, Options{Root: div(Text("x"))})
	if n := x.windows()[0].HitRegionChanges(); n != 0 {
		t.Errorf("%d SetHitRegions changes, want 0", n)
	}
}

// OpenURL and OpenPath go to the backend's desktop services; bad URLs are
// errors that say what to write.
func TestOpenURLAndPath(t *testing.T) {
	x := newWinHarness(t, Options{Root: div(Text("x"))})
	stop := x.h.Tree().ReceivePosts()
	defer stop()
	if err := OpenURL("https://example.com/a"); err != nil {
		t.Fatal(err)
	}
	if err := OpenPath("notes.txt"); err != nil {
		t.Fatal(err)
	}
	op := x.b.Opened()
	if len(op) != 2 || op[0].Target != "https://example.com/a" || op[0].Path || op[1].Target != "notes.txt" || !op[1].Path {
		t.Errorf("opened %+v", op)
	}
	if err := OpenURL("example.com"); err == nil || !strings.Contains(err.Error(), "no scheme") {
		t.Errorf("OpenURL without a scheme: %v", err)
	}
	if err := OpenURL("file:///tmp/x"); err == nil || !strings.Contains(err.Error(), "OpenPath") {
		t.Errorf("OpenURL of a file URL: %v", err)
	}
	if err := OpenPath(""); err == nil {
		t.Error("OpenPath(\"\") is not an error")
	}
	x.b.SetUnsupported(headless.ServiceOpen)
	if err := OpenURL("https://example.com"); err == nil {
		t.Error("OpenURL on a desktop without the service is not an error")
	}
}

// The window sizes are checked with tela's names.
func TestWindowSpecChecks(t *testing.T) {
	b := headless.New(headless.Options{})
	msg := panicMessage(func() { NewHost(b, Options{Window: WindowSpec{Width: 300}, Root: div()}) })
	if !strings.Contains(msg, "tela.App sets only one of Width and Height") {
		t.Errorf("panic %q", msg)
	}
	tr := NewTree(Config{Fonts: testFonts})
	msg = panicMessage(func() { tr.Mount(win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "W", MinWidth: -1}, Root: div()})) })
	if !strings.Contains(msg, `tela.Window "W" has MinWidth -1`) {
		t.Errorf("panic %q", msg)
	}
}
