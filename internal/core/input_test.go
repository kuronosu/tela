package core

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/style"
	"github.com/kuronosu/tela/internal/testutil/golden"
)

// ---------------------------------------------------------------------
// Harness: a headless host driven by injected platform pointer events.

type inputHarness struct {
	t *testing.T
	b *headless.Backend
	h *Host
	w *headless.Window
}

func newInput(t *testing.T, root Node, w, h float32) *inputHarness {
	t.Helper()
	b := headless.New(headless.Options{Size: platform.Size{W: w, H: h}})
	host, err := NewHost(b, Options{Config: Config{Fonts: testFonts}, Root: root})
	if err != nil {
		t.Fatal(err)
	}
	b.Drain(host)
	x := &inputHarness{t: t, b: b, h: host, w: host.Window().(*headless.Window)}
	t.Cleanup(host.stopTimer)
	return x
}

func (x *inputHarness) tree() *Tree { return x.h.Tree() }

// send injects events and delivers them (and the frames due now).
func (x *inputHarness) send(es ...platform.PointerEvent) {
	for _, e := range es {
		x.w.Inject(e)
	}
	x.b.Drain(x.h)
}

// frame advances the clock by one refresh and delivers a frame requested
// meanwhile.
func (x *inputHarness) frame() {
	x.b.Advance(x.b.FrameInterval())
	x.b.Drain(x.h)
}

func ptr(a platform.PointerAction, px, py float32, b platform.PointerButton, held platform.Buttons) platform.PointerEvent {
	return platform.PointerEvent{Action: a, Pos: platform.Point{X: px, Y: py}, Button: b, Buttons: held}
}

func move(px, py float32) platform.PointerEvent {
	return ptr(platform.PointerMove, px, py, platform.ButtonNone, 0)
}

func moveHeld(px, py float32) platform.PointerEvent {
	return ptr(platform.PointerMove, px, py, platform.ButtonNone, platform.ButtonPrimary.Mask())
}

func down(px, py float32) platform.PointerEvent {
	return ptr(platform.PointerDown, px, py, platform.ButtonPrimary, platform.ButtonPrimary.Mask())
}

func up(px, py float32) platform.PointerEvent {
	return ptr(platform.PointerUp, px, py, platform.ButtonPrimary, 0)
}

func (x *inputHarness) click(px, py float32) {
	x.send(move(px, py), down(px, py), up(px, py))
}

// keyed returns the mounted primitive with key k.
func keyed(tr *Tree, k any) *Element {
	var found *Element
	var walk func(el *Element)
	walk = func(el *Element) {
		if found != nil {
			return
		}
		if el.kind == elemPrimitive && el.keyed && safeEqual(el.key, k) {
			found = el
			return
		}
		for _, c := range el.children {
			walk(c)
		}
	}
	walk(tr.root)
	return found
}

func keyName(el *Element) string {
	if el == nil {
		return "<nil>"
	}
	if el.parent == nil {
		return "root"
	}
	if el.keyed {
		return fmt.Sprint(el.key)
	}
	return "<" + el.box.kind.String() + ">"
}

// recorder logs events as "name:key" strings.
type recorder struct{ log []string }

func (r *recorder) add(s string) { r.log = append(r.log, s) }
func (r *recorder) String() string {
	return strings.Join(r.log, " ")
}
func (r *recorder) reset() { r.log = r.log[:0] }

// mouse returns a MouseEvent handler prop logging name:id.
func (r *recorder) mouse(event, id string) Prop {
	return On(event, func(e MouseEvent) { r.add(event + ":" + id) })
}

func (r *recorder) pointer(event, id string) Prop {
	return On(event, func(e PointerEvent) { r.add(event + ":" + id) })
}

// ---------------------------------------------------------------------
// Hit-testing.

func hitName(tr *Tree, x, y float32) string { return keyName(tr.HitTest(x, y)) }

