package core

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
)

// Right-to-left scroll containers (ADR-090): the scroll origin is at the
// right, scrollLeft is 0 there and negative toward the left, user deltas
// keep their physical meaning, and the vertical bar is on the left.

func TestDirScroll(t *testing.T) {
	strip := []Child{Key("strip"), Dir("rtl"), Class("flex w-[200px] h-[50px] overflow-x-auto")}
	for i := range 10 {
		strip = append(strip, div(Key(fmt.Sprintf("c%d", i)), Class("h-[50px] w-[50px] shrink-0")))
	}
	ref := &elemRef{}
	strip = append(strip, AttachRef(ElementRef{r: ref}))
	root := div(div(strip...),
		div(Key("list"), Dir("rtl"), Class("h-[100px] w-[200px] overflow-y-auto"), div(Key("tall"), Class("h-[400px]"))))
	x := newScrollHost(t, root, 400, 300, Config{InstantScroll: true})
	tr := x.tree()
	s := scr(t, tr, "strip")
	pub := func() float32 { v, _ := ElementRef{r: ref}.ScrollOffset(); return v }
	if r := dirRect(t, tr, "c0"); r.X0 != 150 || pub() != 0 || s.max.X != 300 {
		t.Fatalf("start: c0 at %v, scrollLeft %v, max %v", r.X0, pub(), s.max.X)
	}
	horiz := func(dx float32) platform.ScrollEvent {
		return platform.ScrollEvent{Pos: platform.Point{X: 100, Y: 25}, Delta: platform.Point{X: dx}}
	}
	x.wheel(horiz(40)) // toward the right: already at the start
	x.frame()
	if pub() != 0 {
		t.Errorf("scrolling right at the start moved to %v", pub())
	}
	// The thumb starts at the right.
	if g := s.geo[0]; !g.ok || g.thumb.X1 < 190 {
		t.Errorf("horizontal thumb at %v, want at the right end", g.thumb)
	}
	x.wheel(horiz(-40))
	x.frame()
	if pub() != -40 {
		t.Errorf("scrolling left: scrollLeft %v, want -40", pub())
	}
	if r := dirRect(t, tr, "c0"); r.X0 != 190 {
		t.Errorf("after scrolling left c0 at %v, want 190", r.X0)
	}
	if g := s.geo[0]; g.thumb.X1 > 199 || g.thumb.X0 < 1 {
		t.Errorf("thumb %v did not move left", g.thumb)
	}
	ElementRef{r: ref}.ScrollTo(-1000, 0)
	x.frame()
	if pub() != -300 {
		t.Errorf("ScrollTo(-1000, 0): %v, want -300", pub())
	}
	if r := dirRect(t, tr, "c9"); r.X0 != 0 {
		t.Errorf("at the end c9 at %v, want 0", r.X0)
	}
	ElementRef{r: ref}.ScrollBy(100, 0)
	x.frame()
	if pub() != -200 {
		t.Errorf("ScrollBy(100, 0): %v, want -200", pub())
	}
	if ev := s.event(); ev.ScrollLeft != -200 || ev.ScrollWidth != 500 {
		t.Errorf("OnScroll event %+v", ev)
	}
	// The vertical overlay bar is on the left.
	l := scr(t, tr, "list")
	tr.scrollScripted(l, gfx.Point{Y: 10})
	x.frame()
	if g := l.geo[1]; !g.ok || g.track.X0 > 10 {
		t.Errorf("rtl vertical bar at %v, want on the left", g.track)
	}
}

func TestDirClassicScrollbars(t *testing.T) {
	root := div(div(Key("list"), Dir("rtl"), Class("h-[100px] w-[200px] overflow-y-scroll"),
		div(Key("row"), Class("h-[400px]"))))
	x := newScrollHost(t, root, 400, 300, Config{ClassicScrollbars: true})
	tr := x.tree()
	if r := dirRect(t, tr, "row"); r.X0 != classicGutter || r.X1 != 200 {
		t.Errorf("row %v: want the gutter on the left (x from %d)", r, classicGutter)
	}
	l := scr(t, tr, "list")
	if g := l.geo[1]; !g.ok || g.track.X0 != 0 || g.track.X1 != classicGutter {
		t.Errorf("classic vertical bar %v, want x 0 to %d", g.track, classicGutter)
	}
	if c := tr.clientRect(keyed(tr, "list").box); c.X0 != classicGutter || c.X1 != 200 {
		t.Errorf("client rect %v", c)
	}
}
