package core

import (
	"fmt"
	"math"
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
// Harness.

// newScrollHost is newInput with a Config (the scrolling options).
func newScrollHost(t *testing.T, root Node, w, h float32, cfg Config) *inputHarness {
	t.Helper()
	b := headless.New(headless.Options{Size: platform.Size{W: w, H: h}})
	cfg.Fonts = testFonts
	host, err := NewHost(b, Options{Config: cfg, Root: root})
	if err != nil {
		t.Fatal(err)
	}
	b.Drain(host)
	x := &inputHarness{t: t, b: b, h: host, w: host.Window().(*headless.Window)}
	t.Cleanup(host.stopTimer)
	t.Cleanup(func() { checkScrollSets(t, host.tree) })
	return x
}

// rows returns n keyed rows of 50 px.
func rows(n int) []Node {
	out := make([]Node, n)
	for i := range out {
		out[i] = div(Key(fmt.Sprintf("r%d", i)), Class("h-[50px]"))
	}
	return out
}

// listRoot is a 200x200 vertical scroller (key "list") of 20 rows: 1000
// px of content, 800 px of scroll range.
func listRoot(classes string, extra ...Child) Node {
	c := []Child{Key("list"), Class("h-[200px] w-[200px] overflow-y-auto " + classes)}
	c = append(c, extra...)
	for _, r := range rows(20) {
		c = append(c, r)
	}
	return div(Class("p-0"), div(c...))
}

// scr returns the scroller of the element with key k.
func scr(t *testing.T, tr *Tree, k string) *scroller {
	t.Helper()
	el := keyed(tr, k)
	if el == nil || el.box.scr == nil {
		t.Fatalf("%s is not a scroll container", k)
	}
	return el.box.scr
}

// offset returns the vertical offset of k.
func (x *inputHarness) offset(k string) float32 { return scr(x.t, x.tree(), k).pos.Y }

func (x *inputHarness) offsetX(k string) float32 { return scr(x.t, x.tree(), k).pos.X }

// wheel injects one wheel event and delivers it.
func (x *inputHarness) wheel(e platform.ScrollEvent) {
	x.w.Inject(e)
	x.b.Drain(x.h)
}

func notch(px, py, steps float32) platform.ScrollEvent {
	return platform.ScrollEvent{Pos: platform.Point{X: px, Y: py}, Steps: platform.Point{Y: steps}}
}

func touchpad(px, py, dy float32, phase platform.ScrollPhase) platform.ScrollEvent {
	return platform.ScrollEvent{Pos: platform.Point{X: px, Y: py}, Delta: platform.Point{Y: dy}, Phase: phase}
}

// settle delivers frames until none is requested (at most a few
// seconds of frames).
func (x *inputHarness) settle() {
	x.t.Helper()
	x.b.Drain(x.h) // state updates waiting for their wake
	for i := 0; i < 400 && x.w.FrameRequested(); i++ {
		x.frame()
	}
	if x.w.FrameRequested() {
		x.t.Fatal("frames never stopped")
	}
}

// idle checks that nothing runs: no frame requested and, after the
// overlay bars faded, no timer.
func (x *inputHarness) idle() {
	x.t.Helper()
	x.settle()
	if at, ok := x.h.TimerArmed(); ok {
		// The overlay bars' hold: let it run out, then nothing.
		x.b.Advance(max(at-x.b.Now(), 0) + time.Millisecond)
		x.b.Wake()
		x.b.Drain(x.h)
		x.settle()
	}
	if _, ok := x.h.TimerArmed(); ok || x.w.FrameRequested() {
		x.t.Fatalf("not idle: timer %v, frame requested %v", ok, x.w.FrameRequested())
	}
}

func approx(a, b float32) bool { return math.Abs(float64(a-b)) < 0.01 }

// ---------------------------------------------------------------------
// Scroll containers.

func TestScrollExtentsAndClamping(t *testing.T) {
	var setN func(int)
	ref := ElementRef{}
	var refOut ElementRef
	comp := func() Node {
		n, set := UseStateSet(20)
		setN = set
		r := UseElementRef()
		refOut = r
		c := []Child{Key("list"), AttachRef(r), Class("h-[200px] w-[200px] overflow-y-auto p-4")}
		for _, row := range rows(n) {
			c = append(c, row)
		}
		return div(c...)
	}
	x := newScrollHost(t, Component(comp), 400, 400, Config{})
	_ = ref
	s := scr(t, x.tree(), "list")
	// 20 rows of 50 px with 16 px of padding on both ends, in 200 px.
	if s.max.Y != 1000+32-200 || s.max.X != 0 || s.client != (gfx.Size{W: 200, H: 200}) {
		t.Fatalf("extents max %v client %v", s.max, s.client)
	}
	refOut.ScrollTo(0, 5000)
	if got := x.offset("list"); got != 832 {
		t.Errorf("ScrollTo past the end: %g, want 832", got)
	}
	refOut.ScrollTo(-10, -10)
	if x, y := refOut.ScrollOffset(); x != 0 || y != 0 {
		t.Errorf("ScrollTo before the start: %g,%g", x, y)
	}
	refOut.ScrollTo(0, 600)
	x.settle()
	// A rebuild keeps the offset; shrinking content clamps it.
	setN(19)
	x.settle()
	if got := x.offset("list"); got != 600 {
		t.Errorf("after a rebuild: %g, want 600", got)
	}
	setN(10)
	x.settle()
	if got := x.offset("list"); got != 500+32-200 {
		t.Errorf("after shrinking: %g, want %d", got, 500+32-200)
	}
	// The content moves on screen: row 9 (the last) ends at the bottom.
	if r := windowRect(keyed(x.tree(), "r9").box); !approx(r.Y1, 200-16) {
		t.Errorf("last row painted at %v", r)
	}
}

func TestScrollContainerKinds(t *testing.T) {
	root := div(Class("flex gap-2"),
		div(Key("hidden"), Class("h-[100px] w-[100px] overflow-hidden"), div(Class("h-[300px]"))),
		div(Key("clip"), Class("h-[100px] w-[100px] overflow-clip"), div(Class("h-[300px]"))),
		div(Key("auto"), Class("h-[100px] w-[100px] overflow-y-auto"), div(Class("h-[300px] w-[150px]"))),
	)
	x := newScrollHost(t, root, 400, 300, Config{InstantScroll: true})
	tr := x.tree()
	if keyed(tr, "clip").box.scr != nil {
		t.Error("overflow-clip is a scroll container")
	}
	// overflow-y-auto makes x auto too (CSS): both axes scroll.
	a := scr(t, tr, "auto")
	if !a.userX || !a.userY || a.max != (gfx.Point{X: 50, Y: 200}) {
		t.Errorf("auto: user %v %v max %v", a.userX, a.userY, a.max)
	}
	// overflow-hidden: no wheel scrolling, but programmatic scrolling.
	h := scr(t, tr, "hidden")
	x.wheel(notch(50, 50, 1))
	x.settle()
	if h.pos.Y != 0 {
		t.Errorf("wheel scrolled overflow-hidden to %g", h.pos.Y)
	}
	tr.scrollScripted(h, gfx.Point{Y: 70})
	if h.pos.Y != 70 {
		t.Errorf("programmatic scroll of overflow-hidden: %g", h.pos.Y)
	}
}

// ---------------------------------------------------------------------
// Wheel and touchpad.

func TestWheelNotchInstant(t *testing.T) {
	x := newScrollHost(t, listRoot(""), 300, 300, Config{InstantScroll: true})
	x.wheel(notch(100, 100, 1))
	if got := x.offset("list"); got != wheelNotch {
		t.Errorf("one notch: %g, want %d", got, wheelNotch)
	}
	// A high-resolution wheel reports fractions of a notch.
	x.wheel(notch(100, 100, 0.5))
	if got := x.offset("list"); got != wheelNotch*1.5 {
		t.Errorf("half a notch more: %g", got)
	}
	x.wheel(notch(100, 100, -1.5))
	if got := x.offset("list"); got != 0 {
		t.Errorf("back up: %g", got)
	}
	// Outside the scroller nothing scrolls (the move ends the wheel
	// transaction latched to the list).
	x.send(move(250, 250))
	x.wheel(notch(250, 250, 1))
	if got := x.offset("list"); got != 0 {
		t.Errorf("wheel outside: %g", got)
	}
}

func TestWheelNotchAnimation(t *testing.T) {
	x := newScrollHost(t, listRoot(""), 300, 300, Config{})
	t0 := x.b.Now()
	x.wheel(notch(100, 100, 1))
	if got := x.offset("list"); got != 0 {
		t.Errorf("before the first frame: %g", got)
	}
	// The ease-in-out curve over 150 ms: rising, halfway at 75 ms.
	var trace []float32
	for x.b.Now()-t0 < 70*time.Millisecond {
		x.frame()
		trace = append(trace, x.offset("list"))
	}
	for i := 1; i < len(trace); i++ {
		if trace[i] < trace[i-1] {
			t.Errorf("not monotonic: %v", trace)
		}
	}
	mid := trace[len(trace)-1]
	if mid <= 5 || mid >= 48 {
		t.Errorf("at %v: %g, want between", x.b.Now()-t0, mid)
	}
	// A second notch retargets: from where it is, with its velocity, to
	// two notches.
	x.wheel(notch(100, 100, 1))
	prev := x.offset("list")
	x.frame()
	if got := x.offset("list"); got < prev {
		t.Errorf("retarget went back: %g -> %g", prev, got)
	}
	x.settle()
	if got := x.offset("list"); got != 2*wheelNotch {
		t.Errorf("after two notches: %g, want %d", got, 2*wheelNotch)
	}
	if d := x.b.Now() - t0; d > 400*time.Millisecond {
		t.Errorf("the animations took %v", d)
	}
}

func TestTouchpadDeltaAndFling(t *testing.T) {
	x := newScrollHost(t, listRoot(""), 300, 300, Config{})
	// Precise deltas follow the fingers at once.
	x.wheel(touchpad(100, 100, 12, platform.ScrollPhaseUpdate))
	if got := x.offset("list"); got != 12 {
		t.Fatalf("touchpad delta: %g, want 12", got)
	}
	for range 5 {
		x.b.Advance(10 * time.Millisecond)
		x.wheel(touchpad(100, 100, 12, platform.ScrollPhaseUpdate))
	}
	if got := x.offset("list"); got != 72 {
		t.Fatalf("after the gesture: %g, want 72", got)
	}
	// The fingers lift: a fling at about 1200 px/s decelerates.
	x.b.Advance(5 * time.Millisecond)
	x.wheel(touchpad(100, 100, 0, platform.ScrollPhaseEnd))
	s := scr(t, x.tree(), "list")
	if !s.fling[1].active || s.fling[1].v0 < 1000 || s.fling[1].v0 > 1500 {
		t.Fatalf("fling %+v", s.fling[1])
	}
	x.frame()
	a := x.offset("list")
	x.frame()
	b := x.offset("list")
	x.frame()
	c := x.offset("list")
	if !(a > 72 && b > a && c > b && c-b < b-a) {
		t.Errorf("fling not decelerating: %g %g %g", a, b, c)
	}
	x.idle()
	// v0 / friction: about 300 px more, within the range.
	if got := x.offset("list"); got < 72+200 || got > 72+400 {
		t.Errorf("fling ended at %g", got)
	}

	// Without the end of the gesture (X11), nothing flings.
	x2 := newScrollHost(t, listRoot(""), 300, 300, Config{})
	for range 6 {
		x2.b.Advance(10 * time.Millisecond)
		x2.wheel(touchpad(100, 100, 12, platform.ScrollPhaseNone))
	}
	x2.settle()
	if got := x2.offset("list"); got != 72 {
		t.Errorf("phaseless gesture: %g, want 72 (no fling)", got)
	}
	// Fingers that rested before lifting do not fling.
	x3 := newScrollHost(t, listRoot(""), 300, 300, Config{})
	for range 6 {
		x3.b.Advance(10 * time.Millisecond)
		x3.wheel(touchpad(100, 100, 12, platform.ScrollPhaseUpdate))
	}
	x3.b.Advance(400 * time.Millisecond)
	x3.wheel(touchpad(100, 100, 0, platform.ScrollPhaseEnd))
	x3.settle()
	if got := x3.offset("list"); got != 72 {
		t.Errorf("rested fingers: %g, want 72", got)
	}
}

func TestInstantSettings(t *testing.T) {
	fling := func(x *inputHarness) {
		for range 6 {
			x.b.Advance(10 * time.Millisecond)
			x.wheel(touchpad(100, 100, 12, platform.ScrollPhaseUpdate))
		}
		x.b.Advance(5 * time.Millisecond)
		x.wheel(touchpad(100, 100, 0, platform.ScrollPhaseEnd))
	}
	for _, tc := range []struct {
		name     string
		classes  string
		cfg      Config
		reduced  bool
		animated bool
	}{
		{"default", "", Config{}, false, true},
		{"InstantScroll", "", Config{InstantScroll: true}, false, false},
		{"scroll-instant", "scroll-instant", Config{}, false, false},
		{"motion-reduce", "", Config{}, true, false},
		{"scroll-smooth overrides InstantScroll", "scroll-smooth", Config{InstantScroll: true}, false, true},
		{"motion-reduce beats scroll-smooth", "scroll-smooth", Config{}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			x := newScrollHost(t, listRoot(tc.classes), 300, 300, tc.cfg)
			if tc.reduced {
				x.b.SetSettings(platform.SystemSettings{ReducedMotion: true})
				x.b.Drain(x.h)
				x.settle()
			}
			x.wheel(notch(100, 100, 1))
			if jumped := x.offset("list") == wheelNotch; jumped == tc.animated {
				t.Errorf("notch jumped %v, want animated %v", jumped, tc.animated)
			}
			x.settle()
			fling(x)
			x.settle()
			flung := x.offset("list") > wheelNotch+72
			if flung != tc.animated {
				t.Errorf("flung %v (at %g), want %v", flung, x.offset("list"), tc.animated)
			}
		})
	}
}

