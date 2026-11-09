package core

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

// WindowResize (ADR-257): a band along the inside of the card's border
// box, sides then corners, on top of the drag area inside the card, cut
// by the window; presses there are the OS's resize, elsewhere content.
func TestWindowResizeRegions(t *testing.T) {
	var clicks int
	card := div(Class("absolute left-[20px] top-[10px] w-[200px] h-[100px]"), WindowResize(),
		div(Class("h-8"), WindowDrag()),
		button(Class("w-10 h-10"), On("click", func(MouseEvent) { clicks++ }), Text("B")),
	)
	x := newWinHarness(t, Options{Window: WindowSpec{Frameless: true}, Root: div(Class("relative w-full h-full"), card)})
	main := x.windows()[0]
	r := func(k platform.HitKind, x0, y0, w, h float32) platform.HitRegion {
		return platform.HitRegion{Kind: k, Rect: platform.Rect{X: x0, Y: y0, W: w, H: h}}
	}
	want := []platform.HitRegion{
		r(platform.HitCaption, 20, 10, 200, 32),
		r(platform.HitResizeTop, 20, 10, 200, 5),
		r(platform.HitResizeBottom, 20, 105, 200, 5),
		r(platform.HitResizeLeft, 20, 10, 5, 100),
		r(platform.HitResizeRight, 215, 10, 5, 100),
		r(platform.HitResizeTopLeft, 20, 10, 16, 5),
		r(platform.HitResizeTopLeft, 20, 10, 5, 16),
		r(platform.HitResizeTopRight, 204, 10, 16, 5),
		r(platform.HitResizeTopRight, 215, 10, 5, 16),
		r(platform.HitResizeBottomLeft, 20, 105, 16, 5),
		r(platform.HitResizeBottomLeft, 20, 94, 5, 16),
		r(platform.HitResizeBottomRight, 204, 105, 16, 5),
		r(platform.HitResizeBottomRight, 215, 94, 5, 16),
	}
	if got := main.HitRegions(); !platform.EqualHitRegions(got, want) {
		t.Fatalf("hit regions\n got %v\nwant %v", got, want)
	}
	for _, c := range []struct {
		x, y float32
		want platform.Edge
	}{
		{100, 12, platform.EdgeTop}, // over the drag area: the band wins
		{22, 12, platform.EdgeTop | platform.EdgeLeft},
		{22, 50, platform.EdgeLeft},
		{218, 108, platform.EdgeBottom | platform.EdgeRight},
		{100, 50, 0},
	} {
		if e := platform.HitEdge(main.HitRegions(), platform.Point{X: c.x, Y: c.y}); e != c.want {
			t.Errorf("edge at %v,%v = %v, want %v", c.x, c.y, e, c.want)
		}
	}
	// The press on the left side is the OS's: a resize, not delivered.
	main.Inject(down(21, 60))
	main.Inject(up(21, 60))
	x.drain()
	if got := main.ResizeStarts(); len(got) != 1 || got[0] != platform.EdgeLeft {
		t.Errorf("resize starts %v, want [left]", got)
	}
	// Maximized, the handles are content: the "OS" does not resize.
	main.SetState(platform.WindowState{Maximized: true, ClientSideDecorations: true})
	x.drain()
	main.Inject(down(21, 60))
	main.Inject(up(21, 60))
	x.drain()
	if n := len(main.ResizeStarts()); n != 1 {
		t.Errorf("%d resize starts while maximized, want still 1", n)
	}
	if clicks != 1 {
		t.Errorf("%d clicks, want the one on the button under the band while maximized", clicks)
	}
}