func TestHitTestBasics(t *testing.T) {
	root := div(Class("relative h-[300px] w-[400px]"), Key("page"),
		// Overflow clip: the child extends beyond its clipping parent.
		div(Key("clip"), Class("absolute left-0 top-0 h-[50px] w-[50px] overflow-hidden"),
			div(Key("wide"), Class("h-[50px] w-[200px]"))),
		// Rounded clip and rounded self.
		div(Key("round"), Class("absolute left-[100px] top-0 size-[100px] overflow-hidden rounded-full"),
			div(Key("fill"), Class("size-[100px]"))),
		div(Key("pill"), Class("absolute left-[250px] top-0 h-[40px] w-[100px] rounded-full")),
		// pointer-events-none overlay with an auto child.
		div(Key("under"), Class("absolute left-0 top-[120px] h-[60px] w-[120px]")),
		div(Key("overlay"), Class("absolute left-0 top-[120px] h-[60px] w-[120px] pointer-events-none"),
			div(Key("auto"), Class("pointer-events-auto h-[20px] w-[20px]"))),
		// visibility: hidden.
		div(Key("visible"), Class("absolute left-[200px] top-[120px] size-[40px]")),
		div(Key("hidden"), Class("invisible absolute left-[200px] top-[120px] size-[40px]")),
		// z-index: the earlier sibling on top.
		div(Key("z10"), Class("absolute left-[300px] top-[120px] z-10 size-[40px]")),
		div(Key("z0"), Class("absolute left-[300px] top-[120px] size-[40px]")),
		div(Key("neg"), Class("absolute left-[300px] top-[200px] -z-10 size-[40px]")),
	)
	tr := newTestTree(t, 400, 300, root, nil)
	tr.Frame()
	for _, c := range []struct {
		x, y float32
		want string
	}{
		{10, 10, "wide"},
		{70, 10, "page"}, // the wide child is clipped by "clip"
		{150, 50, "fill"},
		{103, 3, "page"},  // the round clip's corner
		{252, 2, "page"},  // the pill's rounded corner
		{300, 20, "pill"}, // its middle
		{10, 125, "auto"},
		{60, 150, "under"}, // through the pointer-events-none overlay
		{210, 130, "visible"},
		{310, 130, "z10"},
		{310, 210, "page"}, // a negative z-index paints below the positioned page
		{-5, 10, "<nil>"},
		{10, 400, "<nil>"},
	} {
		if got := hitName(tr, c.x, c.y); got != c.want {
			t.Errorf("hit(%g, %g) = %s, want %s", c.x, c.y, got, c.want)
		}
	}
}

func TestHitTestNegativeZ(t *testing.T) {
	// A negative z-index box paints above its stacking context's own
	// background and below its in-flow content.
	root := div(Key("ctx"), Class("relative isolate h-[200px] w-[200px]"),
		div(Key("flow"), Class("h-[50px] w-[50px]")),
		div(Key("neg"), Class("absolute left-0 top-0 -z-10 size-[100px]")),
	)
	tr := newTestTree(t, 400, 300, root, nil)
	tr.Frame()
	if got := hitName(tr, 10, 10); got != "flow" {
		t.Errorf("over the in-flow box: %s, want flow", got)
	}
	if got := hitName(tr, 80, 80); got != "neg" {
		t.Errorf("over the negative box: %s, want neg", got)
	}
	if got := hitName(tr, 150, 150); got != "ctx" {
		t.Errorf("over the context: %s, want ctx", got)
	}
}

func TestHitTestTransforms(t *testing.T) {
	var x *Value[float32]
	app := func() Node {
		x = UseValue[float32](0)
		return div(Class("relative h-[300px] w-[400px]"), Key("page"),
			div(Key("moved"), Class("absolute left-0 top-0 size-[40px] translate-x-[100px]")),
			div(Key("scaled"), Class("absolute left-[200px] top-[100px] size-[40px] scale-200")),
			div(Key("deferred"), Class("absolute left-0 top-[200px] size-[40px]"),
				Style(style.TranslateFn(func() (float32, float32) { return x.Get(), 0 }))),
			div(Key("zero"), Class("absolute left-[300px] top-[200px] size-[40px] scale-0")),
		)
	}
	tr := newTestTree(t, 400, 300, Component(app), nil)
	tr.Frame()
	for _, c := range []struct {
		x, y float32
		want string
	}{
		{10, 10, "page"},
		{110, 10, "moved"},
		{185, 85, "scaled"}, // scale 2 about the center grows the box to 180..260
		{10, 210, "deferred"},
		{320, 220, "page"}, // scale 0 hits nothing
	} {
		if got := hitName(tr, c.x, c.y); got != c.want {
			t.Errorf("hit(%g, %g) = %s, want %s", c.x, c.y, got, c.want)
		}
	}
	x.Set(150)
	tr.Frame()
	if got := hitName(tr, 10, 210); got != "page" {
		t.Errorf("after the deferred translate, old place: %s", got)
	}
	if got := hitName(tr, 160, 210); got != "deferred" {
		t.Errorf("after the deferred translate, new place: %s", got)
	}
}