func TestShiftWheelScrollsHorizontally(t *testing.T) {
	root := div(Key("h"), Class("h-[100px] w-[100px] overflow-auto"), div(Class("h-[300px] w-[400px]")))
	x := newScrollHost(t, root, 300, 300, Config{InstantScroll: true})
	e := notch(50, 50, 1)
	e.Mods = platform.ModShift
	x.wheel(e)
	if x.offsetX("h") != wheelNotch || x.offset("h") != 0 {
		t.Errorf("shift+wheel: %g,%g", x.offsetX("h"), x.offset("h"))
	}
	// A touchpad's horizontal delta is used as is.
	x.wheel(platform.ScrollEvent{Pos: platform.Point{X: 50, Y: 50}, Delta: platform.Point{X: 10}})
	if x.offsetX("h") != wheelNotch+10 {
		t.Errorf("horizontal delta: %g", x.offsetX("h"))
	}
	// Ctrl+wheel does not scroll (browsers zoom).
	e = notch(50, 50, 1)
	e.Mods = platform.ModCtrl
	x.wheel(e)
	if x.offset("h") != 0 {
		t.Errorf("ctrl+wheel scrolled to %g", x.offset("h"))
	}
}

func TestWheelEventAndPreventDefault(t *testing.T) {
	var got []string
	prevent := false
	root := div(Key("outer"), Class("p-0"),
		On("wheel", func(e WheelEvent) {
			got = append(got, fmt.Sprintf("outer %g,%g local %g,%g", e.DeltaX, e.DeltaY, e.LocalX, e.LocalY))
		}),
		div(Key("list"), Class("ml-[10px] h-[200px] w-[200px] overflow-y-auto"),
			On("wheel", func(e WheelEvent) {
				got = append(got, fmt.Sprintf("list %g,%g local %g,%g", e.DeltaX, e.DeltaY, e.LocalX, e.LocalY))
				if prevent {
					e.PreventDefault()
				}
			}),
			div(Class("h-[1000px]"))),
	)
	x := newScrollHost(t, root, 300, 300, Config{InstantScroll: true})
	x.wheel(notch(60, 40, 1))
	if s := strings.Join(got, "; "); s != "list 0,53 local 50,40; outer 0,53 local 60,40" {
		t.Errorf("events: %s", s)
	}
	if x.offset("list") != wheelNotch {
		t.Errorf("default scroll: %g", x.offset("list"))
	}
	prevent = true
	x.b.Advance(time.Second)
	x.wheel(notch(60, 40, 1))
	if x.offset("list") != wheelNotch {
		t.Errorf("prevented wheel scrolled to %g", x.offset("list"))
	}
}

