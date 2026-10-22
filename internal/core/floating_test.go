package core

import (
	"image/color"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/semantics"
	"github.com/kuronosu/tela/internal/style"
)

// below places the floating element under its anchor's left edge, gap
// pixels down, and counts the calls.
func below(gap float32, calls *int) func(FloatInput) FloatResult {
	return func(in FloatInput) FloatResult {
		*calls++
		return FloatResult{X: in.Anchor.X, Y: in.Anchor.Bottom() + gap}
	}
}

// popRect returns the window rectangle of the box with key k as painted.
func popRect(tr *Tree, k string) gfx.Rect { return tr.boxWindowRect(keyed(tr, k).box) }

func rectNear(a, b gfx.Rect) bool {
	return near(a.X0, b.X0, 1e-3) && near(a.Y0, b.Y0, 1e-3) && near(a.X1, b.X1, 1e-3) && near(a.Y1, b.Y1, 1e-3)
}

// semByName returns the first semantics node named name.
func semByName(root *semantics.Node, name string) *semantics.Node {
	var found *semantics.Node
	root.Walk(func(n *semantics.Node) bool {
		if found == nil && n.Name == name && n.Kind == semantics.KindElement {
			found = n
		}
		return found == nil
	})
	return found
}

// TestFloatingFirstFrame: a popover in a Portal is painted, hit and
// bounded at its place in the first frame that shows it.
func TestFloatingFirstFrame(t *testing.T) {
	calls := 0
	var anchor, pop ElementRef
	comp := func() Node {
		anchor, pop = UseElementRef(), UseElementRef()
		return div(Class("p-10"),
			div(Key("anchor"), AttachRef(anchor), Class("ml-[30px] h-8 w-20"), Text("Open")),
			Portal(div(Key("pop"), AttachRef(pop), Floating(FloatOptions{Anchor: anchor, Place: below(4, &calls)}),
				Class("fixed h-20 w-40 bg-rose-500"),
				button(Key("item"), Class("block h-6 w-full"), Text("Item")))))
	}
	tr := newTestTree(t, 400, 300, Component(comp), nil)
	dl, _ := tr.Frame()
	// The anchor: x 40 + 30, y 40, 80x32; the popover 4 px below it.
	want := gfx.Rect{X0: 70, Y0: 76, X1: 230, Y1: 156}
	if g := popRect(tr, "pop"); !rectNear(g, want) {
		t.Errorf("first frame: popover at %v, want %v", g, want)
	}
	if calls != 1 {
		t.Errorf("Place ran %d times in the first frame, want 1", calls)
	}
	if b := pop.Bounds(); !rectNear(gfx.RectFromOriginSize(gfx.Point{X: b.X, Y: b.Y}, gfx.Size{W: b.Width, H: b.Height}), want) {
		t.Errorf("ElementRef.Bounds %v, want %v", b, want)
	}
	if g := hitName(tr, 100, 80); g != "item" {
		t.Errorf("hit inside the popover: %s, want item", g)
	}
	if g := hitName(tr, 100, 150); g != "pop" {
		t.Errorf("hit in the popover below its item: %s, want pop", g)
	}
	if g := hitName(tr, 10, 10); g == "pop" || g == "item" {
		t.Errorf("hit at the popover's laid-out place (0, 0): %s", g)
	}
	if n := semByName(tr.Semantics(), "Item"); n == nil || !rectNear(n.Bounds, gfx.Rect{X0: 70, Y0: 76, X1: 230, Y1: 100}) {
		t.Errorf("semantics bounds of the item: %+v", n)
	}
	img := tr.Rasterize(dl, 1)
	rose := color.RGBA{0xff, 0x20, 0x56, 0xff} // bg-rose-500, approximately
	if c := img.RGBAAt(120, 150); !nearRGBA(c, rose) {
		t.Errorf("pixel inside the popover %v, want rose", c)
	}
	if c := img.RGBAAt(5, 5); nearRGBA(c, rose) {
		t.Errorf("the popover is painted at its laid-out place too")
	}
	// Nothing changes: no further frame, no call.
	if tr.Dirty() {
		t.Errorf("the tree is dirty after the first frame")
	}
	if _, changed := tr.Frame(); changed || calls != 1 {
		t.Errorf("an idle frame changed %v, %d calls", changed, calls)
	}
}

