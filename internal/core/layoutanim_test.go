package core

import (
	"fmt"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/style"
)

// Tests of layout animations (layoutanim.go) on the frame clock of a
// headless window.

// vis returns the window bounds of the element with key k as painted
// (ElementRef.Bounds: transforms included).
func (x *inputHarness) vis(k string) gfx.Rect {
	x.t.Helper()
	el := keyed(x.tree(), k)
	if el == nil {
		x.t.Fatalf("%q is not mounted", k)
	}
	return x.tree().elemBounds(el)
}

// laid returns the layout box of the element with key k in window
// coordinates (no transform).
func (x *inputHarness) laid(k string) gfx.Rect {
	x.t.Helper()
	return windowRect(keyed(x.tree(), k).box)
}

func nearRect(a, b gfx.Rect, eps float32) bool {
	return near32(a.X0, b.X0, eps) && near32(a.Y0, b.Y0, eps) && near32(a.X1, b.X1, eps) && near32(a.Y1, b.Y1, eps)
}

// orderProbe renders keyed rows in the order of its state.
type orderProbe struct {
	set   Setter[[]string]
	class string
	extra []Child
}

func (o *orderProbe) root(initial ...string) Node {
	return Component(func() Node {
		items, set := UseState(initial)
		o.set = set
		return div(Class("flex flex-col gap-2 p-2"), Map(items, func(k string) Node {
			c := []Child{Key(k), Class("h-10 rounded-lg bg-sky-500 " + o.class), AnimateLayout(), Text(k)}
			return div(append(c, o.extra...)...)
		}))
	})
}

func TestLayoutReorder(t *testing.T) {
	var o orderProbe
	x := newHarness(t, o.root("a", "b", "c"), false)
	before := map[string]gfx.Rect{"a": x.laid("a"), "b": x.laid("b"), "c": x.laid("c")}
	if !x.quiet() {
		t.Fatal("not idle at rest")
	}
	o.set.Set([]string{"c", "b", "a"})
	x.frame() // lays out the new order: the boxes are drawn where they were
	for _, k := range []string{"a", "b", "c"} {
		if v := x.vis(k); !nearRect(v, before[k], 0.01) {
			t.Errorf("%s at the frame of the change: %v, want its old box %v", k, v, before[k])
		}
	}
	if x.laid("a") == before["a"] {
		t.Fatal("the layout did not change")
	}
	if !layoutActive(keyed(x.tree(), "a").box) || !keyed(x.tree(), "a").box.layerSC {
		t.Error("an animating box is not a layer of its own")
	}
	// It moves toward the new box: a goes down, c up, b stays.
	s0 := x.tree().Stats()
	var ya []float32
	for range 6 {
		x.frame()
		ya = append(ya, x.vis("a").Y0)
		if v := x.vis("b"); !nearRect(v, before["b"], 0.01) {
			t.Errorf("b, which did not move, is drawn at %v", v)
		}
	}
	for i := 1; i < len(ya); i++ {
		if ya[i] <= ya[i-1] {
			t.Errorf("a does not move down: %v", ya)
			break
		}
	}
	if ya[0] <= before["a"].Y0 || ya[len(ya)-1] >= x.laid("a").Y0+5 {
		t.Errorf("a's path %v, from %v to %v", ya, before["a"].Y0, x.laid("a").Y0)
	}
	if d := work(s0, x.tree().Stats()); d.Builds != 0 || d.Styles != 0 || d.Layouts != 0 {
		t.Errorf("animation frames did more than paint: %+v", d)
	}
	x.settle()
	for _, k := range []string{"a", "b", "c"} {
		bx := keyed(x.tree(), k).box
		if v := x.vis(k); v != x.laid(k) {
			t.Errorf("%s settled at %v, laid out at %v", k, v, x.laid(k))
		}
		if !bx.pcs.Transform.IsIdentity() || layoutActive(bx) {
			t.Errorf("%s keeps a transform at rest: %+v", k, bx.pcs.Transform)
		}
	}
	if !x.quiet() {
		t.Error("not idle after the animation")
	}
}