func TestScrollChaining(t *testing.T) {
	inner := func(classes string) Node {
		return div(Key("outer"), Class("h-[200px] w-[200px] overflow-y-auto"),
			div(Key("inner"), Class("h-[100px] overflow-y-auto "+classes), div(Class("h-[150px]"))),
			div(Class("h-[400px]")),
		)
	}
	x := newScrollHost(t, inner(""), 300, 300, Config{InstantScroll: true})
	x.wheel(notch(50, 50, 1)) // inner: 50 of range, takes it all
	if x.offset("inner") != 50 || x.offset("outer") != 0 {
		t.Fatalf("first notch: inner %g outer %g", x.offset("inner"), x.offset("outer"))
	}
	// Latched: the next notch of the burst does not chain.
	x.b.Advance(100 * time.Millisecond)
	x.wheel(notch(50, 50, 1))
	if x.offset("outer") != 0 {
		t.Errorf("chained inside a wheel transaction: outer %g", x.offset("outer"))
	}
	// A new transaction (after a pause) chains to the outer scroller.
	x.b.Advance(time.Second)
	x.wheel(notch(50, 50, 1))
	if x.offset("outer") != wheelNotch {
		t.Errorf("chain: outer %g, want %d", x.offset("outer"), wheelNotch)
	}
	// So does a moved pointer.
	x.send(move(52, 50))
	x.wheel(notch(52, 50, 1))
	if x.offset("outer") != 2*wheelNotch {
		t.Errorf("chain after a move: outer %g", x.offset("outer"))
	}

	// overscroll-contain stops the chain.
	x = newScrollHost(t, inner("overscroll-contain"), 300, 300, Config{InstantScroll: true})
	x.wheel(notch(50, 50, 1))
	x.b.Advance(time.Second)
	x.wheel(notch(50, 50, 1))
	if x.offset("inner") != 50 || x.offset("outer") != 0 {
		t.Errorf("contained: inner %g outer %g", x.offset("inner"), x.offset("outer"))
	}
}

// ---------------------------------------------------------------------
// Geometry.

func TestScrolledHitTestingAndLocal(t *testing.T) {
	var local []float32
	root := div(Class("p-0"),
		div(Key("list"), Class("relative ml-[20px] mt-[30px] h-[200px] w-[200px] overflow-y-auto"),
			div(Key("r0"), Class("h-[50px]")), div(Key("r1"), Class("h-[50px]")),
			div(Key("r2"), Class("h-[50px]"), On("click", func(e MouseEvent) { local = append(local, e.LocalX, e.LocalY) })),
			div(Class("h-[500px]")),
			// Scrolls with the list: its containing block is the list.
			div(Key("abs"), Class("absolute left-0 top-[400px] h-[20px] w-[20px]")),
		),
		// Escapes: its containing block is the root.
		div(Class("h-0"), div(Key("fixedish"), Class("absolute left-[300px] top-[40px] h-[20px] w-[20px]"))),
	)
	x := newScrollHost(t, root, 400, 400, Config{InstantScroll: true})
	tr := x.tree()
	x.wheel(notch(50, 50, 2)) // 106
	x.settle()
	// (25, 35) is 5,5 into the list's padding box: content y 111 → r2.
	if got := hitName(tr, 25, 35); got != "r2" {
		t.Errorf("hit at scrolled point: %s, want r2", got)
	}
	x.click(25, 35)
	if len(local) != 2 || local[0] != 5 || local[1] != 111-100 {
		t.Errorf("local %v, want [5 11]", local)
	}
	if r := windowRect(keyed(tr, "abs").box); r.Y0 != 30+400-106 {
		t.Errorf("absolute descendant at %v, want y %d", r, 30+400-106)
	}
	if r := windowRect(keyed(tr, "fixedish").box); r.Y0 != 40 {
		t.Errorf("escaping absolute box moved to %v", r)
	}
	// The clip still cuts content scrolled out.
	if got := hitName(tr, 25, 25); got != "root" && got != "<div>" {
		t.Errorf("hit above the list: %s", got)
	}
}