func nearRGBA(a, b color.RGBA) bool {
	d := func(x, y uint8) int {
		if x > y {
			return int(x - y)
		}
		return int(y - x)
	}
	return d(a.R, b.R) < 40 && d(a.G, b.G) < 40 && d(a.B, b.B) < 40 && d(a.A, b.A) < 40
}

// TestFloatingFollowsScroll: scrolling the anchor's container moves the
// popover in frames that only repaint (no build, style or layout), and
// hit-testing and bounds follow.
func TestFloatingFollowsScroll(t *testing.T) {
	calls := 0
	var anchor, list ElementRef
	comp := func() Node {
		anchor, list = UseElementRef(), UseElementRef()
		return div(
			div(Key("list"), AttachRef(list), Class("h-[200px] overflow-y-auto"),
				div(Class("h-[100px]")),
				div(Key("anchor"), AttachRef(anchor), Class("h-8 w-20"), Text("Open")),
				div(Class("h-[600px]"))),
			Portal(div(Key("pop"), Floating(FloatOptions{Anchor: anchor, Place: below(0, &calls)}), Class("fixed h-10 w-30 bg-white"))))
	}
	tr := newTestTree(t, 400, 300, Component(comp), nil)
	tr.Frame()
	if g := popRect(tr, "pop"); !near(g.Y0, 132, 1e-3) {
		t.Fatalf("popover at %v, want y 132", g)
	}
	before := tr.Stats()
	for i, y := range []float32{30, 75, 76, 0} {
		list.ScrollTo(0, y)
		tr.Frame()
		s := tr.Stats()
		if s.Builds != before.Builds || s.Styles != before.Styles || s.Layouts != before.Layouts {
			t.Errorf("scroll %g: the frame built, restyled or laid out: %+v -> %+v", y, before, s)
		}
		want := 132 - y
		if g := popRect(tr, "pop"); !near(g.Y0, want, 1e-3) {
			t.Errorf("scroll %g (%d): popover at y %g, want %g", y, i, g.Y0, want)
		}
		if g := hitName(tr, 5, want+5); g != "pop" {
			t.Errorf("scroll %g: hit %s, want pop", y, g)
		}
	}
	if calls != 5 {
		t.Errorf("Place ran %d times, want 5 (one per frame that moved the anchor)", calls)
	}
	if tr.Dirty() {
		t.Error("dirty after the scroll frames")
	}
}

// TestFloatingFollowsTranslateFn: an anchor moved by a paint-phase
// TranslateFn (a drag) carries the popover in the same frames.
func TestFloatingFollowsTranslateFn(t *testing.T) {
	calls := 0
	var anchor ElementRef
	var x *Value[float32]
	comp := func() Node {
		anchor = UseElementRef()
		x = UseValue[float32](0)
		return div(
			div(Key("anchor"), AttachRef(anchor), Class("relative h-8 w-20"),
				Style(style.TranslateFn(func() (float32, float32) { return x.Get(), x.Get() / 2 }))),
			Portal(div(Key("pop"), Floating(FloatOptions{Anchor: anchor, Place: below(0, &calls)}), Class("fixed h-10 w-30"))))
	}
	tr := newTestTree(t, 400, 300, Component(comp), nil)
	tr.Frame()
	before := tr.Stats()
	x.Set(40)
	tr.Frame()
	if g := popRect(tr, "pop"); !near(g.X0, 40, 1e-3) || !near(g.Y0, 52, 1e-3) {
		t.Errorf("popover at %v, want (40, 52)", g)
	}
	if s := tr.Stats(); s.Builds != before.Builds || s.Layouts != before.Layouts {
		t.Errorf("the frame built or laid out")
	}
}