func TestHitTestInline(t *testing.T) {
	root := div(Class("p-0"),
		p(Key("para"), Class("w-[300px] text-[20px] leading-[30px]"),
			Text("Hello "), strong(Key("strong"), Text("world"), em(Key("em"), Text("!!!"))),
			span(Key("ghost"), Class("pointer-events-none"), Text(" ghost")),
		),
	)
	tr := newTestTree(t, 400, 300, root, nil)
	tr.Frame()
	para := keyed(tr, "para")
	lay := para.box.para.lay
	// The x of a byte offset: the start of its cluster.
	xAt := func(off int) float32 {
		c := lay.Cluster(lay.ClusterAt(off))
		return c.X + c.Advance/2
	}
	text := lay.Text()
	for _, c := range []struct {
		sub, want string
	}{
		{"Hello", "para"},
		{"world", "strong"},
		{"!!!", "em"},
		{"ghost", "para"},
	} {
		off := strings.Index(text, c.sub)
		if got := hitName(tr, xAt(off), 15); got != c.want {
			t.Errorf("on %q: %s, want %s", c.sub, got, c.want)
		}
	}
	if got := hitName(tr, 290, 15); got != "para" {
		t.Errorf("after the text: %s", got)
	}
}

// ---------------------------------------------------------------------
// Dispatch.

func TestEventOrder(t *testing.T) {
	var r recorder
	var counts []int
	root := div(Class("p-10"),
		button(Key("b"), Class("h-[40px] w-[100px]"),
			r.pointer("pointerdown", "b"), r.mouse("mousedown", "b"),
			r.pointer("pointerup", "b"), r.mouse("mouseup", "b"),
			On("click", func(e MouseEvent) { r.add("click:b"); counts = append(counts, e.ClickCount) }),
			r.mouse("dblclick", "b"), r.mouse("contextmenu", "b"),
		),
	)
	x := newInput(t, root, 400, 300)
	x.click(50, 50)
	if got := r.String(); got != "pointerdown:b mousedown:b pointerup:b mouseup:b click:b" {
		t.Errorf("order: %s", got)
	}
	r.reset()
	x.b.Advance(100 * time.Millisecond)
	x.click(52, 51)
	if got := r.String(); got != "pointerdown:b mousedown:b pointerup:b mouseup:b click:b dblclick:b" {
		t.Errorf("second click: %s", got)
	}
	if fmt.Sprint(counts) != "[1 2]" {
		t.Errorf("click counts %v", counts)
	}
	r.reset()
	x.b.Advance(time.Second)
	x.send(ptr(platform.PointerDown, 50, 50, platform.ButtonSecondary, platform.ButtonSecondary.Mask()),
		ptr(platform.PointerUp, 50, 50, platform.ButtonSecondary, 0))
	if got := r.String(); got != "pointerdown:b mousedown:b pointerup:b mouseup:b contextmenu:b" {
		t.Errorf("secondary: %s", got)
	}
	// Chorded buttons: pointerdown and pointerup once, mousedown and
	// mouseup per button.
	r.reset()
	x.b.Advance(time.Second)
	both := platform.ButtonPrimary.Mask() | platform.ButtonSecondary.Mask()
	x.send(down(50, 50),
		ptr(platform.PointerDown, 50, 50, platform.ButtonSecondary, both),
		ptr(platform.PointerUp, 50, 50, platform.ButtonSecondary, platform.ButtonPrimary.Mask()),
		up(50, 50))
	if got := r.String(); got != "pointerdown:b mousedown:b mousedown:b mouseup:b contextmenu:b pointerup:b mouseup:b click:b" {
		t.Errorf("chorded: %s", got)
	}
}

func TestClickOnCommonAncestor(t *testing.T) {
	var r recorder
	root := div(Key("row"), Class("flex h-[50px] w-[300px]"), r.mouse("click", "row"),
		div(Key("a"), Class("w-[100px]"), r.mouse("click", "a")),
		div(Key("b"), Class("w-[100px]"), r.mouse("click", "b")),
	)
	x := newInput(t, root, 400, 300)
	// Press on a, release on b (the mouse moved 150 px: still a click,
	// as no drag recognizer is on the path).
	x.send(move(50, 20), down(50, 20), moveHeld(150, 20), up(150, 20))
	if got := r.String(); got != "click:row" {
		t.Errorf("got %q, want the click on the common ancestor", got)
	}
	r.reset()
	x.b.Advance(time.Second)
	x.send(down(50, 20), moveHeld(80, 25), up(80, 25))
	if got := r.String(); got != "click:a click:row" {
		t.Errorf("got %q", got)
	}
}