func TestHoverFollowsScrolledContent(t *testing.T) {
	var r recorder
	c := []Child{Key("list"), Class("h-[200px] w-[200px] overflow-y-auto")}
	for i := range 10 {
		k := fmt.Sprintf("r%d", i)
		c = append(c, div(Key(k), Class("h-[50px] hover:bg-sky-500"), r.mouse("mouseenter", k), r.mouse("mouseleave", k)))
	}
	x := newScrollHost(t, div(c...), 300, 300, Config{InstantScroll: true})
	x.send(move(50, 20))
	r.reset()
	x.wheel(notch(50, 20, 1)) // 53: r1 is now under (50, 20)
	x.settle()
	if got := r.String(); got != "mouseleave:r0 mouseenter:r1" {
		t.Errorf("after scrolling: %q", got)
	}
	if keyed(x.tree(), "r0").istate&1 != 0 || keyed(x.tree(), "r1").hoverN != 1 {
		t.Error("hover states did not follow")
	}
}

// ---------------------------------------------------------------------
// Keyboard and focus.

func TestKeyboardScrolling(t *testing.T) {
	root := div(Class("p-0"),
		div(Key("list"), TabIndex(0), Class("h-[200px] w-[200px] overflow-auto"),
			div(Class("h-[1000px] w-[300px]")),
			button(Key("btn"), Class("h-[20px] w-[50px]"))),
	)
	x := newKeys(t, root, 300, 300)
	// No focus: the outermost scroller.
	x.press(platform.KeyArrowDown, platform.CodeArrowDown, 0)
	x.settle()
	if got := x.offset("list"); got != lineStep {
		t.Errorf("ArrowDown without focus: %g", got)
	}
	x.tab() // the list
	steps := []struct {
		k     platform.Key
		c     platform.KeyCode
		mods  platform.Modifiers
		want  float32
		wantX float32
	}{
		{platform.KeyArrowDown, platform.CodeArrowDown, 0, 2 * lineStep, 0},
		{platform.KeyArrowUp, platform.CodeArrowUp, 0, lineStep, 0},
		{platform.KeyPageDown, platform.CodePageDown, 0, lineStep + 175, 0}, // 87.5% of 200
		{platform.KeyPageUp, platform.CodePageUp, 0, lineStep, 0},
		{platform.KeyEnd, platform.CodeEnd, 0, 1020 - 200, 0},
		{platform.KeyHome, platform.CodeHome, platform.ModCtrl, 0, 0},
		{" ", platform.CodeSpace, 0, 175, 0},
		{" ", platform.CodeSpace, platform.ModShift, 0, 0},
		{platform.KeyArrowRight, platform.CodeArrowRight, 0, 0, lineStep},
		{platform.KeyArrowDown, platform.CodeArrowDown, platform.ModAlt, 0, lineStep}, // Alt: nothing
	}
	for _, st := range steps {
		x.press(st.k, st.c, st.mods)
		x.settle()
		if got, gx := x.offset("list"), x.offsetX("list"); got != st.want || gx != st.wantX {
			t.Errorf("%v %v: %g,%g, want %g,%g", st.mods, st.k, gx, got, st.wantX, st.want)
		}
	}
	// Space on a focused button clicks it instead of scrolling; arrows
	// scroll its scroll container.
	x.tab()
	if focusedName(x.tree()) != "btn" {
		t.Fatalf("focus on %s", focusedName(x.tree()))
	}
	x.settle()
	at := x.offset("list")
	x.space()
	x.settle()
	if got := x.offset("list"); got != at {
		t.Errorf("Space on a button scrolled %g -> %g", at, got)
	}
	x.press(platform.KeyArrowUp, platform.CodeArrowUp, 0)
	x.settle()
	if got := x.offset("list"); got != at-lineStep {
		t.Errorf("ArrowUp on a button: %g, want %g", got, at-lineStep)
	}
}

func TestFocusRevealsElement(t *testing.T) {
	c := []Child{Key("list"), Class("h-[200px] w-[200px] overflow-y-auto")}
	for i := range 10 {
		c = append(c, button(Key(fmt.Sprintf("b%d", i)), Class("block h-[50px] w-full")))
	}
	x := newKeys(t, div(c...), 300, 300)
	for range 5 { // b0..b4
		x.tab()
	}
	x.settle()
	// b4 (200..250) is aligned with the bottom edge (nearest).
	if got := x.offset("list"); got != 50 {
		t.Errorf("after Tab to b4: %g, want 50", got)
	}
	x.shiftTab() // b3 (150..200): visible, no scroll
	x.settle()
	if got := x.offset("list"); got != 50 {
		t.Errorf("after Shift+Tab to b3: %g, want 50", got)
	}
	// A mouse focus does not scroll; a programmatic one does.
	x.click(50, 10) // b1 (50..100) at offset 50: 0..50 on screen
	x.settle()
	if focusedName(x.tree()) != "b1" || x.offset("list") != 50 {
		t.Errorf("mouse focus %s, offset %g", focusedName(x.tree()), x.offset("list"))
	}
	x.tree().focusElement(keyed(x.tree(), "b9"))
	x.settle()
	if got := x.offset("list"); got != 300 {
		t.Errorf("programmatic focus of b9: %g, want 300", got)
	}
}

// ---------------------------------------------------------------------
// ElementRef and UseScroll.

