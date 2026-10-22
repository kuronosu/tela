package core

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
)

// settleFrames runs frames until the tree has no work, at most 8, and
// returns how many ran.
func settleFrames(t *testing.T, tr *Tree) int {
	t.Helper()
	n := 0
	for tr.Dirty() {
		if n == 8 {
			t.Fatal("the tree does not settle")
		}
		tr.Frame()
		n++
	}
	return n
}

func wantBounds(t *testing.T, what string, got Rect, x, y, w, h float32) {
	t.Helper()
	if !near(got.X, x, 0.01) || !near(got.Y, y, 0.01) || !near(got.Width, w, 0.01) || !near(got.Height, h, 0.01) {
		t.Errorf("%s: bounds (%g,%g %gx%g), want (%g,%g %gx%g)", what, got.X, got.Y, got.Width, got.Height, x, y, w, h)
	}
}

func TestElementRefBounds(t *testing.T) {
	var ref ElementRef
	var before Rect
	comp := func() Node {
		ref = UseElementRef()
		before = ref.Bounds() // not mounted yet
		return div(Class("p-4"),
			div(Key("plain"), AttachRef(ref), Class("ml-6 mt-2 h-10 w-20 bg-sky-500")))
	}
	tr := newTestTree(t, 300, 200, Component(comp), nil)
	if before != (Rect{}) {
		t.Errorf("bounds before mounting: %+v", before)
	}
	tr.Frame()
	// Valid after layout, and the layout rect of an element nothing moves.
	wantBounds(t, "plain", ref.Bounds(), 16+24, 16+8, 80, 40)
	if got, want := ref.Bounds(), rectOf(mustFind(t, tr, "plain").Rect); got != want {
		t.Errorf("bounds %+v, layout rect %+v", got, want)
	}
	if (ElementRef{}).Bounds() != (Rect{}) {
		t.Error("the zero ElementRef has bounds")
	}
	if got := ref.Bounds(); got.Right() != 120 || got.Bottom() != 64 || !got.Contains(40, 24) || got.Contains(120, 24) {
		t.Errorf("Right, Bottom, Contains of %+v", got)
	}
}

func TestBoundsIncludeTransforms(t *testing.T) {
	var scaled, moved, rotated, nested ElementRef
	comp := func() Node {
		scaled, moved, rotated, nested = UseElementRef(), UseElementRef(), UseElementRef(), UseElementRef()
		return div(Class("p-4"),
			// 80x40 at (16, 16), scaled about its center by 1/2.
			div(AttachRef(scaled), Class("h-10 w-20 scale-50 bg-sky-500")),
			// 80x40 at (16, 56), moved by (10, -5).
			div(AttachRef(moved), Class("h-10 w-20 bg-sky-500"), Style(style.Translate(10, -5))),
			// 40x40 at (16, 96), rotated by 45 degrees about its center.
			div(AttachRef(rotated), Class("size-10 rotate-45 bg-sky-500")),
			// The child of a moved and scaled box.
			div(Class("translate-x-4 scale-50"), div(AttachRef(nested), Class("h-10 w-20 bg-sky-500"))),
		)
	}
	tr := newTestTree(t, 300, 300, Component(comp), nil)
	tr.Frame()
	wantBounds(t, "scaled", scaled.Bounds(), 16+20, 16+10, 40, 20)
	wantBounds(t, "moved", moved.Bounds(), 16+10, 56-5, 80, 40)
	half := float32(40 * 0.70710678)
	side := 2 * half // the bounding box of a square turned 45 degrees: 40 * sqrt 2
	wantBounds(t, "rotated", rotated.Bounds(), 36-side/2, 116-side/2, side, side)
	// The parent (268x40 at (16, 136), center (150, 156)) moves 16 px and
	// scales by 1/2 about its center: the child, 80x40 at its left edge,
	// is 40x20 from x = 150 + (-134 / 2 + 16).
	wantBounds(t, "nested", nested.Bounds(), 99, 146, 40, 20)
}