// TestFloatingSizeLimit: a size limit (the size middleware) lays the
// popover out again in the same frame, and again when scrolling changes
// the room below the anchor, without building.
func TestFloatingSizeLimit(t *testing.T) {
	calls := 0
	var anchor, list, pop ElementRef
	var size *Value[Size]
	place := func(in FloatInput) FloatResult {
		calls++
		y := in.Anchor.Bottom()
		return FloatResult{X: in.Anchor.X, Y: y, MaxHeight: in.Clip.Bottom() - y - 8, HasMaxHeight: true}
	}
	comp := func() Node {
		anchor, list, pop = UseElementRef(), UseElementRef(), UseElementRef()
		size = UseSize(pop)
		return div(
			div(Key("list"), AttachRef(list), Class("h-[300px] overflow-y-auto"),
				div(Class("h-[40px]")),
				div(Key("anchor"), AttachRef(anchor), Class("h-8 w-20"), Text("Open")),
				div(Class("h-[600px]"))),
			Portal(div(Key("pop"), AttachRef(pop), Floating(FloatOptions{Anchor: anchor, Place: place}),
				Class("fixed max-h-[400px] w-40 overflow-y-auto"),
				div(Key("content"), Class("h-[500px]")))))
	}
	tr := newTestTree(t, 400, 300, Component(comp), nil)
	tr.Frame()
	// The anchor's bottom is at 72: 300 - 72 - 8 = 220.
	if g := popRect(tr, "pop"); !near(g.Height(), 220, 1e-3) || !near(g.Y0, 72, 1e-3) {
		t.Errorf("first frame: popover %v, want height 220 at y 72", g)
	}
	if s := tr.Stats(); s.Frames != 1 {
		t.Errorf("%d frames", s.Frames)
	}
	if got := size.Get(); got.Height != 220 || got.Width != 160 {
		t.Errorf("UseSize of the limited popover: %v", got)
	}
	if s := scrollerOf(keyed(tr, "pop")); s == nil || s.max.Y != 280 {
		t.Errorf("the popover's scroll extents follow its limit in the frame: %+v", s)
	}
	before := tr.Stats()
	list.ScrollTo(0, 30) // the anchor rises: more room below
	tr.Frame()
	if g := popRect(tr, "pop"); !near(g.Height(), 250, 1e-3) || !near(g.Y0, 42, 1e-3) {
		t.Errorf("after scrolling: popover %v, want height 250 at y 42", g)
	}
	s := tr.Stats()
	if s.Builds != before.Builds || s.Styles != before.Styles {
		t.Errorf("the scroll frame built or restyled")
	}
	if s.Layouts == before.Layouts {
		t.Errorf("the new limit did not lay out again")
	}
	// The limit never exceeds the element's own max-height.
	list.ScrollTo(0, 0)
	tr.Frame()
	if g := popRect(tr, "pop"); !near(g.Height(), 220, 1e-3) {
		t.Errorf("popover %v, want height 220 again", g)
	}
}