func TestBubblingAndStopPropagation(t *testing.T) {
	var r recorder
	stop := false
	root := div(Key("outer"), Class("p-4"), r.mouse("click", "outer"), r.mouse("mousedown", "outer"),
		div(Key("mid"), Class("p-4"), r.mouse("click", "mid"),
			div(Key("inner"), Class("size-10"),
				On("click", func(e MouseEvent) {
					r.add("click:inner")
					if stop {
						e.StopPropagation()
					}
				}),
				On("click", func(e MouseEvent) { r.add("click:inner2") }),
			)),
	)
	x := newInput(t, root, 400, 300)
	x.click(40, 40)
	if got := r.String(); got != "mousedown:outer click:inner click:inner2 click:mid click:outer" {
		t.Errorf("bubbling: %s", got)
	}
	r.reset()
	stop = true
	x.b.Advance(time.Second)
	x.click(40, 40)
	if got := r.String(); got != "mousedown:outer click:inner click:inner2" {
		t.Errorf("stopped: %s", got)
	}
}

func TestLocalCoordinates(t *testing.T) {
	var got []float32
	root := div(Class("relative h-[300px] w-[400px]"),
		div(Key("box"), Class("absolute left-[100px] top-[50px] size-[100px] scale-200"),
			On("mousedown", func(e MouseEvent) { got = append(got, e.X, e.Y, e.LocalX, e.LocalY) }),
			div(Key("child"), Class("ml-[10px] mt-[10px] size-[20px]"),
				On("mousedown", func(e MouseEvent) { got = append(got, e.LocalX, e.LocalY) })),
		),
	)
	x := newInput(t, root, 400, 300)
	// The box is scaled 2x about its center (150, 100): local (15, 15)
	// is at window (150 + (15-50)*2, 100 + (15-50)*2) = (80, 30).
	x.send(down(80, 30), up(80, 30))
	want := []float32{5, 5, 80, 30, 15, 15}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("got %v, want %v (child local, then window and box local)", got, want)
	}
}

func TestEnterLeave(t *testing.T) {
	var r recorder
	root := div(Class("flex"),
		div(Key("a"), Class("p-4"), r.mouse("mouseenter", "a"), r.mouse("mouseleave", "a"),
			r.pointer("pointerenter", "a"), r.pointer("pointerleave", "a"),
			div(Key("b"), Class("size-10"), r.mouse("mouseenter", "b"), r.mouse("mouseleave", "b"),
				r.mouse("mousemove", "b"))),
		div(Key("c"), Class("size-10"), r.mouse("mouseenter", "c"), r.mouse("mouseleave", "c")),
	)
	x := newInput(t, root, 400, 300)
	x.send(move(20, 20)) // into b (and a)
	if got := r.String(); got != "pointerenter:a mouseenter:a mouseenter:b mousemove:b" {
		t.Errorf("enter: %s", got)
	}
	r.reset()
	x.send(move(22, 22)) // within b: no boundary events
	if got := r.String(); got != "mousemove:b" {
		t.Errorf("move: %s", got)
	}
	r.reset()
	x.send(move(80, 10)) // to c
	if got := r.String(); got != "pointerleave:a mouseleave:b mouseleave:a mouseenter:c" {
		t.Errorf("to c: %s", got)
	}
	r.reset()
	x.send(ptr(platform.PointerLeave, 80, 10, 0, 0))
	if got := r.String(); got != "mouseleave:c" {
		t.Errorf("leave the window: %s", got)
	}
}