func TestBoundsInsideScrolledContent(t *testing.T) {
	var list, row ElementRef
	comp := func() Node {
		list, row = UseElementRef(), UseElementRef()
		return div(Class("p-0"),
			div(AttachRef(list), Class("h-[100px] w-[200px] overflow-y-auto"),
				div(Class("h-[100px]")), div(Class("h-[100px]")),
				div(AttachRef(row), Class("h-[30px] w-[50px] bg-sky-500")),
				div(Class("h-[300px]"))))
	}
	tr := newTestTree(t, 300, 300, Component(comp), nil)
	tr.Frame()
	wantBounds(t, "unscrolled", row.Bounds(), 0, 200, 50, 30)
	list.ScrollTo(0, 150)
	tr.Frame()
	wantBounds(t, "scrolled", row.Bounds(), 0, 50, 50, 30)
}

func TestBoundsOfInlineAndHiddenElements(t *testing.T) {
	var inl, none ElementRef
	comp := func() Node {
		inl, none = UseElementRef(), UseElementRef()
		return div(Class("w-[300px]"),
			p(Text("Some "), span(AttachRef(inl), Class("px-2"), Text("inline")), Text(" text")),
			div(AttachRef(none), Class("hidden h-10 w-10")))
	}
	tr := newTestTree(t, 300, 200, Component(comp), nil)
	tr.Frame()
	b := inl.Bounds()
	if b.Width < 30 || b.Height < 10 || b.X < 10 {
		t.Errorf("inline bounds %+v", b)
	}
	if none.Bounds() != (Rect{}) {
		t.Errorf("display: none bounds %+v", none.Bounds())
	}
}

// boundsHarness is a component that shows its bounds.
type boundsHarness struct {
	t      *testing.T
	tr     *Tree
	ref    ElementRef
	bounds *Value[Rect]
	builds int
	// seen are the bounds the component read in each build.
	seen []Rect
}

func newBoundsHarness(t *testing.T, w, h float32, classes string, extra ...Child) *boundsHarness {
	x := &boundsHarness{t: t}
	comp := func() Node {
		x.builds++
		x.ref = UseElementRef()
		x.bounds = UseBounds(x.ref)
		x.seen = append(x.seen, x.bounds.Get())
		c := append([]Child{AttachRef(x.ref), Key("box"), Class(classes)}, extra...)
		return div(Class("p-4"), div(c...))
	}
	x.tr = newTestTree(t, w, h, Component(comp), nil)
	return x
}

func TestUseBoundsFollowsLayout(t *testing.T) {
	x := newBoundsHarness(t, 300, 200, "h-10 w-1/2 bg-sky-500")
	if x.bounds.Get() != (Rect{}) {
		t.Errorf("before the first frame: %+v", x.bounds.Get())
	}
	n := settleFrames(t, x.tr)
	wantBounds(t, "first", x.bounds.Get(), 16, 16, 134, 40) // (300 - 32) / 2
	if x.builds != 2 || n != 2 {
		t.Errorf("builds %d, frames %d: want the first build and one rebuild for the measured bounds", x.builds, n)
	}
	if len(x.seen) != 2 || x.seen[0] != (Rect{}) {
		t.Errorf("the component read %+v", x.seen)
	}
	// Resizing moves them again; settled, no more frames run.
	x.tr.SetViewport(gfx.Size{W: 200, H: 200})
	settleTree(t, x.tr)
	wantBounds(t, "resized", x.bounds.Get(), 16, 16, 84, 40)
	s0 := x.tr.Stats()
	if _, changed := x.tr.Frame(); changed || x.tr.Dirty() || work(s0, x.tr.Stats()) != (Stats{}) {
		t.Errorf("a settled tree ran work: %+v", work(s0, x.tr.Stats()))
	}
}

func TestUseBoundsUnchangedRequestsNoFrames(t *testing.T) {
	x := newBoundsHarness(t, 300, 200, "h-10 w-20 bg-sky-500")
	settleTree(t, x.tr)
	builds := x.builds
	// An unrelated change: a frame runs, the bounds are the same.
	x.tr.SetViewport(gfx.Size{W: 310, H: 200})
	x.tr.Frame()
	if x.tr.Dirty() {
		t.Error("equal bounds requested another frame")
	}
	if x.builds != builds {
		t.Errorf("equal bounds rebuilt the component (%d builds, %d before)", x.builds, builds)
	}
}