// TestLayoutSpringTiming: without duration or ease classes the motion is
// the layout spring; with them, a tween of their values.
func TestLayoutSpringTiming(t *testing.T) {
	var o orderProbe
	x := newHarness(t, o.root("a", "b"), false)
	o.set.Set([]string{"b", "a"})
	x.frame()
	la := keyed(x.tree(), "a").box.lanim
	if !la.running || !la.m.Spec.Spring || la.m.Spec.Physics.Stiffness != 500 || la.m.Spec.Physics.Damping != 25 {
		t.Errorf("default motion %+v", la.m.Spec)
	}
	x.settle()

	o2 := orderProbe{class: "duration-200 ease-linear"}
	y := newHarness(t, o2.root("a", "b"), false)
	from, to := y.laid("a"), y.laid("b")
	o2.set.Set([]string{"b", "a"})
	y.frame()
	la = keyed(y.tree(), "a").box.lanim
	if la.m.Spec.Spring || la.m.Spec.Duration != 200*time.Millisecond {
		t.Fatalf("tween %+v", la.m.Spec)
	}
	start := la.m.Start
	y.advanceTo(start + 50*time.Millisecond)
	want := from.Y0 + (to.Y0-from.Y0)/4
	if v := y.vis("a"); !near32(v.Y0, want, 0.01) {
		t.Errorf("a quarter of the way: %v, want %v", v.Y0, want)
	}
	y.advanceTo(start + 200*time.Millisecond)
	if v := y.vis("a"); v != y.laid("a") {
		t.Errorf("at the end: %v, want %v", v, y.laid("a"))
	}
	if !y.quiet() {
		t.Error("not idle after the tween")
	}
}

// TestLayoutInterruption: a change mid-flight starts from the visual box
// of that frame, keeping the speed.
func TestLayoutInterruption(t *testing.T) {
	var o orderProbe
	x := newHarness(t, o.root("a", "b", "c"), false)
	o.set.Set([]string{"b", "c", "a"}) // a goes down
	x.frame()
	x.runFor(60 * time.Millisecond)
	la := keyed(x.tree(), "a").box.lanim
	old := la.m
	o.set.Set([]string{"a", "b", "c"}) // and back up
	x.frame()
	now := x.tree().anim.tk.Now()
	pos, vel, _ := old.At(now)
	if !la.running || la.m.Start != now {
		t.Fatalf("no new motion at the interruption: %+v", la.m)
	}
	if la.m.From.C[1] != pos.C[1] || la.m.Vel.C[1] != vel.C[1] || vel.C[1] <= 0 {
		t.Errorf("the new motion starts at y %v moving %v/s; the old one was at %v moving %v/s",
			la.m.From.C[1], la.m.Vel.C[1], pos.C[1], vel.C[1])
	}
	if v := x.vis("a"); !near32(v.Y0, float32(pos.C[1]), 0.01) {
		t.Errorf("drawn at %v, want %v", v.Y0, pos.C[1])
	}
	// The velocity carries it further down before it turns back.
	y0 := x.vis("a").Y0
	x.frame()
	if y1 := x.vis("a").Y0; y1 <= y0 {
		t.Errorf("the velocity was lost: %v then %v", y0, y1)
	}
	x.settle()
	if x.vis("a") != x.laid("a") || !x.quiet() {
		t.Errorf("settled at %v (laid %v), idle %v", x.vis("a"), x.laid("a"), x.quiet())
	}
}