func TestDisabled(t *testing.T) {
	var r recorder
	disabled := true
	var setDisabled Setter[bool]
	app := func() Node {
		d, set := UseState(disabled)
		setDisabled = set
		return div(Key("card"), Class("p-4"), r.mouse("click", "card"),
			button(Key("btn"), Class("h-[40px] w-[100px] bg-white disabled:bg-slate-200 active:bg-sky-500"),
				Disabled(d), r.mouse("click", "btn"), r.mouse("mousedown", "btn"),
				r.mouse("mouseenter", "btn"),
				span(Key("label"), r.mouse("click", "label"), Text("Save"))))
	}
	x := newInput(t, Component(app), 400, 300)
	tr := x.tree()
	btn := keyed(tr, "btn")
	if btn.box.cs.BackgroundColor == gfx.White {
		t.Error("disabled: not applied")
	}
	x.send(move(66, 36), down(66, 36))
	if btn.istate&style.StateActive != 0 {
		t.Error("a disabled element became active")
	}
	x.send(up(66, 36))
	if got := r.String(); got != "click:card" {
		t.Errorf("disabled: %s, want only the ancestor's click", got)
	}
	r.reset()
	setDisabled.Set(false)
	x.frame()
	if btn.box.cs.BackgroundColor != gfx.White {
		t.Error("disabled: still applied")
	}
	x.b.Advance(time.Second)
	x.send(down(66, 36), up(66, 36))
	if got := r.String(); got != "mousedown:btn click:label click:btn click:card" {
		t.Errorf("enabled: %s", got)
	}
}

// ---------------------------------------------------------------------
// Gestures.

func TestArena(t *testing.T) {
	var r recorder
	var drags []DragEvent
	logDrag := func(name string) Prop {
		return On(name, func(e DragEvent) { r.add(name); drags = append(drags, e) })
	}
	root := div(Class("p-10"),
		div(Key("card"), Class("size-[100px]"),
			r.mouse("click", "card"), logDrag("dragstart"), logDrag("drag"), logDrag("dragend"),
			r.pointer("longpress", "card")),
	)
	x := newInput(t, root, 400, 300)

	// A press without movement: a click.
	x.click(50, 50)
	if got := r.String(); got != "click:card" {
		t.Errorf("tap: %s", got)
	}
	// Moving 3 px (within the mouse slop): still a click.
	r.reset()
	x.b.Advance(time.Second)
	x.send(down(50, 50), moveHeld(53, 50), up(53, 50))
	if got := r.String(); got != "click:card" {
		t.Errorf("small move: %s", got)
	}
	// Past the slop: a drag, no click.
	r.reset()
	drags = nil
	x.b.Advance(time.Second)
	x.send(down(50, 50), moveHeld(56, 50), moveHeld(70, 60), up(70, 60))
	if got := r.String(); got != "dragstart drag dragend" {
		t.Errorf("drag: %s", got)
	}
	if d := drags[0]; d.OffsetX != 6 || d.DeltaX != 6 || d.LocalX != 16 || d.LocalY != 10 || d.Button != MouseButtonLeft {
		t.Errorf("drag start %+v", d)
	}
	if d := drags[1]; d.DeltaX != 14 || d.DeltaY != 10 || d.OffsetX != 20 || d.OffsetY != 10 {
		t.Errorf("drag %+v", d)
	}
	// Holding still: a long press after 500 ms of the backend clock, and
	// no click on release.
	r.reset()
	x.b.Advance(time.Second)
	x.send(down(50, 50))
	at, ok := x.h.TimerArmed()
	if !ok || at != x.b.Now()+500*time.Millisecond {
		t.Fatalf("timer %v %v, want armed at now+500ms", at, ok)
	}
	x.b.Advance(499 * time.Millisecond)
	x.b.Wake()
	x.b.Drain(x.h)
	if r.String() != "" {
		t.Fatalf("early: %s", r.String())
	}
	x.b.Advance(time.Millisecond)
	x.b.Wake()
	x.b.Drain(x.h)
	if got := r.String(); got != "longpress:card" {
		t.Errorf("long press: %s", got)
	}
	if _, ok := x.h.TimerArmed(); ok {
		t.Error("timer still armed after the long press")
	}
	x.send(moveHeld(90, 90), up(90, 90))
	if got := r.String(); got != "longpress:card" {
		t.Errorf("after the long press: %s", got)
	}
	// A release before the timeout disarms the timer.
	r.reset()
	x.b.Advance(time.Second)
	x.send(down(50, 50))
	if _, ok := x.h.TimerArmed(); !ok {
		t.Error("no timer while pressed")
	}
	x.send(up(50, 50))
	if _, ok := x.h.TimerArmed(); ok {
		t.Error("timer armed after the release")
	}
	if got := r.String(); got != "click:card" {
		t.Errorf("quick press: %s", got)
	}
}

