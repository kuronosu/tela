package core

import (
	"image"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// The app-scoped events of the backend reach the listeners of package
// desktop on the UI thread, and stop reaching them when they stop
// (ADR-181).
func TestDesktopListen(t *testing.T) {
	x := newWinHarness(t, Options{Root: div(Text("x"))})
	stopPosts := x.h.Tree().ReceivePosts()
	defer stopPosts()
	d, err := RunningDesktop("test")
	if err != nil {
		t.Fatal(err)
	}
	if d2, ok := DesktopOf(x.h.Handle(x.windows()[0])); !ok || d2 != d {
		t.Fatalf("DesktopOf(main) = %v, %v; want the running desktop", d2, ok)
	}
	tray, err := d.Services().NewTray(platform.TrayOptions{
		Icon: []image.Image{image.NewRGBA(image.Rect(0, 0, 16, 16))},
		Menu: []platform.MenuItem{{ID: 1, Label: "Quit"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	var got []platform.Event
	stop := d.Listen(func(e platform.Event) { got = append(got, e) })
	x.b.InjectApp(platform.TrayEvent{Tray: tray, Kind: platform.TrayMenuItem, Item: 1})
	x.drain()
	if len(got) != 1 || got[0] != (platform.TrayEvent{Tray: tray, Kind: platform.TrayMenuItem, Item: 1}) {
		t.Fatalf("listener got %v", got)
	}
	stop()
	x.b.InjectApp(platform.TrayEvent{Tray: tray, Kind: platform.TrayActivated})
	x.drain()
	if len(got) != 1 {
		t.Errorf("a stopped listener got %v", got[1:])
	}
	if a, b := d.Local("k", func() any { return 1 }), d.Local("k", func() any { return 2 }); a != 1 || b != 1 {
		t.Errorf("Local = %v then %v, want 1 twice", a, b)
	}
}

// Without a running app, RunningDesktop says how to call it.
func TestRunningDesktopWithoutApp(t *testing.T) {
	if _, err := RunningDesktop("desktop.OpenFile"); err == nil || !strings.Contains(err.Error(), "needs a running app") {
		t.Errorf("RunningDesktop without an app: %v", err)
	}
	if _, ok := DesktopOf(WindowHandle{}); ok {
		t.Error("DesktopOf(zero handle) is ok")
	}
}

// WatchPlacement reports each change of a window's placement, and nothing
// after stop.
func TestWatchPlacement(t *testing.T) {
	x := newWinHarness(t, Options{Window: WindowSpec{Width: 400, Height: 300}, Root: div(Text("x"))})
	main := x.windows()[0]
	var got []WindowPlacement
	stop := WatchPlacement(x.h.Handle(main), func(p WindowPlacement) { got = append(got, p) })
	main.Resize(platform.Size{W: 450, H: 320})
	x.drain()
	main.SetState(platform.WindowState{Maximized: true})
	x.drain()
	if len(got) != 2 || got[0].Width != 450 || got[0].Height != 320 || got[0].Maximized || !got[1].Maximized || got[1].Width != 450 {
		t.Fatalf("placements %+v; want 450x320, then 450x320 maximized", got)
	}
	stop()
	main.SetState(platform.WindowState{})
	x.drain()
	if len(got) != 2 {
		t.Errorf("a stopped watcher got %+v", got[2:])
	}
	if s := WatchPlacement(WindowHandle{}, func(WindowPlacement) {}); s == nil {
		t.Error("WatchPlacement of the zero handle returned a nil stop")
	}
}

// A window opened by code is a modal child of the window it is opened
// from, runs OnClose on a close request, closes with Close, and closes
// with its parent's tree.
func TestOpenWindowByCode(t *testing.T) {
	x := newWinHarness(t, Options{Root: div(Text("main"))})
	main := x.windows()[0]
	closes := 0
	open := func() *CodeWindow {
		cw, err := OpenWindow(x.h.Handle(main), ChildWindowSpec{
			WindowSpec: WindowSpec{Title: "Box", Width: 300, Height: 120},
			Modal:      true,
			OnClose:    func() { closes++ },
			Root:       div(Text("box content")),
		})
		if err != nil {
			t.Fatal(err)
		}
		x.drain()
		return cw
	}
	cw := open()
	c := x.child(0)
	if c.Title() != "Box" || c.Parent() != main || !c.Options().Modal {
		t.Errorf("window: title %q, parent %v, options %+v", c.Title(), c.Parent(), c.Options())
	}
	if s := x.snapshot(c); !strings.Contains(s, "box content") {
		t.Errorf("the window shows %q", s)
	}
	c.RequestClose()
	x.drain()
	if closes != 1 || c.Closed() {
		t.Errorf("after a close request: %d OnClose runs, closed %v; want 1 and open", closes, c.Closed())
	}
	cw.Update(ChildWindowSpec{
		WindowSpec: WindowSpec{Title: "Box 2", Width: 300, Height: 160},
		Modal:      true, OnClose: func() {}, Root: div(Text("new content")),
	})
	x.drain()
	if c.Title() != "Box 2" || !strings.Contains(x.snapshot(c), "new content") {
		t.Errorf("after Update: title %q, content %q", c.Title(), x.snapshot(c))
	}
	cw.Close()
	x.drain()
	if !c.Closed() || !cw.Closed() || len(x.windows()) != 1 {
		t.Errorf("after Close: closed %v, %d windows", c.Closed(), len(x.windows()))
	}

	cw = open()
	c = x.child(0)
	x.h.Tree().Unmount()
	x.drain()
	if !c.Closed() || !cw.Closed() {
		t.Errorf("the parent tree unmounted: window closed %v", c.Closed())
	}

	if _, err := OpenWindow(WindowHandle{}, ChildWindowSpec{Root: div()}); err == nil {
		t.Error("OpenWindow from the zero handle is not an error")
	}
}

// A press in the parent of a window opened by code with Modal is dropped,
// as for a modal Window component.
func TestOpenWindowByCodeIsModal(t *testing.T) {
	clicks := 0
	x := newWinHarness(t, Options{Root: div(button(Class("block w-40 h-10"), On("click", func(MouseEvent) { clicks++ }), Text("press")))})
	main := x.windows()[0]
	cw, err := OpenWindow(x.h.Handle(main), ChildWindowSpec{
		WindowSpec: WindowSpec{Title: "Box", Width: 200, Height: 100},
		Modal:      true, OnClose: func() {}, Root: div(Text("box")),
	})
	if err != nil {
		t.Fatal(err)
	}
	x.drain()
	if !main.Blocked() {
		t.Error("the parent of a modal window opened by code is not blocked")
	}
	x.click(main, 20, 20)
	if clicks != 0 {
		t.Errorf("%d clicks reached the blocked parent", clicks)
	}
	cw.Close()
	x.drain()
	x.click(main, 20, 20)
	if clicks != 1 {
		t.Errorf("%d clicks after the window closed, want 1", clicks)
	}
}