// TestLayoutExpand: a size change is a scale of the box toward its new
// size; its radius is corrected.
func TestLayoutExpand(t *testing.T) {
	var set Setter[bool]
	root := Component(func() Node {
		open, s := UseState(false)
		set = s
		h := "h-10"
		if open {
			h = "h-40"
		}
		return div(Class("p-4"),
			div(Key("card"), Class("w-40 rounded-xl bg-white shadow-md "+h), AnimateLayout(),
				div(Key("title"), Class("h-6 bg-slate-300"), AnimateLayout())),
			div(Key("below"), Class("h-8 bg-sky-500"), AnimateLayout()))
	})
	x := newHarness(t, root, false)
	before := x.vis("card")
	set.Set(true)
	x.frame()
	if v := x.vis("card"); !nearRect(v, before, 0.01) {
		t.Errorf("card at the change: %v, want %v", v, before)
	}
	x.runFor(80 * time.Millisecond)
	v, laid := x.vis("card"), x.laid("card")
	if !(v.Height() > before.Height() && v.Height() < laid.Height()) || !near32(v.Width(), laid.Width(), 0.01) {
		t.Errorf("card mid-way %v, from %v to %v", v, before, laid)
	}
	bx := keyed(x.tree(), "card").box
	if sy := bx.lanim.scale.Y; !(sy > 0 && sy < 1) {
		t.Errorf("card scale %v", bx.lanim.scale)
	}
	// The radius is corrected: geom divides its vertical radii by the
	// scale, so that the drawn corner stays 12 px.
	p := painter{t: x.tree(), scale: 1}
	g := p.geom(bx)
	if r := g.border.Radii[0]; !near32(r.X, 12, 0.01) || !near32(r.Y*bx.lanim.scale.Y, 12, 0.05) {
		t.Errorf("corrected radius %v at scale %v", r, bx.lanim.scale)
	}
	// The child is counter-scaled: its drawn height stays 24 px, at the
	// top of the card.
	if tv := x.vis("title"); !near32(tv.Height(), 24, 0.05) || !near32(tv.Y0, v.Y0, 0.05) {
		t.Errorf("title drawn at %v in the card %v", tv, v)
	}
	// The box below moves down with the card's bottom edge.
	if bv := x.vis("below"); !near32(bv.Y0, v.Y1, 1) {
		t.Errorf("below at %v, card bottom %v", bv.Y0, v.Y1)
	}
	x.settle()
	if x.vis("card") != x.laid("card") || x.vis("title") != x.laid("title") || !x.quiet() {
		t.Error("not settled")
	}
}

// TestLayoutScrollDoesNotAnimate: scrolling moves boxes on screen but not
// in their layout: nothing animates.
func TestLayoutScrollDoesNotAnimate(t *testing.T) {
	var items []Child
	items = append(items, Key("sc"), Class("h-[150px] overflow-y-auto"))
	for i := range 20 {
		k := fmt.Sprint("r", i)
		items = append(items, div(Key(k), Class("h-10 bg-sky-500"), AnimateLayout(), Text(k)))
	}
	x := newHarness(t, div(div(items...)), false)
	y0 := x.vis("r3").Y0
	x.send(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: 50, Y: 50}})
	x.w.Inject(platform.ScrollEvent{Pos: platform.Point{X: 50, Y: 50}, Delta: platform.Point{Y: 100}})
	x.settle()
	if v := x.vis("r3"); near32(v.Y0, y0, 0.5) {
		t.Fatalf("did not scroll: %v", v)
	}
	for i := range 20 {
		if la := keyed(x.tree(), fmt.Sprint("r", i)).box.lanim; la == nil || la.running || la.active {
			t.Fatalf("r%d animates after a scroll: %+v", i, la)
		}
	}
}

// TestLayoutNested: a child that keeps its place in a parent that moves
// moves with it, whatever its own timing.
func TestLayoutNested(t *testing.T) {
	var set Setter[bool]
	root := Component(func() Node {
		down, s := UseState(false)
		set = s
		top := "mt-0"
		if down {
			top = "mt-[100px]"
		}
		return div(
			div(Key("parent"), Class("w-40 h-20 bg-slate-200 "+top), AnimateLayout(),
				div(Key("child"), Class("ml-4 mt-4 size-8 bg-rose-500 duration-1000 ease-linear"), AnimateLayout())))
	})
	x := newHarness(t, root, false)
	set.Set(true)
	x.frame()
	for range 5 {
		x.frame()
		pv, cv := x.vis("parent"), x.vis("child")
		if !near32(cv.X0-pv.X0, 16, 0.01) || !near32(cv.Y0-pv.Y0, 16, 0.01) || !near32(cv.Height(), 32, 0.01) {
			t.Fatalf("child %v not glued to parent %v", cv, pv)
		}
	}
	if la := keyed(x.tree(), "child").box.lanim; la.running {
		t.Error("the child runs a motion of its own")
	}
	x.settle()
	if !x.quiet() || x.vis("child") != x.laid("child") {
		t.Error("not settled")
	}
}