func TestDragVelocity(t *testing.T) {
	var end DragEvent
	root := div(Key("knob"), Class("size-[100px]"),
		On("dragend", func(e DragEvent) { end = e }), On("drag", func(DragEvent) {}))
	x := newInput(t, root, 400, 300)
	t0 := x.b.Now()
	ev := func(a platform.PointerAction, px float32, ms int, held platform.Buttons, b platform.PointerButton) platform.PointerEvent {
		e := ptr(a, px, 50, b, held)
		e.Time = t0 + time.Duration(ms)*time.Millisecond + 1
		return e
	}
	held := platform.ButtonPrimary.Mask()
	es := []platform.PointerEvent{ev(platform.PointerDown, 10, 0, held, platform.ButtonPrimary)}
	for i := 1; i <= 8; i++ {
		es = append(es, ev(platform.PointerMove, 10+float32(i*8), i*8, held, 0)) // 1000 px/s
	}
	es = append(es, ev(platform.PointerUp, 74, 64, 0, platform.ButtonPrimary))
	x.send(es...)
	if end.VelocityX < 990 || end.VelocityX > 1010 || end.VelocityY > 1e-3 || end.VelocityY < -1e-3 {
		t.Errorf("velocity (%g, %g), want (1000, 0)", end.VelocityX, end.VelocityY)
	}
	if end.OffsetX != 64 {
		t.Errorf("offset %g, want 64", end.OffsetX)
	}
}

func TestNestedDragInnermostWins(t *testing.T) {
	var r recorder
	root := div(Key("outer"), Class("p-10"), On("dragstart", func(DragEvent) { r.add("outer") }),
		div(Key("inner"), Class("size-10"), On("dragstart", func(DragEvent) { r.add("inner") })))
	x := newInput(t, root, 400, 300)
	x.send(down(50, 50), moveHeld(80, 50), up(80, 50))
	if got := r.String(); got != "inner" {
		t.Errorf("got %q", got)
	}
}

func TestTouch(t *testing.T) {
	var r recorder
	root := div(Class("flex"),
		div(Key("a"), Class("size-[100px] hover:bg-red-500"), r.pointer("pointerenter", "a"), r.pointer("pointerdown", "a"),
			r.pointer("pointermove", "a"), r.pointer("pointerup", "a"), r.pointer("pointerleave", "a"), r.mouse("click", "a")),
		div(Key("b"), Class("size-[100px]"), r.pointer("pointermove", "b")),
	)
	x := newInput(t, root, 400, 300)
	tp := func(a platform.PointerAction, px float32, b platform.PointerButton, held platform.Buttons) platform.PointerEvent {
		e := ptr(a, px, 50, b, held)
		e.Kind, e.ID = platform.PointerTouch, 7
		return e
	}
	held := platform.ButtonPrimary.Mask()
	x.send(tp(platform.PointerDown, 50, platform.ButtonPrimary, held))
	a := keyed(x.tree(), "a")
	if a.istate&style.StateHover != 0 {
		t.Error("touch hovers")
	}
	// Implicit capture: the move over b still targets a.
	x.send(tp(platform.PointerMove, 150, 0, held), tp(platform.PointerUp, 150, platform.ButtonPrimary, 0))
	if got := r.String(); got != "pointerenter:a pointerdown:a pointermove:a pointerup:a click:a pointerleave:a" {
		t.Errorf("touch: %s", got)
	}
	if len(x.tree().in.ptrs) != 0 {
		t.Errorf("touch pointer kept after release: %d", len(x.tree().in.ptrs))
	}
}

// ---------------------------------------------------------------------
// Interaction variants.

func TestHoverActiveRestyleOnlyAffected(t *testing.T) {
	items := make([]Node, 50)
	for i := range items {
		items[i] = div(Key(i), Class("h-[4px] bg-slate-100"))
	}
	root := div(Class("flex flex-col"),
		div(Key("plain"), Class("h-[20px]")),
		button(Key("btn"), Class("h-[20px] w-[80px] bg-white hover:bg-sky-500 active:scale-95")),
		List(items),
	)
	x := newInput(t, root, 400, 300)
	tr := x.tree()
	btn := keyed(tr, "btn")
	s0 := tr.Stats()
	x.send(move(10, 10)) // over "plain": no class tests hover there
	if tr.Dirty() {
		t.Error("hovering an element without hover classes made the tree dirty")
	}
	x.send(move(10, 30)) // over the button
	if !x.w.FrameRequested() {
		t.Fatal("hover did not request a frame")
	}
	x.frame()
	s1 := tr.Stats()
	if d := s1.Styles - s0.Styles; d != 1 {
		t.Errorf("hover restyled %d elements, want 1", d)
	}
	if d := s1.Builds - s0.Builds; d != 0 {
		t.Errorf("hover built %d components", d)
	}
	if btn.box.cs.BackgroundColor == gfx.White {
		t.Error("hover:bg-sky-500 not applied")
	}
	x.send(down(10, 30))
	x.frame()
	if btn.box.cs.Transform.ScaleX != 0.95 {
		t.Errorf("active:scale-95 not applied: %+v", btn.box.cs.Transform)
	}
	x.send(up(10, 30))
	x.frame()
	if btn.box.cs.Transform.ScaleX != 1 {
		t.Error("active not cleared on release")
	}
	x.send(move(10, 100))
	x.frame()
	if btn.box.cs.BackgroundColor != gfx.White {
		t.Error("hover not cleared")
	}
}

