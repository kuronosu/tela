package core

import (
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
)

// TestFixedToTheWindow: a fixed box is placed against the window, not its
// positioned ancestors; a scroll of the page does not move it; ancestor
// overflow clips do not cut it; it is hit where it is painted.
func TestFixedToTheWindow(t *testing.T) {
	var ref ElementRef
	comp := func() Node {
		ref = UseElementRef()
		return div(Class("relative m-8 h-[1000px] p-4"),
			div(Key("clip"), Class("relative h-10 w-10 overflow-hidden"),
				div(Key("bar"), Class("fixed top-0 right-0 left-0 h-12 bg-white"))),
			div(Key("static"), Class("ml-5 fixed h-6 w-6")),
			div(Key("btn"), AttachRef(ref), Class("fixed right-4 bottom-4 size-10 rounded-full bg-rose-500")),
		)
	}
	tr := newTestTree(t, 400, 300, Component(comp), nil)
	tr.Frame()
	if r := mustFind(t, tr, "bar").Rect; r != (gfx.Rect{X0: 0, Y0: 0, X1: 400, Y1: 48}) {
		t.Errorf("bar at %v, want the window's top 48 px", r)
	}
	if r := mustFind(t, tr, "btn").Rect; r != (gfx.Rect{X0: 344, Y0: 244, X1: 384, Y1: 284}) {
		t.Errorf("btn at %v", r)
	}
	// Auto insets: the static position (after the 40 px tall clip box,
	// inside the padding, with its own margin).
	if r := mustFind(t, tr, "static").Rect; r.X0 != 32+16+20 || r.Y0 != 32+16+40 {
		t.Errorf("static-position box at %v, want (68, 88)", r)
	}
	// Not clipped by the overflow-hidden parent: a point far right of it
	// hits the bar.
	if el := tr.HitTest(300, 20); el == nil || el.key != "bar" {
		t.Errorf("hit %v, want bar", el)
	}
	tr.rootBox.el.box.scr.pos.Y = 0
	scrollRoot := func(y float32) {
		tr.setScroll(tr.rootBox.scr, gfx.Point{Y: y})
		tr.Frame()
	}
	scrollRoot(200)
	if r := tr.boxWindowRect(ref.element().box); r.Y0 != 244 {
		t.Errorf("after scrolling the page the button is at %v, want y 244", r)
	}
	if el := tr.HitTest(360, 258); el == nil || el.key != "btn" {
		t.Errorf("hit %v after scrolling, want btn", el)
	}
	if b := ref.Bounds(); b.Y != 244 {
		t.Errorf("Bounds %+v, want y 244", b)
	}
	// Focus does not scroll the page for a fixed box.
	tr.scrollIntoView(ref.element())
	if y := tr.rootBox.scr.pos.Y; y != 200 {
		t.Errorf("scrollIntoView of a fixed box scrolled the page to %g", y)
	}
	// The page's scrollable extent ignores fixed boxes.
	if m := tr.rootBox.scr.max.Y; m != 32+1000-300 {
		t.Errorf("root scroll extent %g, want 732 (the page, not the fixed boxes)", m)
	}
}

// TestFixedInsideTransform: a transformed (or filtered) ancestor is the
// containing block of fixed and absolute descendants, like Chromium.
func TestFixedInsideTransform(t *testing.T) {
	tr := newTestTree(t, 400, 300, div(Class("p-10"),
		div(Key("card"), Class("h-20 w-40 translate-x-1"),
			div(Key("fixed"), Class("fixed top-2 left-2 size-4"))),
		div(Key("blur"), Class("mt-4 h-20 w-40 blur-sm"),
			div(Key("abs"), Class("absolute right-0 bottom-0 size-4"))),
	), nil)
	tr.Frame()
	if r := mustFind(t, tr, "fixed").Rect; r.X0 != 48 || r.Y0 != 48 { // the card is at (40, 40)
		t.Errorf("fixed in a transformed box at %v, want (48, 48)", r)
	}
	if r := mustFind(t, tr, "abs").Rect; r.X1 != 200 || r.Y1 != 40+80+16+80 {
		t.Errorf("absolute in a filtered box at %v, want its bottom-right corner at (200, 216)", r)
	}
}

// TestFixedRelayout: a change inside a relayout boundary that holds a
// fixed box lays the fixed box out again against the window.
func TestFixedRelayout(t *testing.T) {
	var set Setter[int]
	comp := func() Node {
		v, s := UseState(0)
		set = s
		w := "w-10"
		if v == 1 {
			w = "w-1/2"
		}
		return div(Class("h-[100px] w-[100px] overflow-hidden"),
			div(Key("f"), Class("fixed top-0 left-0 h-4 "+w)))
	}
	tr := newTestTree(t, 400, 300, Component(comp), nil)
	tr.Frame()
	if r := mustFind(t, tr, "f").Rect; r.Width() != 40 {
		t.Fatalf("fixed width %g, want 40", r.Width())
	}
	set.Set(1)
	tr.Frame()
	if r := mustFind(t, tr, "f").Rect; r.Width() != 200 {
		t.Errorf("fixed w-1/2 width %g, want 200 (half the window)", r.Width())
	}
}

// TestFixedInsideAnimatedTransform: a keyframe animation of the transform
// (animate-spin) makes its box a containing block while it is attached,
// even where the animated value is the identity (Chromium counts running
// transform animations); a transition counts by its target.
func TestFixedInsideAnimatedTransform(t *testing.T) {
	var set Setter[bool]
	comp := func() Node {
		on, s := UseState(false)
		set = s
		cls := "mt-10 ml-10 size-20 transition-transform duration-500"
		if on {
			cls += " translate-x-4"
		}
		return div(
			div(Key("spin"), Class("ml-10 mt-10 size-20 animate-spin"),
				div(Key("f1"), Class("fixed top-0 left-0 size-4"))),
			div(Key("move"), Class(cls),
				div(Key("f2"), Class("fixed top-0 left-0 size-4"))),
		)
	}
	tr := newTestTree(t, 400, 300, Component(comp), nil)
	tr.Animate(0)
	tr.Frame()
	if r := mustFind(t, tr, "f1").Rect; r.X0 != 40 || r.Y0 != 40 {
		t.Errorf("fixed in a spinning box at %v, want its corner (40, 40)", r)
	}
	if r := mustFind(t, tr, "f2").Rect; r.X0 != 0 || r.Y0 != 0 {
		t.Errorf("fixed in an untransformed box at %v, want the window's corner", r)
	}
	set.Set(true)
	tr.Animate(16 * time.Millisecond)
	tr.Frame()
	// The transition just started: its value is still near the identity,
	// its target is not.
	if r := mustFind(t, tr, "f2").Rect; r.X0 != 40 || r.Y0 != 40+80+40 {
		t.Errorf("fixed in a box transitioning to a transform at %v, want (40, 160)", r)
	}
	if b := keyed(tr, "move").box; b.anim == nil || b.anim.target == nil {
		t.Errorf("no transition running on the box")
	}
}