// TestLayoutIDHandoff: an element with a LayoutID that mounts in the
// commit where another with the same ID unmounts starts from its box.
func TestLayoutIDHandoff(t *testing.T) {
	var set Setter[int]
	root := Component(func() Node {
		tab, s := UseState(0)
		set = s
		var tabs []Child
		tabs = append(tabs, Class("flex gap-4 p-2"))
		for i := range 3 {
			var under Node
			if i == tab {
				under = div(Key("u"), Class("h-1 bg-indigo-500"), LayoutID("underline"))
			}
			tabs = append(tabs, div(Key(fmt.Sprint("tab", i)), Class("w-20 h-10 flex flex-col justify-end"), under))
		}
		return div(tabs...)
	})
	x := newHarness(t, root, false)
	from := x.vis("u")
	set.Set(2)
	x.frame()
	if v := x.vis("u"); !nearRect(v, from, 0.01) {
		t.Errorf("the new underline starts at %v, want the old one's %v", v, from)
	}
	to := x.laid("u")
	if near32(to.X0, from.X0, 1) {
		t.Fatal("the underline did not move")
	}
	x.runFor(50 * time.Millisecond)
	if v := x.vis("u"); !(v.X0 > from.X0 && v.X0 < to.X0+5) {
		t.Errorf("mid-way %v, from %v to %v", v, from, to)
	}
	x.settle()
	if x.vis("u") != to || !x.quiet() {
		t.Errorf("settled at %v, want %v", x.vis("u"), to)
	}
	// Without a clock (Render) or with reduced motion: at its place.
	y := newHarness(t, root, true)
	set.Set(1)
	y.frame()
	if v := y.vis("u"); v != y.laid("u") || !y.quiet() {
		t.Errorf("reduced motion: %v, laid %v", v, y.laid("u"))
	}
}

// TestLayoutIDFromExiting: the element that exits under a Presence hands
// its box to the new one.
func TestLayoutIDFromExiting(t *testing.T) {
	var set Setter[bool]
	root := Component(func() Node {
		big, s := UseState(false)
		set = s
		var thumb, detail Node
		if !big {
			thumb = div(Key("thumb"), Class("ml-10 mt-10 size-10 bg-amber-500 transition-opacity duration-200"), Exit("opacity-0"), LayoutID("photo"))
		} else {
			detail = div(Key("detail"), Class("ml-40 mt-4 w-40 h-30 bg-amber-500"), LayoutID("photo"))
		}
		return div(Class("relative"), Presence(thumb), detail)
	})
	x := newHarness(t, root, false)
	from := x.vis("thumb")
	set.Set(true)
	x.frame()
	if !keyed(x.tree(), "thumb").exiting() {
		t.Fatal("the thumbnail does not exit")
	}
	if v := x.vis("detail"); !nearRect(v, from, 0.05) {
		t.Errorf("detail starts at %v, want the thumbnail's %v", v, from)
	}
	x.settle()
	if keyed(x.tree(), "thumb") != nil || x.vis("detail") != x.laid("detail") || !x.quiet() {
		t.Error("not settled")
	}
}

// TestLayoutReducedMotion: no animation, at once.
func TestLayoutReducedMotion(t *testing.T) {
	var o orderProbe
	x := newHarness(t, o.root("a", "b"), true)
	o.set.Set([]string{"b", "a"})
	x.frame()
	if v := x.vis("a"); v != x.laid("a") || !x.quiet() {
		t.Errorf("reduced motion animates: %v vs %v", v, x.laid("a"))
	}
	// Render (no clock): the final layout.
	var o2 orderProbe
	r := newTestTree(t, 200, 200, o2.root("a", "b"), nil)
	r.Frame()
	o2.set.Set([]string{"b", "a"})
	r.ApplyUpdates()
	r.Frame()
	if la := keyed(r, "a").box.lanim; la == nil || la.active || la.running || r.Animating() {
		t.Errorf("a tree without a clock animates: %+v", la)
	}
}