// TestFloatingLimitsAfterSpentBudget: a size limit that still changes
// after the relayouts a frame allows is not dropped: the frame after
// calls Place again and applies it, until the limits settle; then
// nothing runs. A limit that never settles stops asking for frames.
func TestFloatingLimitsAfterSpentBudget(t *testing.T) {
	var anchor ElementRef
	var tick *Value[float32]
	calls := 0
	const floor = 50
	// Each call shrinks the element by 10 px, down to floor: it settles
	// only after several layouts.
	place := func(in FloatInput) FloatResult {
		calls++
		return FloatResult{X: in.Anchor.X, Y: in.Anchor.Bottom(), MaxHeight: max(in.Floating.Height-10, floor), HasMaxHeight: true}
	}
	comp := func() Node {
		anchor = UseElementRef()
		tick = UseValue[float32](0)
		return div(
			div(Key("anchor"), AttachRef(anchor), Class("h-8 w-20"), Style(style.TranslateFn(func() (float32, float32) { return tick.Get(), 0 })), Text("Open")),
			Portal(div(Key("pop"), Floating(FloatOptions{Anchor: anchor, Place: func(in FloatInput) FloatResult { return place(in) }}),
				Class("fixed h-[100px] w-40"))))
	}
	tr := newTestTree(t, 400, 300, Component(comp), nil)
	tr.Frame()
	if g := popRect(tr, "pop").Height(); g >= 100 {
		t.Fatalf("first frame: height %g, want a limit", g)
	}
	for i := 0; i < 10 && tr.Dirty(); i++ {
		tr.Frame()
	}
	if g := popRect(tr, "pop").Height(); !near(g, 50, 1e-3) {
		t.Errorf("after the follow-up frames: height %g, want 50", g)
	}
	if tr.Dirty() {
		t.Errorf("settled limits still ask for frames")
	}
	// A frame for something else (the anchor moves) keeps them.
	tick.storeUI(1)
	tr.Frame()
	if g := popRect(tr, "pop").Height(); !near(g, 50, 1e-3) {
		t.Errorf("after an unrelated frame: height %g, want 50", g)
	}
	// A limit that never settles (it keeps shrinking to 0 and then
	// growing back) does not spin.
	grow := false
	place2 := func(in FloatInput) FloatResult {
		calls++
		grow = !grow
		h := float32(20)
		if grow {
			h = 40
		}
		return FloatResult{X: in.Anchor.X, Y: in.Anchor.Bottom(), MaxHeight: h, HasMaxHeight: true}
	}
	place = place2
	tick.storeUI(2)
	frames := 0
	for ; frames < 50 && tr.Dirty(); frames++ {
		tr.Frame()
	}
	if frames >= 50 {
		t.Errorf("limits that never settle keep requesting frames")
	}
}

// TestFloatingRebuildPlaces: a build of the component that renders a
// floating element gives it a new Place closure, whose captured options
// core cannot compare (functions are not comparable, and the same code
// with other captured values places elsewhere): Place runs again in that
// frame, so a changed option (here the gap) moves the element at once. A
// build elsewhere that does not render it calls nothing.
func TestFloatingRebuildPlaces(t *testing.T) {
	calls := 0
	var setGap, setOther Setter[float32]
	other := func() Node {
		n, s := UseState[float32](0)
		setOther = s
		return div(Key("other"), Textf("%g", n))
	}
	comp := func() Node {
		anchor := UseElementRef()
		gap, s := UseState[float32](4)
		setGap = s
		return div(Class("p-10"),
			div(Key("anchor"), AttachRef(anchor), Class("h-8 w-20"), Text("Open")),
			Portal(div(Key("pop"), Floating(FloatOptions{Anchor: anchor, Place: below(gap, &calls)}), Class("fixed h-20 w-40"))))
	}
	tr := newTestTree(t, 400, 300, div(Component(comp), Component(other)), nil)
	tr.Frame()
	if g := popRect(tr, "pop").Y0; !near(g, 76, 1e-3) || calls != 1 {
		t.Fatalf("first frame: y %g, %d calls", g, calls)
	}
	setGap.Set(12)
	tr.Frame()
	if g := popRect(tr, "pop").Y0; !near(g, 84, 1e-3) || calls != 2 {
		t.Errorf("a build with a new gap: y %g (want 84), %d calls (want 2)", g, calls)
	}
	setOther.Set(1)
	tr.Frame()
	if calls != 2 {
		t.Errorf("a build that does not render the floating element called Place: %d calls", calls)
	}
}