func TestGroupAndPeer(t *testing.T) {
	root := div(Class("flex flex-col"),
		div(Key("outer"), Class("group h-[100px]"),
			div(Key("pad"), Class("h-[50px]")),
			div(Key("inner"), Class("group h-[50px]"),
				span(Key("label"), Class("text-black group-hover:text-red-500"), Text("x")))),
		div(Key("peer"), Class("peer h-[20px]")),
		div(Key("between"), Class("h-[20px]")),
		div(Key("sib"), Class("h-[20px] bg-white peer-hover:bg-black peer-disabled:bg-red-500")),
	)
	x := newInput(t, root, 400, 300)
	tr := x.tree()
	label, sib := keyed(tr, "label"), keyed(tr, "sib")
	red := label.box.cs.Text.Color
	x.send(move(10, 10)) // over the outer group only: Tailwind's group-hover tests any group ancestor
	x.frame()
	if label.box.cs.Text.Color == red {
		t.Error("group-hover: not applied when an outer group is hovered")
	}
	x.send(move(10, 250))
	x.frame()
	if label.box.cs.Text.Color != red {
		t.Error("group-hover: not cleared")
	}
	s0 := tr.Stats()
	x.send(move(10, 110)) // over the peer
	x.frame()
	if sib.box.cs.BackgroundColor != gfx.Black {
		t.Errorf("peer-hover: not applied: %v", sib.box.cs.BackgroundColor)
	}
	if d := tr.Stats().Styles - s0.Styles; d != 1 {
		t.Errorf("peer hover restyled %d elements, want 1", d)
	}
}

func TestGroupDisabledAndEnabled(t *testing.T) {
	var set Setter[bool]
	app := func() Node {
		d, s := UseState(true)
		set = s
		return div(Key("g"), Class("group"), Disabled(d),
			div(Key("c"), Class("h-4 bg-white group-disabled:bg-black")),
			div(Key("e"), Class("h-4 bg-white enabled:bg-black")),
		)
	}
	x := newInput(t, Component(app), 400, 300)
	tr := x.tree()
	if keyed(tr, "c").box.cs.BackgroundColor != gfx.Black {
		t.Error("group-disabled: not applied")
	}
	set.Set(false)
	x.frame()
	if keyed(tr, "c").box.cs.BackgroundColor != gfx.White {
		t.Error("group-disabled: not cleared")
	}
	if keyed(tr, "e").box.cs.BackgroundColor != gfx.Black {
		t.Error("enabled: not applied to an element without Disabled")
	}
}

func TestCursor(t *testing.T) {
	root := div(Class("p-10"),
		button(Key("b"), Class("h-[40px] w-[100px]"), span(Text("label"))),
		div(Key("wait"), Class("cursor-wait h-[40px] w-[100px]")),
	)
	x := newInput(t, root, 400, 300)
	if c := x.w.Cursor(); c != platform.CursorDefault {
		t.Fatalf("initial cursor %v", c)
	}
	x.send(move(50, 50))
	if c := x.w.Cursor(); c != platform.CursorPointer {
		t.Errorf("over the button: %v, want pointer (inherited by the label)", c)
	}
	x.send(move(50, 90))
	if c := x.w.Cursor(); c != platform.CursorWait {
		t.Errorf("over cursor-wait: %v", c)
	}
	x.send(move(300, 250))
	if c := x.w.Cursor(); c != platform.CursorDefault {
		t.Errorf("over the page: %v", c)
	}
}

// ---------------------------------------------------------------------
// Builds between discrete events, idle.