func TestUseBoundsFollowsTransforms(t *testing.T) {
	var pos *Value[float32]
	var b *Value[Rect]
	comp := func() Node {
		ref := UseElementRef()
		b = UseBounds(ref)
		pos = UseValue[float32](0)
		return div(Class("p-4"), div(AttachRef(ref), Class("h-10 w-20 bg-sky-500"),
			Style(style.TranslateFn(func() (float32, float32) { return pos.Get(), 0 }))))
	}
	tr := newTestTree(t, 300, 200, Component(comp), nil)
	settleTree(t, tr)
	wantBounds(t, "at rest", b.Get(), 16, 16, 80, 40)
	pos.Set(25)
	settleTree(t, tr)
	wantBounds(t, "moved", b.Get(), 41, 16, 80, 40)
}

func TestUseBoundsFollowsScrolling(t *testing.T) {
	var list ElementRef
	var b *Value[Rect]
	comp := func() Node {
		list = UseElementRef()
		row := UseElementRef()
		b = UseBounds(row)
		return div(AttachRef(list), Class("h-[100px] w-[200px] overflow-y-auto"),
			div(Class("h-[100px]")),
			div(AttachRef(row), Class("h-[30px] w-[50px] bg-sky-500")),
			div(Class("h-[300px]")))
	}
	tr := newTestTree(t, 300, 300, Component(comp), nil)
	settleTree(t, tr)
	wantBounds(t, "unscrolled", b.Get(), 0, 100, 50, 30)
	list.ScrollTo(0, 60)
	settleTree(t, tr)
	wantBounds(t, "scrolled", b.Get(), 0, 40, 50, 30)
}

func TestUseBoundsInDeferredPropsBuildsNothing(t *testing.T) {
	builds := 0
	var b *Value[Rect]
	comp := func() Node {
		builds++
		ref := UseElementRef()
		b = UseBounds(ref)
		return div(Class("p-4"),
			div(AttachRef(ref), Class("h-10 w-1/2 bg-slate-200"),
				div(Key("fill"), Class("h-10 bg-sky-500"), Style(style.WidthFn(func() float32 { return b.Get().Width / 2 })))))
	}
	tr := newTestTree(t, 300, 200, Component(comp), nil)
	settleTree(t, tr)
	wantRect(t, mustFind(t, tr, "fill"), 16, 16, 67, 40)
	builds0 := builds
	tr.SetViewport(gfx.Size{W: 200, H: 200})
	settleTree(t, tr)
	wantRect(t, mustFind(t, tr, "fill"), 16, 16, 42, 40)
	if builds != builds0 {
		t.Errorf("a deferred read of the bounds built the component %d times", builds-builds0)
	}
}

func TestUseBoundsLifecycle(t *testing.T) {
	// A ref that is not attached any more reads zero bounds; unmounting
	// the component removes its slot.
	show := true
	var set func(bool)
	var b *Value[Rect]
	child := func() Node {
		ref := UseElementRef()
		b = UseBounds(ref)
		return div(AttachRef(ref), Class("h-10 w-10 bg-sky-500"))
	}
	comp := func() Node {
		s, st := UseStateSet(true)
		set, show = st, s
		if !show {
			return div()
		}
		return div(Component(child))
	}
	tr := newTestTree(t, 100, 100, Component(comp), nil)
	settleTree(t, tr)
	wantBounds(t, "attached", b.Get(), 0, 0, 40, 40)
	if len(tr.bnd.follows) != 1 {
		t.Fatalf("%d slots", len(tr.bnd.follows))
	}
	set(false)
	settleTree(t, tr)
	if len(tr.bnd.follows) != 0 {
		t.Errorf("%d slots after unmounting", len(tr.bnd.follows))
	}
}

func TestUseBoundsHookKindAndZeroRef(t *testing.T) {
	bounds := true
	comp := func() Node {
		r := UseElementRef()
		if bounds {
			UseBounds(r)
		} else {
			UseValue(Rect{})
		}
		return div(AttachRef(r))
	}
	tr := devTree(t, Component(comp), nil)
	bounds = false
	msg := panicMessage(func() { tr.Mount(Component(comp)) })
	if !strings.Contains(msg, "hook 2 is UseValue") || !strings.Contains(msg, "but it was UseBounds") {
		t.Errorf("panic %q", msg)
	}
	msg = panicMessage(func() { UseBounds(ElementRef{}) })
	if !strings.Contains(msg, "UseBounds(zero ElementRef)") {
		t.Errorf("zero ref: %q", msg)
	}
}