func TestElementRefScrolling(t *testing.T) {
	var outer, inner, item, plain ElementRef
	comp := func() Node {
		outer, inner, item, plain = UseElementRef(), UseElementRef(), UseElementRef(), UseElementRef()
		return div(AttachRef(outer), Key("outer"), Class("h-[200px] w-[200px] overflow-auto"),
			div(Class("h-[300px]")),
			div(AttachRef(inner), Key("inner"), Class("h-[100px] w-[100px] overflow-auto"),
				div(Class("h-[300px]")),
				div(AttachRef(item), Key("item"), Class("h-[20px] w-[20px]")),
				div(Class("h-[100px]"))),
			div(AttachRef(plain), Class("h-[300px]")),
		)
	}
	x := newScrollHost(t, Component(comp), 400, 400, Config{})
	item.ScrollIntoView()
	x.settle()
	// item: 300..320 in inner (client 100): inner scrolls 220; inner sits
	// at 300..400 in outer (client 200): outer scrolls 200.
	if a, b := x.offset("inner"), x.offset("outer"); a != 220 || b != 200 {
		t.Errorf("ScrollIntoView: inner %g, outer %g; want 220, 200", a, b)
	}
	if r := windowRect(keyed(x.tree(), "item").box); r.Y0 < 0 || r.Y1 > 200 {
		t.Errorf("item painted at %v", r)
	}
	outer.ScrollBy(0, -50)
	if _, y := outer.ScrollOffset(); y != 150 {
		t.Errorf("ScrollBy: %g", y)
	}
	outer.ScrollTo(0, 0)
	if _, y := outer.ScrollOffset(); y != 0 {
		t.Errorf("ScrollTo: %g", y)
	}
	// Not a scroll container, or unmounted: nothing.
	plain.ScrollTo(0, 100)
	if x0, y0 := plain.ScrollOffset(); x0 != 0 || y0 != 0 {
		t.Errorf("plain element offset %g,%g", x0, y0)
	}
	var zero ElementRef
	zero.ScrollTo(1, 1)
	zero.ScrollBy(1, 1)
	zero.ScrollIntoView()
	if x0, y0 := zero.ScrollOffset(); x0 != 0 || y0 != 0 {
		t.Error("zero ref has an offset")
	}
}

func TestScrollSmoothProgrammatic(t *testing.T) {
	var ref ElementRef
	comp := func() Node {
		ref = UseElementRef()
		c := []Child{AttachRef(ref), Key("list"), Class("h-[200px] w-[200px] overflow-y-auto scroll-smooth")}
		for _, r := range rows(20) {
			c = append(c, r)
		}
		return div(c...)
	}
	x := newScrollHost(t, Component(comp), 300, 300, Config{})
	ref.ScrollTo(0, 400)
	if _, y := ref.ScrollOffset(); y != 0 {
		t.Errorf("smooth ScrollTo jumped to %g", y)
	}
	x.frame()
	x.frame()
	if _, y := ref.ScrollOffset(); y <= 0 || y >= 400 {
		t.Errorf("smooth ScrollTo after two frames: %g", y)
	}
	x.settle()
	if _, y := ref.ScrollOffset(); y != 400 {
		t.Errorf("smooth ScrollTo ended at %g", y)
	}
}

func TestUseScrollDrivesPaintOnly(t *testing.T) {
	var opacities []float32
	var ref ElementRef
	comp := func() Node {
		ref = UseElementRef()
		_, y := UseScroll(ref)
		c := []Child{AttachRef(ref), Key("list"), Class("h-[200px] w-[200px] overflow-y-auto")}
		for _, r := range rows(20) {
			c = append(c, r)
		}
		return div(
			div(Key("header"), Class("h-[20px] bg-sky-500"), Style(style.OpacityFn(func() float32 {
				o := 1 - min(y.Get()/200, 1)
				opacities = append(opacities, o)
				return o
			}))),
			div(c...),
		)
	}
	x := newScrollHost(t, Component(comp), 300, 300, Config{InstantScroll: true})
	s0 := x.tree().Stats()
	x.wheel(notch(50, 50, 1))
	x.settle()
	w := work(s0, x.tree().Stats())
	if w.Builds != 0 || w.Layouts != 0 || w.Styles != 0 || w.Paints < 1 {
		t.Errorf("scroll work %+v, want paints only", w)
	}
	if got := opacities[len(opacities)-1]; !approx(got, 1-53.0/200) {
		t.Errorf("opacity %g, want %g", got, 1-53.0/200)
	}
	ref.ScrollTo(0, 300)
	x.settle()
	if got := opacities[len(opacities)-1]; got != 0 {
		t.Errorf("opacity after ScrollTo: %g", got)
	}
}

func TestUseScrollFollowsTheRef(t *testing.T) {
	var y *Value[float32]
	var setOnB func(bool)
	comp := func() Node {
		r := UseElementRef()
		_, y = UseScroll(r)
		onB, set := UseStateSet(false)
		setOnB = set
		a := []Child{Key("a"), Class("h-[100px] w-[100px] overflow-y-auto"), div(Class("h-[400px]"))}
		b := []Child{Key("b"), Class("h-[100px] w-[100px] overflow-y-auto"), div(Class("h-[400px]"))}
		if onB {
			b = append(b, AttachRef(r))
		} else {
			a = append(a, AttachRef(r))
		}
		return div(Class("flex"), div(a...), div(b...))
	}
	x := newScrollHost(t, Component(comp), 300, 300, Config{InstantScroll: true})
	x.wheel(notch(50, 50, 1)) // a: 53
	x.send(move(150, 50))
	x.wheel(notch(150, 50, 2)) // b: 106
	x.settle()
	if got := y.Get(); got != wheelNotch {
		t.Errorf("following a: %g", got)
	}
	setOnB(true)
	x.settle()
	if got := y.Get(); got != 2*wheelNotch {
		t.Errorf("following b: %g, want %d", got, 2*wheelNotch)
	}
	x.b.Advance(time.Second)
	x.wheel(notch(50, 50, 1)) // a moves: not followed any more
	x.settle()
	if got := y.Get(); got != 2*wheelNotch {
		t.Errorf("a's scroll leaked into b's value: %g", got)
	}
}

func TestUseScrollHookKind(t *testing.T) {
	// UseScroll is its own kind in the dev-mode order check.
	scroll := true
	comp := func() Node {
		r := UseElementRef()
		if scroll {
			UseScroll(r)
		} else {
			UseValue(float32(0))
		}
		return div(AttachRef(r))
	}
	tr := devTree(t, Component(comp), nil)
	scroll = false
	msg := panicMessage(func() { tr.Mount(Component(comp)) })
	if !strings.Contains(msg, "hook 2 is UseValue") || !strings.Contains(msg, "but it was UseScroll") {
		t.Errorf("panic %q", msg)
	}
	msg = panicMessage(func() { UseScroll(ElementRef{}) })
	if !strings.Contains(msg, "UseScroll(zero ElementRef)") {
		t.Errorf("zero ref: %q", msg)
	}
}