// TestLayoutHitTest: during the animation the pointer hits the box where
// it is drawn.
func TestLayoutHitTest(t *testing.T) {
	var o orderProbe
	o.class = "duration-1000 ease-linear"
	x := newHarness(t, o.root("a", "b", "c"), false)
	o.set.Set([]string{"c", "b", "a"})
	x.frame()
	x.runFor(300 * time.Millisecond)
	v := x.vis("a")
	c := v.Center()
	if got := x.tree().HitTest(c.X, c.Y); got != keyed(x.tree(), "a") {
		t.Errorf("hit at a's drawn center %v: %v", c, got)
	}
	l := x.laid("a").Center()
	if got := x.tree().HitTest(l.X, l.Y); got == keyed(x.tree(), "a") {
		t.Errorf("a is hit at its layout box %v while drawn at %v", l, v)
	}
	// Bounds see the drawn box (getBoundingClientRect sees transforms).
	if b := (ElementRef{&elemRef{el: keyed(x.tree(), "a")}}).Bounds(); !near32(b.Y, v.Y0, 0.01) {
		t.Errorf("Bounds %v, drawn %v", b, v)
	}
}

func TestLayoutDamageMatchesFullRender(t *testing.T) {
	for _, scale := range []float64{1, 1.5, 2} {
		t.Run(fmt.Sprint(scale), func(t *testing.T) {
			var setOrder Setter[[]string]
			var setOpen Setter[bool]
			var setTab Setter[int]
			root := Component(func() Node {
				order, so := UseState([]string{"a", "b", "c", "d"})
				open, sp := UseState(false)
				tab, st := UseState(0)
				setOrder, setOpen, setTab = so, sp, st
				h := "h-12"
				if open {
					h = "h-32"
				}
				var tabs []Child
				tabs = append(tabs, Class("flex gap-3"))
				for i := range 3 {
					var u Node
					if i == tab {
						u = div(Class("h-1 rounded-full bg-indigo-500"), LayoutID("u"))
					}
					tabs = append(tabs, div(Class("w-16 h-8 flex flex-col justify-end"), Text(fmt.Sprint("Tab ", i)), u))
				}
				return div(Class("flex gap-4 p-3"),
					div(Class("flex flex-col gap-2 w-28"), Map(order, func(k string) Node {
						return div(Key(k), Class("h-8 rounded-lg bg-sky-500 shadow-md px-2 text-white"), AnimateLayout(), Text(k))
					})),
					div(Class("flex flex-col gap-3"),
						div(tabs...),
						div(Class("w-40 rounded-2xl bg-white shadow-lg border border-slate-200 "+h), AnimateLayout(),
							div(Class("m-2 h-6 rounded bg-slate-300"), AnimateLayout())),
						div(Class("w-40 h-8 rounded bg-amber-400"), AnimateLayout())))
			})
			x := newDamageHarness(t, root, 360, 260, scale, Config{}, 11)
			x.frame("start")
			setOrder.Set([]string{"d", "c", "b", "a"})
			x.b.Drain(x.h)
			for i := range 8 {
				x.frame(fmt.Sprint("reorder ", i))
			}
			setOpen.Set(true)
			setTab.Set(2)
			x.b.Drain(x.h)
			for i := range 8 {
				x.frame(fmt.Sprint("open ", i))
			}
			setOrder.Set([]string{"b", "a", "d", "c"}) // interrupts
			setOpen.Set(false)
			x.b.Drain(x.h)
			for i := range 40 {
				x.frame(fmt.Sprint("back ", i))
			}
			if x.tree().Animating() {
				t.Error("still animating")
			}
			if x.partial == 0 {
				t.Error("no frame was rasterized partially")
			}
			t.Logf("frames %d: full %d, partial %d, shifted %d, skipped %d", x.frames, x.full, x.partial, x.shifted, x.skipped)
		})
	}
}

// TestLayoutTransformDoesNotAnimate: a transform of an ancestor moves the
// box on screen, not in its layout.
func TestLayoutTransformDoesNotAnimate(t *testing.T) {
	root := div(Class("p-8"),
		div(Key("parent"), Class("w-40 p-4 bg-slate-200 hover:scale-110 hover:translate-x-4 transition duration-200"),
			div(Key("kid"), Class("h-10 bg-rose-500"), AnimateLayout())))
	x := newHarness(t, root, false)
	x.send(move(60, 50))
	x.frame()
	x.runFor(100 * time.Millisecond)
	if !keyed(x.tree(), "parent").box.layerSC {
		t.Fatal("the parent does not transition")
	}
	if la := keyed(x.tree(), "kid").box.lanim; la.running || la.active {
		t.Errorf("a transform of the parent starts a layout animation: %+v", la)
	}
	x.settle()
}