// TestFloatingPublish: Place publishes the arrow's offset (read by its
// TranslateFn in the same frame) and the final side (read by a build in
// the next frame).
func TestFloatingPublish(t *testing.T) {
	var anchor, arrow ElementRef
	var arrowX *Value[float32]
	var side *Value[string]
	builds := 0
	place := func(in FloatInput) FloatResult {
		x := in.Anchor.X + 40
		PublishValue(arrowX, in.Anchor.X+in.Anchor.Width/2-x-in.Arrow.Width/2)
		PublishValue(side, "bottom")
		return FloatResult{X: x, Y: in.Anchor.Bottom()}
	}
	label := func() Node {
		builds++
		return span(Key("side"), Text(side.Get()))
	}
	comp := func() Node {
		anchor, arrow = UseElementRef(), UseElementRef()
		arrowX = UseValue[float32](0)
		side = UseValue("top")
		return div(Class("pl-[100px]"),
			div(Key("anchor"), AttachRef(anchor), Class("h-8 w-20")),
			Portal(div(Key("pop"), Floating(FloatOptions{Anchor: anchor, Arrow: arrow, Place: place}), Class("fixed h-10 w-40"),
				div(Key("arrow"), AttachRef(arrow), Class("absolute top-0 left-0 size-2"),
					Style(style.TranslateFn(func() (float32, float32) { return arrowX.Get(), 0 }))),
				Component(label))))
	}
	tr := newTestTree(t, 400, 300, Component(comp), nil)
	tr.Frame()
	// The popover at x 140; the arrow centered under the anchor's center
	// (140): 140 - 140 - 4 = -4 from the popover's left edge.
	if g := popRect(tr, "arrow"); !near(g.X0, 136, 1e-3) {
		t.Errorf("arrow at %v in the first frame, want x 136", g)
	}
	if side.Get() != "bottom" {
		t.Errorf("published side %q", side.Get())
	}
	if !tr.Dirty() {
		t.Fatal("the build reader of the published side waits for no frame")
	}
	b := builds
	tr.Frame()
	if builds != b+1 {
		t.Errorf("the reader of the side built %d times, want 1", builds-b)
	}
	if tr.Dirty() {
		t.Error("published values keep the tree dirty")
	}
}

// TestFloatingWithoutAnchor: when the anchor goes, the popover stays where
// it was; one placed for the first time without an anchor goes where the
// result says.
func TestFloatingWithoutAnchor(t *testing.T) {
	var anchor ElementRef
	var set Setter[bool]
	var anchored []bool
	place := func(in FloatInput) FloatResult {
		anchored = append(anchored, in.Anchored)
		return FloatResult{X: in.Anchor.X + 10, Y: in.Anchor.Y + 20}
	}
	comp := func() Node {
		anchor = UseElementRef()
		show, s := UseState(true)
		set = s
		return div(Class("p-8"),
			If(show, div(Key("anchor"), AttachRef(anchor), Class("h-8 w-20"))),
			Portal(div(Key("pop"), Floating(FloatOptions{Anchor: anchor, Place: place}), Class("fixed size-10"))))
	}
	tr := newTestTree(t, 400, 300, Component(comp), nil)
	tr.Frame()
	if g := popRect(tr, "pop"); !near(g.X0, 42, 1e-3) || !near(g.Y0, 52, 1e-3) {
		t.Fatalf("popover at %v", g)
	}
	set.Set(false)
	tr.Frame()
	if g := popRect(tr, "pop"); !near(g.X0, 42, 1e-3) || !near(g.Y0, 52, 1e-3) {
		t.Errorf("without its anchor the popover moved to %v", g)
	}
	if len(anchored) != 2 || anchored[1] {
		t.Errorf("Anchored inputs %v, want [true false]", anchored)
	}
}

// TestFloatingNested: a submenu anchored to an item of a menu that is
// itself floating is placed after the menu in the same frame, even when
// its portal comes first in tree order.
func TestFloatingNested(t *testing.T) {
	var anchor, item ElementRef
	right := func(in FloatInput) FloatResult { return FloatResult{X: in.Anchor.Right(), Y: in.Anchor.Y} }
	var c1 int
	comp := func() Node {
		anchor, item = UseElementRef(), UseElementRef()
		return div(Class("p-4"),
			Portal(div(Key("sub"), Floating(FloatOptions{Anchor: item, Place: right}), Class("fixed h-10 w-20"))),
			div(Key("anchor"), AttachRef(anchor), Class("h-8 w-20")),
			Portal(div(Key("menu"), Floating(FloatOptions{Anchor: anchor, Place: below(2, &c1)}), Class("fixed w-24 p-1"),
				div(Key("i0"), Class("h-6")),
				div(Key("i1"), AttachRef(item), Class("h-6")))))
	}
	tr := newTestTree(t, 400, 300, Component(comp), nil)
	tr.Frame()
	// The menu at (16, 50); its second item at (20, 78), 88 wide: the
	// submenu at (108, 78).
	if g := popRect(tr, "menu"); !near(g.X0, 16, 1e-3) || !near(g.Y0, 50, 1e-3) {
		t.Errorf("menu at %v", g)
	}
	if g := popRect(tr, "sub"); !near(g.X0, 108, 1e-3) || !near(g.Y0, 78, 1e-3) {
		t.Errorf("submenu at %v, want (108, 78)", g)
	}
}