// Only the sides asked for, a band of another width, and the band cut by
// the window and a clipping ancestor.
func TestWindowResizeOptions(t *testing.T) {
	card := div(Class("absolute left-[-10px] top-0 w-[100px] h-[50px]"),
		WindowResize(ResizeEdges(EdgeLeft|EdgeRight|EdgeBottom), ResizeBand(8)))
	x := newWinHarness(t, Options{Window: WindowSpec{Frameless: true}, Root: div(Class("relative w-full h-full"), card)})
	main := x.windows()[0]
	r := func(k platform.HitKind, x0, y0, w, h float32) platform.HitRegion {
		return platform.HitRegion{Kind: k, Rect: platform.Rect{X: x0, Y: y0, W: w, H: h}}
	}
	want := []platform.HitRegion{
		r(platform.HitResizeBottom, 0, 42, 90, 8),
		r(platform.HitResizeRight, 82, 0, 8, 50),
		r(platform.HitResizeBottomLeft, 0, 42, 6, 8),
		r(platform.HitResizeBottomRight, 74, 42, 16, 8),
		r(platform.HitResizeBottomRight, 82, 34, 8, 16),
	}
	if got := main.HitRegions(); !platform.EqualHitRegions(got, want) {
		t.Fatalf("hit regions\n got %v\nwant %v", got, want)
	}
	for _, c := range []struct {
		want string
		f    func()
	}{
		{"positive", func() { ResizeBand(0) }},
		{"EdgeTop", func() { ResizeEdges(0) }},
		{"zero tela.ResizeOption", func() { WindowResize(ResizeOption{}) }},
	} {
		func() {
			defer func() {
				if s, _ := recover().(string); !strings.Contains(s, c.want) {
					t.Errorf("panic %q, want one containing %q", s, c.want)
				}
			}()
			c.f()
		}()
	}
}

// A tree whose only region prop is WindowResize still computes regions,
// and they go when it unmounts.
func TestWindowResizeUnmount(t *testing.T) {
	show := true
	var set func(bool)
	app := func() Node {
		s, setS := UseState(show)
		set = func(v bool) { setS.Set(v) }
		if !s {
			return div(Class("w-full h-full"))
		}
		return div(Class("w-full h-full"), WindowResize())
	}
	x := newWinHarness(t, Options{Window: WindowSpec{Frameless: true}, Root: Component(app)})
	main := x.windows()[0]
	if len(main.HitRegions()) != 12 {
		t.Fatalf("%d regions, want 12", len(main.HitRegions()))
	}
	set(false)
	x.drain()
	if n := len(main.HitRegions()); n != 0 {
		t.Errorf("%d regions after unmount", n)
	}
}

// Content in a portal layer paints above the card, so it takes the
// presses over the card's resize band and drag area (ADR-261): a
// popover over the top edge and a menu over the drag area punch HitClient
// holes after the card's regions; a pointer-events-none overlay and the
// portal's empty parts punch none, and elements inside a hole add none.
func TestWindowResizePortalOverBand(t *testing.T) {
	var clicks int
	card := div(Class("absolute left-[20px] top-[10px] w-[200px] h-[100px]"), WindowResize(),
		div(Class("h-8"), WindowDrag()),
		Portal(
			div(Class("absolute left-[150px] top-[0px] w-[60px] h-[40px]"), On("click", func(MouseEvent) { clicks++ }),
				div(Class("w-full h-4")), // inside the popover: no region of its own
			),
			div(Class("fixed inset-0 pointer-events-none")), // a toast layer
		),
	)
	far := Portal(div(Class("absolute left-[300px] top-[150px] w-[20px] h-[20px]"))) // over nothing
	x := newWinHarness(t, Options{Window: WindowSpec{Frameless: true},
		Root: div(Class("relative w-full h-full"), card, far)})
	main := x.windows()[0]
	got := main.HitRegions()
	hole := platform.HitRegion{Kind: platform.HitClient, Rect: platform.Rect{X: 150, Y: 0, W: 60, H: 40}}
	if n := len(got); n != 14 || got[n-1] != hole {
		t.Fatalf("hit regions %v\nwant the card's 13 then %v", got, hole)
	}
	for _, c := range []struct {
		x, y float32
		want platform.Edge
	}{
		{160, 12, 0},                // the popover over the top band
		{100, 12, platform.EdgeTop}, // the band beside it
		{218, 50, platform.EdgeRight},
	} {
		if e := platform.HitEdge(got, platform.Point{X: c.x, Y: c.y}); e != c.want {
			t.Errorf("edge at %v,%v = %v, want %v", c.x, c.y, e, c.want)
		}
	}
	if k := platform.HitTest(got, platform.Point{X: 160, Y: 30}); k != platform.HitClient {
		t.Errorf("the popover over the drag area is %v, want client", k)
	}
	main.Inject(down(160, 12))
	main.Inject(up(160, 12))
	x.drain()
	if clicks != 1 || len(main.ResizeStarts()) != 0 {
		t.Errorf("%d clicks and resizes %v, want the popover's click and no resize", clicks, main.ResizeStarts())
	}
}