// TestLayoutWithPresence: an exiting row keeps its place; when it
// unmounts, the rows below slide up.
func TestLayoutWithPresence(t *testing.T) {
	var set Setter[[]string]
	root := Component(func() Node {
		items, s := UseState([]string{"a", "b", "c"})
		set = s
		return div(Class("flex flex-col gap-2"), Presence(Map(items, func(k string) Node {
			return div(Key(k), AnimateLayout(), Class("h-10 bg-sky-500 transition-opacity duration-200 ease-linear"), Exit("opacity-0"), Text(k))
		})))
	})
	x := newHarness(t, root, false)
	b0 := x.vis("b")
	set.Set([]string{"b", "c"})
	x.frame()
	x.runFor(100 * time.Millisecond)
	if !keyed(x.tree(), "a").exiting() {
		t.Fatal("a does not exit")
	}
	if v := x.vis("b"); v != b0 {
		t.Errorf("b moves while a exits: %v, was %v", v, b0)
	}
	x.runFor(120 * time.Millisecond) // the exit ends: a unmounts
	if keyed(x.tree(), "a") != nil {
		t.Fatal("a is still mounted")
	}
	if la := keyed(x.tree(), "b").box.lanim; !la.running {
		t.Error("b does not slide into a's place")
	}
	if v := x.vis("b"); !(v.Y0 < b0.Y0 && v.Y0 > x.laid("b").Y0) {
		t.Errorf("b at %v, from %v to %v", v.Y0, b0.Y0, x.laid("b").Y0)
	}
	x.settle()
	if x.vis("b") != x.laid("b") || !x.quiet() {
		t.Error("not settled")
	}
}

// still fails the test if a box with key k runs or shows a layout
// animation.
func (x *inputHarness) still(what string, keys ...string) {
	x.t.Helper()
	for _, k := range keys {
		if la := keyed(x.tree(), k).box.lanim; la == nil || la.running || la.active {
			x.t.Fatalf("%s: %s animates its layout: %+v", what, k, la)
		}
	}
}

// TestLayoutStickyDoesNotAnimate: a sticky box that sticks and unsticks
// while its container scrolls moves by its sticky offset (a paint
// offset), not in its layout.
func TestLayoutStickyDoesNotAnimate(t *testing.T) {
	kids := []Child{Key("sc"), Class("h-[150px] overflow-y-auto"),
		div(Key("r0"), Class("h-10 bg-sky-500"), AnimateLayout()),
		div(Key("r1"), Class("h-10 bg-sky-400"), AnimateLayout()),
		div(Key("head"), Class("sticky top-0 h-8 bg-white"), AnimateLayout(), Text("Header")),
	}
	for i := 2; i < 20; i++ {
		kids = append(kids, div(Key(fmt.Sprint("r", i)), Class("h-10 bg-sky-300"), AnimateLayout()))
	}
	x := newHarness(t, div(div(kids...)), false)
	laid := keyed(x.tree(), "head").box.u
	x.wheel(notch(50, 50, 3))
	for i := 0; i < 400 && x.w.FrameRequested(); i++ {
		x.frame()
		x.still(fmt.Sprint("scrolling, frame ", i), "head", "r0", "r5")
	}
	v := x.vis("head")
	if u := keyed(x.tree(), "head").box.u; !near32(v.Y0, 0, 0.01) || u != laid || u.Y0 != 80 {
		t.Fatalf("the header is drawn at %v (laid out at %v, was %v): not stuck to the top", v, u, laid)
	}
	x.wheel(notch(50, 50, -3))
	for i := 0; i < 400 && x.w.FrameRequested(); i++ {
		x.frame()
		x.still(fmt.Sprint("scrolling back, frame ", i), "head", "r0", "r5")
	}
	if v := x.vis("head"); !near32(v.Y0, 80, 0.01) {
		t.Errorf("unstuck header at %v", v)
	}
}