func TestTwoClicksBeforeAFrame(t *testing.T) {
	x := newInput(t, goldenCounter(), 400, 300)
	tr := x.tree()
	b := tr.Inspect()
	btn := findKind(&b, "button")
	cx, cy := btn.Rect.Center().X, btn.Rect.Center().Y
	frames := tr.Stats().Frames
	// Both clicks are delivered in one loop iteration, before any frame.
	for _, e := range []platform.PointerEvent{move(cx, cy), down(cx, cy), up(cx, cy)} {
		x.w.Inject(e)
	}
	x.b.Advance(time.Second) // not a double click: two single clicks
	for _, e := range []platform.PointerEvent{down(cx, cy), up(cx, cy)} {
		e.Time = x.b.Now()
		x.w.Inject(e)
	}
	x.b.Step(x.h)
	if d := tr.Stats().Frames - frames; d != 1 {
		t.Errorf("%d frames, want 1 after the events", d)
	}
	b = tr.Inspect()
	if h := findKind(&b, "h1"); h == nil || h.Text != "Count: 2" {
		t.Errorf("heading %+v, want Count: 2", h)
	}
}

func findKind(n *InspectNode, kind string) *InspectNode {
	if n.Kind == kind {
		return n
	}
	for i := range n.Children {
		if f := findKind(&n.Children[i], kind); f != nil {
			return f
		}
	}
	return nil
}

func TestIdleInput(t *testing.T) {
	root := div(Class("p-10"),
		div(Key("lp"), Class("size-10"), On("longpress", func(PointerEvent) {})),
		div(Key("plain"), Class("size-10"), On("click", func(MouseEvent) {})))
	x := newInput(t, root, 400, 300)
	presents := x.h.Presents()
	x.send(move(50, 50), move(52, 52), move(50, 90))
	if x.w.FrameRequested() {
		t.Error("moving over elements without hover styles requested a frame")
	}
	x.click(50, 90)
	if x.w.FrameRequested() || x.h.Presents() != presents {
		t.Error("a click without state changes requested a frame")
	}
	if _, ok := x.h.TimerArmed(); ok {
		t.Error("timer armed while idle")
	}
	x.send(down(50, 50))
	if _, ok := x.h.TimerArmed(); !ok {
		t.Error("no long-press timer while pressed")
	}
	x.send(up(50, 50))
	if _, ok := x.h.TimerArmed(); ok {
		t.Error("timer armed after the release")
	}
	if x.b.Pending() {
		t.Error("work pending while idle")
	}
}

func TestUnmountedTargetIsSkipped(t *testing.T) {
	var r recorder
	var set Setter[bool]
	app := func() Node {
		show, s := UseState(true)
		set = s
		return div(Key("page"), Class("h-[300px]"), r.mouse("click", "page"),
			If(show, div(Key("btn"), Class("size-10"),
				On("mousedown", func(MouseEvent) { set.Set(false) }),
				r.mouse("click", "btn"))))
	}
	x := newInput(t, Component(app), 400, 300)
	// The press removes the button (rebuilt before mouseup); the click
	// goes to the nearest mounted ancestor.
	x.click(10, 10)
	if got := r.String(); got != "click:page" {
		t.Errorf("got %q", got)
	}
}

// ---------------------------------------------------------------------
// Golden: the Counter hovered and pressed.

func TestCounterInteractionGolden(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			b := headless.New(headless.Options{Size: platform.Size{W: 400, H: 300}, Scale: scale})
			host, err := NewHost(b, Options{Config: Config{Fonts: testFonts}, Root: goldenCounter()})
			if err != nil {
				t.Fatal(err)
			}
			b.Drain(host)
			x := &inputHarness{t: t, b: b, h: host, w: host.Window().(*headless.Window)}
			t.Cleanup(host.stopTimer)
			in := x.tree().Inspect()
			btn := findKind(&in, "button")
			c := btn.Rect.Center()
			x.send(move(c.X, c.Y))
			x.settle() // the button transitions its colors and transform
			golden.Image(t, fmt.Sprintf("counter_hover_x%g", scale), x.w.Frame())
			x.send(down(c.X, c.Y))
			x.settle()
			golden.Image(t, fmt.Sprintf("counter_active_x%g", scale), x.w.Frame())
			x.send(up(c.X, c.Y))
			x.frame()
			in = x.tree().Inspect()
			if h := findKind(&in, "h1"); h.Text != "Count: 1" {
				t.Errorf("heading %q after a click", h.Text)
			}
			if x.w.Cursor() != platform.CursorPointer {
				t.Errorf("cursor %v over the button", x.w.Cursor())
			}
		})
	}
}