func TestOnScrollOncePerFrame(t *testing.T) {
	var got []ScrollEvent
	root := div(Key("list"), Class("h-[200px] w-[200px] overflow-y-auto p-2"),
		On("scroll", func(e ScrollEvent) { got = append(got, e) }),
		div(Class("h-[1000px]")))
	x := newScrollHost(t, root, 300, 300, Config{InstantScroll: true})
	// Two notches before the next frame: one event, with the final offset.
	x.w.Inject(notch(50, 50, 1))
	x.w.Inject(notch(50, 50, 1))
	x.b.Drain(x.h)
	x.settle()
	if len(got) != 1 {
		t.Fatalf("%d events, want 1: %+v", len(got), got)
	}
	want := ScrollEvent{ScrollTop: 106, ScrollWidth: 200, ScrollHeight: 1016, ClientWidth: 200, ClientHeight: 200}
	e := got[0]
	e.st = nil
	if e != want {
		t.Errorf("event %+v, want %+v", e, want)
	}
	// An animated scroll fires once per frame while it moves.
	x2 := newScrollHost(t, root, 300, 300, Config{})
	got = got[:0]
	x2.wheel(notch(50, 50, 1))
	frames := 0
	for x2.w.FrameRequested() && frames < 100 {
		x2.frame()
		frames++
	}
	if len(got) < 5 || len(got) > frames {
		t.Errorf("%d events in %d frames", len(got), frames)
	}
	if last := got[len(got)-1]; last.ScrollTop != wheelNotch {
		t.Errorf("last event at %g", last.ScrollTop)
	}
}

// ---------------------------------------------------------------------
// Scrollbars.

func TestOverlayBarVisibilityAndFade(t *testing.T) {
	x := newScrollHost(t, listRoot(""), 300, 300, Config{InstantScroll: true})
	s := scr(t, x.tree(), "list")
	if s.bars.alpha != 0 || s.barVisible(1) {
		t.Fatal("overlay bar visible before any scroll")
	}
	t0 := x.b.Now()
	x.wheel(notch(100, 100, 1))
	x.settle()
	if s.bars.alpha != 1 || !s.barVisible(1) {
		t.Fatalf("bar not shown while scrolling: %+v", s.bars)
	}
	at, ok := x.h.TimerArmed()
	if !ok || at != t0+overlayHold {
		t.Fatalf("fade timer %v %v, want %v", at, ok, t0+overlayHold)
	}
	// Nothing happens until the hold ends.
	x.b.Advance(overlayHold - time.Millisecond)
	if x.w.FrameRequested() {
		t.Error("frames during the hold")
	}
	x.b.Advance(2 * time.Millisecond)
	x.b.Wake()
	x.b.Drain(x.h)
	x.frame()
	x.frame()
	if s.bars.alpha <= 0 || s.bars.alpha >= 1 {
		t.Errorf("fading alpha %g", s.bars.alpha)
	}
	x.idle()
	if s.bars.alpha != 0 || s.barVisible(1) {
		t.Errorf("bar still visible: %+v", s.bars)
	}
	if d := x.b.Now() - t0; d > overlayHold+overlayFade+100*time.Millisecond {
		t.Errorf("faded after %v", d)
	}
	// A mouse near the edge shows the bars; over the bar it grows.
	x.send(move(190, 100))
	x.settle()
	if s.bars.alpha != 1 || !s.bars.near[1] {
		t.Errorf("near the edge: %+v", s.bars)
	}
	if !s.bars.over[1] || s.geo[1].thumb.Width() != overlayThick {
		t.Errorf("over the bar: %+v width %g", s.bars, s.geo[1].thumb.Width())
	}
	x.send(move(50, 100))
	x.settle()
	if s.geo[1].thumb.Width() != overlayThin {
		t.Errorf("thumb width away from the bar: %g", s.geo[1].thumb.Width())
	}
	x.idle()
}

func TestThumbDragAndTrackPaging(t *testing.T) {
	var clicks int
	root := div(Class("p-0"),
		div(Key("list"), Class("h-[200px] w-[200px] overflow-y-auto"), On("click", func(MouseEvent) { clicks++ }),
			div(Class("h-[1000px]"))))
	x := newScrollHost(t, root, 300, 300, Config{ClassicScrollbars: true, InstantScroll: true})
	s := scr(t, x.tree(), "list")
	g := s.geo[1]
	if !g.ok || g.thumb.Empty() {
		t.Fatalf("no classic bar: %+v", g)
	}
	// The thumb: 196 px of lane, 200/1000 of it long.
	if !approx(g.thumbLen, 196*200/1000.0+0.0) && g.thumbLen != minThumb {
		t.Errorf("thumb length %g", g.thumbLen)
	}
	travel := g.length - g.thumbLen
	cx, cy := g.thumb.Center().X, g.thumb.Center().Y
	x.send(move(cx, cy), down(cx, cy), moveHeld(cx, cy+10))
	want := float32(10 * float64(s.max.Y) / float64(travel))
	if got := x.offset("list"); !approx(got, want) {
		t.Errorf("thumb drag 10 px: %g, want %g", got, want)
	}
	x.send(up(cx, cy+10))
	x.settle()
	if clicks != 0 {
		t.Errorf("a thumb drag clicked %d times", clicks)
	}
	// A press on the track below the thumb pages down.
	at := x.offset("list")
	x.click(cx, 190)
	x.settle()
	if got := x.offset("list"); got != at+175 {
		t.Errorf("track page: %g, want %g", got, at+175)
	}
	if clicks != 0 {
		t.Errorf("a track press clicked")
	}
	// Above the thumb pages up.
	x.b.Advance(time.Second)
	x.click(cx, 2)
	x.settle()
	if got := x.offset("list"); !approx(got, at) {
		t.Errorf("track page up: %g, want %g", got, at)
	}
}

func TestOverlayBarHitPriority(t *testing.T) {
	var clicks []string
	c := []Child{Key("list"), Class("h-[200px] w-[200px] overflow-y-auto")}
	for i := range 20 {
		k := fmt.Sprintf("r%d", i)
		c = append(c, div(Key(k), Class("h-[50px] w-full"), On("click", func(MouseEvent) { clicks = append(clicks, k) })))
	}
	x := newScrollHost(t, div(c...), 300, 300, Config{InstantScroll: true})
	// Hidden bars do not take hits (a mouse moving there shows them).
	if got := hitName(x.tree(), 195, 20); got != "r0" {
		t.Fatalf("hit under a hidden bar: %s, want r0", got)
	}
	x.wheel(notch(100, 100, 1))
	x.settle()
	s := scr(t, x.tree(), "list")
	g := s.geo[1]
	if !s.barVisible(1) || g.thumb.Empty() {
		t.Fatalf("no visible overlay thumb: %+v", g)
	}
	// The thumb takes the press before the row under it, and drags.
	cx, cy := g.thumb.Center().X, g.thumb.Center().Y
	if got := hitName(x.tree(), cx, cy); got != "list" {
		t.Errorf("hit on the thumb: %s, want list", got)
	}
	at := x.offset("list")
	x.send(move(cx, cy), down(cx, cy), moveHeld(cx, cy+20), up(cx, cy+20))
	x.settle()
	if len(clicks) != 0 {
		t.Errorf("a press on the thumb clicked %v", clicks)
	}
	if got := x.offset("list"); got <= at {
		t.Errorf("thumb drag: %g, was %g", got, at)
	}
	// The track below the thumb pages.
	at = x.offset("list")
	g = s.geo[1]
	x.b.Advance(time.Second)
	x.click(cx, g.track.Y1-2)
	x.settle()
	if got := x.offset("list"); got != min(at+175, s.max.Y) || len(clicks) != 0 {
		t.Errorf("overlay track page: %g (from %g), clicks %v", got, at, clicks)
	}
}