func TestInspectBounds(t *testing.T) {
	tr := newTestTree(t, 200, 200, div(
		div(Key("plain"), Class("h-10 w-10 bg-sky-500")),
		div(Key("scaled"), Class("h-10 w-20 scale-50 bg-sky-500"), div(Key("inner"), Class("h-4 w-4"))),
		div(Key("scroller"), Class("h-[50px] overflow-y-auto"), div(Class("h-[80px]")), div(Key("below"), Class("h-4"))),
	), nil)
	tr.Frame()
	if n := mustFind(t, tr, "plain"); n.Bounds != nil {
		t.Errorf("an unmoved box has bounds %v", *n.Bounds)
	}
	n := mustFind(t, tr, "scaled")
	if n.Bounds == nil {
		t.Fatal("a scaled box has no bounds")
	}
	if got, want := rectOf(*n.Bounds), (Rect{20, 50, 40, 20}); got != want {
		t.Errorf("scaled bounds %+v, want %+v", got, want)
	}
	if n.Rect != (gfx.Rect{X0: 0, Y0: 40, X1: 80, Y1: 80}) {
		t.Errorf("the layout rect moved: %+v", n.Rect)
	}
	if in := mustFind(t, tr, "inner"); in.Bounds == nil {
		t.Error("the child of a scaled box has no bounds")
	}
	js, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(js), `"bounds":{"x":20,"y":50,"width":40,"height":20}`) {
		t.Errorf("JSON %s", js)
	}
	if strings.Contains(string(mustJSON(t, mustFind(t, tr, "plain"))), "bounds") {
		t.Error("an unmoved box serializes bounds")
	}
}

func TestInspectBoundsInScrolledContent(t *testing.T) {
	var list ElementRef
	comp := func() Node {
		list = UseElementRef()
		return div(AttachRef(list), Key("scroller"), Class("h-[50px] w-[100px] overflow-y-auto"),
			div(Class("h-[80px]")), div(Key("below"), Class("h-4 w-4")), div(Class("h-[80px]")))
	}
	tr := newTestTree(t, 200, 200, Component(comp), nil)
	tr.Frame()
	if n := mustFind(t, tr, "below"); n.Bounds != nil {
		t.Errorf("unscrolled: bounds %v", *n.Bounds)
	}
	list.ScrollTo(0, 30)
	tr.Frame()
	n := mustFind(t, tr, "below")
	if n.Bounds == nil {
		t.Fatal("a scrolled box has no bounds")
	}
	// The layout rect does not scroll; the bounds do.
	if want := (gfx.Rect{X0: 0, Y0: 80, X1: 16, Y1: 96}); n.Rect != want {
		t.Errorf("rect %+v, want %+v", n.Rect, want)
	}
	if want := (gfx.Rect{X0: 0, Y0: 50, X1: 16, Y1: 66}); *n.Bounds != want {
		t.Errorf("bounds %+v, want %+v", *n.Bounds, want)
	}
}

func mustJSON(t *testing.T, n *InspectNode) []byte {
	t.Helper()
	js, err := json.Marshal(n)
	if err != nil {
		t.Fatal(err)
	}
	return js
}

// A NewElementRef belongs to no hook slot: one per item of a list whose
// length changes, attached, focused and measured like a hook's.
func TestNewElementRef(t *testing.T) {
	refs := map[string]ElementRef{}
	var setN Setter[int]
	comp := func() Node {
		n, set := UseState(2)
		setN = set
		var kids []Child
		for _, k := range []string{"a", "b"}[:n] {
			r, ok := refs[k]
			if !ok {
				r = NewElementRef()
				refs[k] = r
			}
			kids = append(kids, div(Key(k), AttachRef(r), TabIndex(-1), Class("h-10 w-20")))
		}
		return div(kids...)
	}
	tr := newTestTree(t, 300, 200, Component(comp), nil)
	tr.Frame()
	wantBounds(t, "b", refs["b"].Bounds(), 0, 40, 80, 40)
	refs["b"].Focus()
	if !refs["b"].Focused() || refs["a"].Focused() {
		t.Error("the focus did not go to b")
	}
	// The ref of an item that unmounts is detached, and works again when
	// an element with it mounts.
	setN.Set(1)
	settleFrames(t, tr)
	if refs["b"].Bounds() != (Rect{}) || refs["b"].Focused() {
		t.Error("the ref of an unmounted element still follows it")
	}
	setN.Set(2)
	settleFrames(t, tr)
	wantBounds(t, "b again", refs["b"].Bounds(), 0, 40, 80, 40)
}