// TestFloatingSnapsToDevicePixels: at a fractional scale the popover moves
// by whole device pixels.
func TestFloatingSnapsToDevicePixels(t *testing.T) {
	var anchor ElementRef
	place := func(in FloatInput) FloatResult { return FloatResult{X: 10.3, Y: 20.45} }
	comp := func() Node {
		anchor = UseElementRef()
		return div(div(Key("anchor"), AttachRef(anchor), Class("h-8 w-20")),
			Portal(div(Key("pop"), Floating(FloatOptions{Anchor: anchor, Place: place}), Class("fixed size-10"))))
	}
	tr := newTestTree(t, 400, 300, Component(comp), nil)
	tr.SetScale(1.5)
	tr.Frame()
	g := popRect(tr, "pop")
	for _, v := range []float32{g.X0, g.Y0} {
		d := float64(v) * 1.5
		if r := d - float64(int(d+0.5)); r > 1e-4 || r < -1e-4 {
			t.Errorf("popover edge %g is not on the device pixel grid (%g device px)", v, d)
		}
	}
	if !near(g.X0, 10, 1e-3) || !near(g.Y0, 20.666667, 1e-3) {
		t.Errorf("popover at %v, want (10, 20.67): 10.3 and 20.45 rounded to device pixels", g)
	}
}

// TestFloatingVirtualAnchor: AnchorRect's Value reads place the popover
// again when they change (a context menu following the pointer).
func TestFloatingVirtualAnchor(t *testing.T) {
	var at *Value[Point]
	calls := 0
	comp := func() Node {
		at = UseValue(Point{X: 50, Y: 60})
		return div(Portal(div(Key("pop"), Floating(FloatOptions{
			AnchorRect: func() Rect { p := at.Get(); return Rect{X: p.X, Y: p.Y} },
			Place:      below(0, &calls),
		}), Class("fixed size-10"))))
	}
	tr := newTestTree(t, 400, 300, Component(comp), nil)
	tr.Frame()
	if g := popRect(tr, "pop"); !near(g.X0, 50, 1e-3) || !near(g.Y0, 60, 1e-3) {
		t.Errorf("popover at %v", g)
	}
	before := tr.Stats()
	at.Set(Point{X: 70, Y: 90})
	tr.Frame()
	if g := popRect(tr, "pop"); !near(g.X0, 70, 1e-3) || !near(g.Y0, 90, 1e-3) {
		t.Errorf("popover at %v after the anchor moved", g)
	}
	if s := tr.Stats(); s.Builds != before.Builds || s.Layouts != before.Layouts {
		t.Error("a virtual anchor's move built or laid out")
	}
}