// TestLayoutFixedDoesNotAnimate: a fixed box does not move with the
// root's scroll, and does not animate either.
func TestLayoutFixedDoesNotAnimate(t *testing.T) {
	x := newHarness(t, div(Class("h-[1000px]"),
		div(Key("bar"), Class("fixed bottom-4 right-4 size-12 bg-rose-500"), AnimateLayout()),
		div(Key("row"), Class("mt-40 h-10 bg-sky-500"), AnimateLayout())), false)
	before := x.vis("bar")
	x.wheel(notch(50, 50, 3))
	for i := 0; i < 400 && x.w.FrameRequested(); i++ {
		x.frame()
		x.still("scrolling the root", "bar", "row")
	}
	if v := x.vis("bar"); v != before {
		t.Errorf("the fixed box moved: %v, was %v", v, before)
	}
}

// TestLayoutInPortal: the children of a Portal animate their layout like
// any others.
func TestLayoutInPortal(t *testing.T) {
	var set Setter[[]string]
	root := Component(func() Node {
		items, s := UseState([]string{"a", "b", "c"})
		set = s
		return div(Text("page"), Portal(div(Class("fixed right-4 top-4 flex w-32 flex-col gap-2"),
			Map(items, func(k string) Node {
				return div(Key(k), AnimateLayout(), Class("h-8 rounded bg-slate-800 text-white"), Text(k))
			}))))
	})
	x := newHarness(t, root, false)
	before := x.vis("a")
	set.Set([]string{"c", "b", "a"})
	x.frame()
	if v := x.vis("a"); !nearRect(v, before, 0.01) {
		t.Errorf("a in the portal starts at %v, want %v", v, before)
	}
	x.runFor(50 * time.Millisecond)
	if v := x.vis("a"); !(v.Y0 > before.Y0 && v.Y0 < x.laid("a").Y0) {
		t.Errorf("a mid-way at %v, from %v to %v", v.Y0, before.Y0, x.laid("a").Y0)
	}
	x.settle()
	if x.vis("a") != x.laid("a") || !x.quiet() {
		t.Error("not settled")
	}
}

// TestLayoutPropsOnPortal: the portal layer covers the window; the props
// belong on its content.
func TestLayoutPropsOnPortal(t *testing.T) {
	for _, p := range []Prop{AnimateLayout(), LayoutID("x")} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("Portal(%s) does not panic", p.name)
				}
			}()
			Portal(p, div())
		}()
	}
}

// TestLayoutWithColorFns: a BgFn on a box whose layout animates keeps
// following its Value, and a clip-path scales with the flip (it is in the
// box's own space, as under a CSS transform); incremental frames equal
// full ones.
func TestLayoutWithColorFns(t *testing.T) {
	var setOpen Setter[bool]
	var bg *Value[gfx.Color]
	root := Component(func() Node {
		open, s := UseState(false)
		setOpen = s
		bg = UseValue(gfx.RGBA8(14, 165, 233, 255))
		h := "h-12"
		if open {
			h = "h-32"
		}
		return div(Class("flex flex-col gap-3 p-4"),
			div(Key("card"), AnimateLayout(), Class("w-40 rounded-xl "+h), Style(style.BgFn(bg.Get))),
			div(Key("clip"), AnimateLayout(), Class("w-40 bg-violet-500 [clip-path:inset(4px_round_12px)] "+h)),
			div(Key("text"), AnimateLayout(), Class("w-40 h-8"), Style(style.TextColorFn(bg.Get)), Text("colored")))
	})
	x := newDamageHarness(t, root, 240, 320, 1.5, Config{}, 5)
	setOpen.Set(true)
	x.b.Drain(x.h)
	for i := range 4 {
		x.frame(fmt.Sprint("open ", i))
	}
	bg.Set(gfx.RGBA8(244, 63, 94, 255))
	x.frame("recolor")
	card := keyed(x.tree(), "card").box
	if !layoutActive(card) || card.pcs.BackgroundColor != gfx.RGBA8(244, 63, 94, 255) {
		t.Errorf("the BgFn of an animating box: active %v, color %v", layoutActive(card), card.pcs.BackgroundColor)
	}
	for i := range 30 {
		x.frame(fmt.Sprint("recolored ", i))
	}
	if x.tree().Animating() {
		t.Error("still animating")
	}
}