func TestClassicGutters(t *testing.T) {
	content := func(h string) Node { return div(Key("c"), Class(h)) }
	cases := []struct {
		classes, content string
		cfg              Config
		width            float32
	}{
		{"overflow-y-auto", "h-[1000px]", Config{ClassicScrollbars: true}, 200 - classicGutter},
		{"overflow-y-auto", "h-[100px]", Config{ClassicScrollbars: true}, 200},
		{"overflow-y-scroll", "h-[100px]", Config{ClassicScrollbars: true}, 200 - classicGutter},
		{"overflow-y-auto scrollbar-classic", "h-[1000px]", Config{}, 200 - classicGutter},
		{"overflow-y-auto scrollbar-overlay", "h-[1000px]", Config{ClassicScrollbars: true}, 200},
		{"overflow-y-scroll", "h-[1000px]", Config{}, 200}, // overlay bars take no space
	}
	for _, tc := range cases {
		t.Run(tc.classes+"/"+tc.content, func(t *testing.T) {
			root := div(Key("list"), Class("h-[200px] w-[200px] "+tc.classes), content(tc.content))
			x := newScrollHost(t, root, 300, 300, tc.cfg)
			if got := mustFind(t, x.tree(), "c").Rect.Width(); got != tc.width {
				t.Errorf("content width %g, want %g", got, tc.width)
			}
		})
	}
	// A list that grows past its height gets the gutter, and loses it
	// when it shrinks again.
	var setH func(string)
	comp := func() Node {
		h, set := UseStateSet("h-[100px]")
		setH = set
		return div(Key("list"), Class("h-[200px] w-[200px] overflow-y-auto"), content(h))
	}
	x := newScrollHost(t, Component(comp), 300, 300, Config{ClassicScrollbars: true})
	setH("h-[500px]")
	x.settle()
	if got := mustFind(t, x.tree(), "c").Rect.Width(); got != 200-classicGutter {
		t.Errorf("after growing: %g", got)
	}
	if s := scr(t, x.tree(), "list"); s.client.W != 200-classicGutter || s.max.Y != 300 {
		t.Errorf("client %v max %v", s.client, s.max)
	}
	setH("h-[50px]")
	x.settle()
	if got := mustFind(t, x.tree(), "c").Rect.Width(); got != 200 {
		t.Errorf("after shrinking: %g", got)
	}
}

func TestTouchPanAndFling(t *testing.T) {
	var clicks int
	root := div(Key("list"), Class("h-[200px] w-[200px] overflow-y-auto"), On("click", func(MouseEvent) { clicks++ }),
		div(Class("h-[1000px]")))
	x := newScrollHost(t, root, 300, 300, Config{})
	touch := func(a platform.PointerAction, py float32) platform.PointerEvent {
		e := ptr(a, 100, py, platform.ButtonNone, 0)
		e.Kind = platform.PointerTouch
		if a == platform.PointerDown {
			e.Button, e.Buttons = platform.ButtonPrimary, platform.ButtonPrimary.Mask()
		}
		if a == platform.PointerMove {
			e.Buttons = platform.ButtonPrimary.Mask()
		}
		if a == platform.PointerUp {
			e.Button = platform.ButtonPrimary
		}
		return e
	}
	x.send(touch(platform.PointerDown, 150))
	y := float32(150)
	for range 8 {
		x.b.Advance(10 * time.Millisecond)
		y -= 15
		x.send(touch(platform.PointerMove, y))
	}
	// Past the touch slop the content follows the finger (the moves after
	// the one that won).
	if got := x.offset("list"); got < 60 || got > 120 {
		t.Errorf("panned to %g", got)
	}
	at := x.offset("list")
	x.send(touch(platform.PointerUp, y))
	x.settle()
	if clicks != 0 {
		t.Error("a pan clicked")
	}
	if got := x.offset("list"); got <= at+50 {
		t.Errorf("no fling after the release: %g (at release %g)", got, at)
	}
}

func TestScrollIdle(t *testing.T) {
	x := newScrollHost(t, listRoot(""), 300, 300, Config{})
	x.wheel(notch(100, 100, 3))
	frames := x.tree().Stats().Frames
	x.idle()
	if x.tree().Stats().Frames-frames > 40 {
		t.Errorf("%d frames for a notch and a fade", x.tree().Stats().Frames-frames)
	}
	frames = x.tree().Stats().Frames
	x.b.Advance(5 * time.Second)
	x.b.Drain(x.h)
	if x.tree().Stats().Frames != frames || x.w.FrameRequested() {
		t.Error("frames while idle")
	}
}

// ---------------------------------------------------------------------
// The viewport.

// page is a page of 30 rows of 50 px (1500 px) with a fixed header and an
// absolute badge, in a window of 300x300.
func page() Node {
	c := []Child{Key("page")}
	for _, r := range rows(30) {
		c = append(c, r)
	}
	c = append(c,
		div(Key("fixed"), Class("fixed left-0 top-0 h-[10px] w-[10px]")),
		div(Key("abs"), Class("absolute left-[20px] top-[100px] h-[10px] w-[10px]")),
		div(Key("screen"), Class("h-screen")))
	return div(c...)
}