// TestFloatingExitAndLayoutAnimation: a popover keeps following its
// anchor while its Presence exit runs, and follows an anchor moved by a
// layout animation through frames that only repaint.
func TestFloatingExitAndLayoutAnimation(t *testing.T) {
	var anchor, list ElementRef
	var setOpen Setter[bool]
	var setPad Setter[int]
	calls := 0
	comp := func() Node {
		anchor, list = UseElementRef(), UseElementRef()
		open, so := UseState(true)
		pad, sp := UseState(50)
		setOpen, setPad = so, sp
		return div(
			div(Key("list"), AttachRef(list), Class("h-[200px] overflow-y-auto"),
				div(Style(style.Height(float32(pad)))),
				div(Key("anchor"), AttachRef(anchor), AnimateLayout(), Class("h-8 w-20 duration-300 ease-linear")),
				div(Class("h-[600px]"))),
			Presence(If(open, Portal(div(Key("pop"), Floating(FloatOptions{Anchor: anchor, Place: below(0, &calls)}),
				Exit("opacity-0"), Class("fixed size-10 bg-white transition-opacity duration-300"))))))
	}
	x := newInput(t, Component(comp), 400, 300)
	tr := x.tree()
	if g := popRect(tr, "pop"); !near(g.Y0, 82, 1e-3) {
		t.Fatalf("popover at %v, want y 82", g)
	}
	// The anchor moves down by 40 px, animated by a 300 ms linear tween
	// (paint-only frames after the first): the popover follows its drawn
	// position on every frame.
	setPad.Set(90)
	x.frame()
	builds := tr.Stats().Builds
	var ys []float32
	for range 5 {
		x.frame()
		a := tr.boxWindowRect(keyed(tr, "anchor").box)
		g := popRect(tr, "pop")
		if !near(g.Y0, a.Y1, 0.5) { // rounded to device pixels
			t.Errorf("popover at y %g, the drawn anchor's bottom at %g", g.Y0, a.Y1)
		}
		ys = append(ys, g.Y0)
	}
	if ys[0] >= ys[len(ys)-1] || ys[len(ys)-1] > 122 || tr.Stats().Builds != builds {
		t.Errorf("popover positions %v during the layout animation, builds %d -> %d", ys, builds, tr.Stats().Builds)
	}
	for range 30 {
		x.frame()
	}
	if g := popRect(tr, "pop"); !near(g.Y0, 122, 1e-3) {
		t.Fatalf("popover at %v after the animation, want y 122", g)
	}
	// Closing: the popover exits (inert, still painted) and still follows
	// a scroll.
	setOpen.Set(false)
	x.frame()
	list.ScrollTo(0, 20)
	x.frame()
	pop := keyed(tr, "pop")
	if pop == nil || !pop.inert {
		t.Fatal("the popover is not exiting")
	}
	if g := popRect(tr, "pop"); !near(g.Y0, 102, 1e-3) {
		t.Errorf("exiting popover at %v, want y 102", g)
	}
	for range 30 {
		x.frame()
	}
	if keyed(tr, "pop") != nil {
		t.Error("the popover did not unmount after its exit")
	}
}

// TestUseSizeIgnoresScrollAndTransforms: UseSize follows layout only.
func TestUseSizeIgnoresScrollAndTransforms(t *testing.T) {
	var box, list ElementRef
	var size *Value[Size]
	var w *Value[float32]
	var tx *Value[float32]
	var sizes []Size
	reader := func() Node {
		sizes = append(sizes, size.Get())
		return span()
	}
	comp := func() Node {
		box, list = UseElementRef(), UseElementRef()
		size = UseSize(box)
		w = UseValue[float32](100)
		tx = UseValue[float32](0)
		return div(Component(reader),
			div(Key("list"), AttachRef(list), Class("h-[100px] overflow-y-auto"),
				div(Class("h-10")),
				div(Key("box"), AttachRef(box), Class("h-12 scale-150"),
					Style(style.WidthFn(func() float32 { return w.Get() }), style.TranslateFn(func() (float32, float32) { return tx.Get(), 0 }))),
				div(Class("h-[400px]"))))
	}
	tr := newTestTree(t, 400, 300, Component(comp), nil)
	tr.Frame()
	if got := size.Get(); got != (Size{Width: 100, Height: 48}) {
		t.Fatalf("UseSize %v, want 100x48 (the transform does not count)", got)
	}
	tr.Frame() // the reader's build
	n := len(sizes)
	list.ScrollTo(0, 25)
	tr.Frame()
	tx.Set(30)
	tr.Frame()
	if tr.Dirty() || len(sizes) != n {
		t.Errorf("scrolling or moving the box changed its size: %v", sizes[n-1:])
	}
	w.Set(140)
	tr.Frame()
	if got := size.Get(); got != (Size{Width: 140, Height: 48}) {
		t.Errorf("UseSize %v after the width changed", got)
	}
	tr.Frame()
	if got := sizes[len(sizes)-1]; got.Width != 140 {
		t.Errorf("the reader built with %v", got)
	}
}