func TestViewportScrolls(t *testing.T) {
	x := newKeys(t, page(), 300, 300)
	tr := x.tree()
	vs := tr.rootBox.scr
	if vs == nil || !vs.userX || !vs.userY {
		t.Fatal("the root is not a scroll container")
	}
	// h-screen resolves against the window height; the page is 30 rows
	// + 300 tall.
	if h := mustFind(t, tr, "screen").Rect.Height(); h != 300 {
		t.Errorf("h-screen on the page: %g, want 300", h)
	}
	if vs.max.Y != 1500+300-300 || vs.max.X != 0 {
		t.Fatalf("viewport range %v", vs.max)
	}
	// The wheel, anywhere on the page.
	x.wheel(notch(150, 150, 1))
	x.settle()
	if vs.pos.Y != wheelNotch {
		t.Errorf("wheel: %g", vs.pos.Y)
	}
	if r := windowRect(keyed(tr, "r0").box); r.Y0 != -wheelNotch {
		t.Errorf("row 0 painted at %v", r)
	}
	// Fixed boxes stay; absolute boxes of the page scroll with it.
	if r := windowRect(keyed(tr, "fixed").box); r.Y0 != 0 {
		t.Errorf("fixed box moved to %v", r)
	}
	if r := windowRect(keyed(tr, "abs").box); r.Y0 != 100-wheelNotch {
		t.Errorf("absolute box at %v", r)
	}
	if got := hitName(tr, 5, 5); got != "fixed" {
		t.Errorf("hit on the fixed box: %s", got)
	}
	// Keys with nothing focused scroll the viewport.
	x.press(platform.KeyPageDown, platform.CodePageDown, 0)
	x.settle()
	if vs.pos.Y != wheelNotch+262.5 {
		t.Errorf("PageDown: %g, want %g", vs.pos.Y, wheelNotch+262.5)
	}
	x.press(platform.KeyHome, platform.CodeHome, 0)
	x.settle()
	if vs.pos.Y != 0 {
		t.Errorf("Home: %g", vs.pos.Y)
	}
	// ScrollIntoView reaches the viewport as the last container.
	tr.scrollIntoView(keyed(tr, "r20"))
	x.settle()
	if vs.pos.Y != 1050-300 {
		t.Errorf("ScrollIntoView: %g, want 750", vs.pos.Y)
	}
}

func TestViewportHeightOfRootChild(t *testing.T) {
	// h-full on the root's child is the window height (the root keeps the
	// window's size, like html, body { height: 100% }); its overflowing
	// content still scrolls the viewport.
	root := div(Key("full"), Class("h-full"), div(Class("h-[900px]")))
	x := newScrollHost(t, root, 300, 300, Config{})
	if h := mustFind(t, x.tree(), "full").Rect.Height(); h != 300 {
		t.Errorf("h-full: %g, want 300", h)
	}
	if m := x.tree().rootBox.scr.max.Y; m != 600 {
		t.Errorf("viewport range %g, want 600", m)
	}
}

func TestViewportOverflowHidden(t *testing.T) {
	x := newScrollHost(t, page(), 300, 300, Config{RootClass: "overflow-hidden"})
	vs := x.tree().rootBox.scr
	if vs == nil || vs.userX || vs.userY {
		t.Fatal("overflow-hidden: the viewport is not a programmatic-only scroller")
	}
	x.wheel(notch(150, 150, 1))
	x.settle()
	if vs.pos.Y != 0 {
		t.Errorf("wheel scrolled a hidden viewport to %g", vs.pos.Y)
	}
	x.tree().scrollIntoView(keyed(x.tree(), "r20"))
	if vs.pos.Y != 750 {
		t.Errorf("ScrollIntoView in a hidden viewport: %g", vs.pos.Y)
	}
}

func TestViewportFitsAndClassicGutter(t *testing.T) {
	short := func() Node { return div(Key("content"), Class("h-[100px]")) }
	for _, classic := range []bool{false, true} {
		x := newScrollHost(t, short(), 300, 300, Config{ClassicScrollbars: classic})
		vs := x.tree().rootBox.scr
		if vs.max != (gfx.Point{}) {
			t.Errorf("classic %v: a fitting page has range %v", classic, vs.max)
		}
		if w := mustFind(t, x.tree(), "content").Rect.Width(); w != 300 {
			t.Errorf("classic %v: fitting content width %g", classic, w)
		}
		x.wheel(notch(50, 50, 1))
		x.settle()
		if vs.pos != (gfx.Point{}) {
			t.Errorf("classic %v: a fitting page scrolled", classic)
		}
	}
	// A tall page in classic mode gets the gutter, and paints its bar.
	x := newScrollHost(t, page(), 300, 300, Config{ClassicScrollbars: true})
	if w := mustFind(t, x.tree(), "page").Rect.Width(); w != 300-classicGutter {
		t.Errorf("tall page width %g, want %d", w, 300-classicGutter)
	}
	if g := x.tree().rootBox.scr.geo[1]; !g.ok || g.thumb.Empty() || g.track.X1 != 300 {
		t.Errorf("viewport bar %+v", g)
	}
}

// ---------------------------------------------------------------------
// Goldens.

// scrollbarScene is three scroll containers: a list scrolled down, a
// two-axis scroller (its vertical bar hovered) and an overflow-y-scroll
// box whose content fits.
func scrollbarScene(list, both *ElementRef) Node {
	comp := func() Node {
		*list, *both = UseElementRef(), UseElementRef()
		items := make([]Child, 12)
		for i := range items {
			bg := "bg-white dark:bg-slate-800"
			if i%2 == 1 {
				bg = "bg-slate-50 dark:bg-slate-700"
			}
			items[i] = div(Key(i), Class("h-[32px] px-3 text-sm leading-8 text-slate-700 dark:text-slate-200 "+bg), Textf("Row %d", i+1))
		}
		return div(Class("flex h-screen gap-4 bg-slate-100 p-4 dark:bg-slate-900"),
			div(AttachRef(*list), Key("list"), Class("h-[160px] w-[140px] overflow-y-auto rounded-lg border border-slate-300 dark:border-slate-600"),
				Fragment(items...)),
			div(AttachRef(*both), Key("both"), Class("h-[160px] w-[160px] overflow-auto"),
				div(Class("h-[400px] w-[400px] bg-linear-to-br from-sky-200 to-indigo-300"))),
			div(Key("fits"), Class("h-[160px] w-[100px] overflow-y-scroll bg-white p-2 text-sm dark:bg-slate-800 dark:text-slate-200"),
				Text("Fits")),
		)
	}
	return Component(comp)
}

func TestScrollbarGoldens(t *testing.T) {
	for _, classic := range []bool{false, true} {
		for _, dark := range []bool{false, true} {
			for _, scale := range []float32{1, 2} {
				name := "scrollbars_overlay"
				if classic {
					name = "scrollbars_classic"
				}
				if dark {
					name += "_dark"
				}
				name += fmt.Sprintf("_x%g", scale)
				t.Run(name, func(t *testing.T) {
					var list, both ElementRef
					tr := NewTree(Config{Fonts: testFonts, Dark: dark, ClassicScrollbars: classic})
					tr.SetViewport(gfx.Size{W: 460, H: 200})
					tr.SetScale(scale)
					tr.Mount(scrollbarScene(&list, &both))
					tr.Frame()
					list.ScrollTo(0, 100)
					both.ScrollTo(60, 120)
					// The second scroller's vertical bar is hovered.
					scr(t, tr, "both").bars.over[1] = true
					scr(t, tr, "both").bars.thumb[1] = true
					dl, _ := tr.Frame()
					golden.Image(t, name, tr.Rasterize(dl, scale))
				})
			}
		}
	}
}
