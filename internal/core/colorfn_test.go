package core

import (
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/style"
)

// Tests of OnDone and Velocity (valueanim.go) and of the deferred colors
// BgFn and TextColorFn (colorfn.go).

func TestOnDone(t *testing.T) {
	x, vp := animValue[float32](t, 0, false)
	v := *vp
	calls := map[string]int{}
	done := func(name string) func() { return func() { calls[name]++ } }

	Animate(v, 10, OnDone(done("a")))
	x.settle()
	if v.Get() != 10 || calls["a"] != 1 {
		t.Fatalf("value %v, OnDone called %d times", v.Get(), calls["a"])
	}
	// Interrupted by another Animate: only the second one's runs.
	Animate(v, 20, OnDone(done("b")))
	x.runFor(50 * time.Millisecond)
	Animate(v, 30, Duration(100*time.Millisecond), OnDone(done("c")))
	x.settle()
	if calls["b"] != 0 || calls["c"] != 1 {
		t.Errorf("interrupted by Animate: %v", calls)
	}
	// Interrupted by Set and by Update.
	Animate(v, 40, OnDone(done("d")))
	x.runFor(30 * time.Millisecond)
	v.Set(1)
	Animate(v, 50, OnDone(done("e")))
	x.runFor(30 * time.Millisecond)
	v.Update(func(f float32) float32 { return f + 1 })
	x.settle()
	x.runFor(time.Second)
	if calls["d"] != 0 || calls["e"] != 0 {
		t.Errorf("interrupted by Set or Update: %v", calls)
	}
	// Already there and at rest: it ends at once.
	cur := v.Get()
	Animate(v, cur, OnDone(done("f")))
	x.settle()
	if calls["f"] != 1 {
		t.Errorf("a target already reached: %v", calls)
	}
	if !x.quiet() {
		t.Error("not idle")
	}
}

// TestOnDoneRetarget: for UseSpring, the OnDone of the build that set the
// target runs, and not the one of an interrupted motion.
func TestOnDoneRetarget(t *testing.T) {
	var set Setter[float32]
	var ended []float32
	root := Component(func() Node {
		target, s := UseState(float32(0))
		set = s
		v := UseSpring(float32(0), target, Stiffness(200), Damping(20), OnDone(func() { ended = append(ended, target) }))
		return div(Class("size-4"), Style(style.TranslateFn(func() (float32, float32) { return v.Get(), 0 })))
	})
	x := newHarness(t, root, false)
	set.Set(100)
	x.runFor(80 * time.Millisecond)
	set.Set(50)
	x.settle()
	if len(ended) != 1 || ended[0] != 50 {
		t.Errorf("OnDone ran for %v, want only the target 50", ended)
	}
}

func TestOnDoneUnmount(t *testing.T) {
	var setShow Setter[bool]
	calls := 0
	child := func() Node {
		v := UseSpring(float32(0), 100, OnDone(func() { calls++ }))
		return div(Class("size-4"), Style(style.TranslateFn(func() (float32, float32) { return v.Get(), 0 })))
	}
	root := Component(func() Node {
		show, s := UseState(false)
		setShow = s
		return div(If(show, Component(child)))
	})
	x := newHarness(t, root, false)
	setShow.Set(true)
	x.runFor(50 * time.Millisecond)
	setShow.Set(false)
	x.settle()
	x.runFor(time.Second)
	if calls != 0 {
		t.Errorf("OnDone ran %d times after the unmount", calls)
	}
}

// TestOnDoneWithoutClock: with reduced motion, and in a tree no frame
// clock drives, OnDone runs right after the target applies.
func TestOnDoneWithoutClock(t *testing.T) {
	x, vp := animValue[float32](t, 0, true)
	calls := 0
	Animate(*vp, 5, Duration(time.Second), OnDone(func() { calls++ }))
	x.b.Drain(x.h)
	if (*vp).Get() != 5 || calls != 1 {
		t.Errorf("reduced motion: value %v, OnDone %d", (*vp).Get(), calls)
	}
	rendered := 0
	Render(Component(func() Node {
		v := UseSpring(float32(0), 30, OnDone(func() { rendered++ }))
		return div(Class("size-4"), Style(style.TranslateFn(func() (float32, float32) { return v.Get(), 0 })))
	}), RenderOptions{Width: 50, Height: 50, Config: Config{Fonts: testFonts}})
	if rendered != 1 {
		t.Errorf("Render: OnDone ran %d times", rendered)
	}
}

func TestVelocity(t *testing.T) {
	x, vp := animValue[float32](t, 0, false)
	v := *vp
	// From rest at its target, a velocity carries the value away and the
	// spring brings it back.
	Animate(v, 0, Velocity(float32(800)), Stiffness(200), Damping(20))
	x.frame()
	a := v.anim.(*valueAnim[float32])
	if !a.running || a.cur.Vel.C[0] != 800 {
		t.Fatalf("the spring does not start at 800/s: running %v, %v", a.running, a.cur.Vel)
	}
	var peak float32
	for range 30 {
		x.frame()
		peak = max(peak, v.Get())
	}
	if peak < 20 {
		t.Errorf("the value went only to %v", peak)
	}
	x.settle()
	if v.Get() != 0 {
		t.Errorf("ended at %v", v.Get())
	}
	// It replaces the velocity of the interrupted motion.
	Animate(v, 100, Stiffness(200), Damping(20))
	x.runFor(100 * time.Millisecond)
	Animate(v, 100, Velocity(float32(-300)), Stiffness(200), Damping(20))
	x.frame()
	a = v.anim.(*valueAnim[float32])
	start := x.tree().anim.tk.Now()
	if a.cur.Vel.C[0] != -300 || a.cur.Start != start {
		t.Errorf("velocity %v, want -300", a.cur.Vel.C[0])
	}
	before := v.Get()
	x.frame()
	if v.Get() >= before {
		t.Errorf("not moving the way of the given velocity: %v then %v", before, v.Get())
	}
	x.settle()
	// Per component for a Point.
	y, pp := animValue(t, Point{}, false)
	Animate(*pp, Point{}, Velocity(Point{X: 300, Y: -150}))
	y.frame()
	if pa := (*pp).anim.(*valueAnim[Point]); pa.cur.Vel.C[0] != 300 || pa.cur.Vel.C[1] != -150 {
		t.Errorf("point velocity %v", pa.cur.Vel)
	}
	y.frame()
	if p := (*pp).Get(); !(p.X > 0 && p.Y < 0) {
		t.Errorf("point moved to %v", p)
	}
	y.settle()
}

func TestVelocityOptions(t *testing.T) {
	dev := NewTree(Config{Fonts: testFonts, Dev: true, Log: func(string) {}})
	prod := NewTree(Config{Fonts: testFonts})
	panics := func(f func()) (msg string) {
		defer func() {
			if r := recover(); r != nil {
				msg = r.(string)
			}
		}()
		f()
		return ""
	}
	cases := []struct {
		name, typ, msg string
		mode           animMode
		opts           []AnimOption
	}{
		{"on a tween", "float32", "UseTween takes Duration, Ease and OnDone", animTween, []AnimOption{Velocity(float32(1))}},
		{"on a color", "gfx.Color", "Velocity does not apply to colors", animSpring, []AnimOption{Velocity(gfx.Black)}},
		{"another type", "float32", "Velocity of a core.Point for a Value of float32", animAuto, []AnimOption{Velocity(Point{1, 2})}},
		{"inside Spring", "float32", "", animAuto, []AnimOption{Spring(Velocity(float32(3)))}},
		{"OnDone nil", "float32", "OnDone(nil)", animAuto, []AnimOption{OnDone(nil)}},
	}
	for _, c := range cases {
		msg := panics(func() { dev.resolveMotion("Animate", c.mode, c.typ, c.opts) })
		if c.msg == "" && msg != "" || c.msg != "" && !strings.Contains(msg, c.msg) {
			t.Errorf("%s: dev panic %q, want %q", c.name, msg, c.msg)
		}
		m := prod.resolveMotion("Animate", c.mode, c.typ, c.opts)
		if c.msg != "" && m.hasVel {
			t.Errorf("%s: the invalid velocity is used", c.name)
		}
	}
	// Velocity alone makes Animate a spring.
	if m := prod.resolveMotion("Animate", animAuto, "float32", []AnimOption{Velocity(float32(5)), Duration(time.Second)}); !m.spec.Spring || !m.hasVel {
		t.Errorf("Velocity with Duration: %+v", m)
	}
}

// fills returns the colors of the FillRect commands of a display list.
func fills(dl *gfx.DisplayList) []gfx.Color {
	var out []gfx.Color
	for _, c := range dl.All() {
		if f, ok := c.(gfx.FillRect); ok {
			if col, ok := f.Paint.(gfx.Color); ok {
				out = append(out, col)
			}
		}
	}
	return out
}

// glyphPaints returns the paints of the glyph runs of a display list.
func glyphPaints(dl *gfx.DisplayList) []gfx.Paint {
	var out []gfx.Paint
	for _, c := range dl.All() {
		if g, ok := c.(gfx.DrawGlyphRun); ok {
			out = append(out, g.Paint)
		}
	}
	return out
}

func hasFill(dl *gfx.DisplayList, c gfx.Color) bool {
	for _, f := range fills(dl) {
		if f == c {
			return true
		}
	}
	return false
}

func TestBgFnPaintsOnly(t *testing.T) {
	var bg *Value[gfx.Color]
	red, blue := gfx.RGB8(255, 0, 0), gfx.RGB8(0, 0, 255)
	builds := 0
	comp := func() Node {
		builds++
		bg = UseValue(red)
		return div(Class("p-4"),
			// The Fn wins over the class, and over its transition.
			div(Key("b"), Class("size-10 bg-[#00ff00] transition-colors"), Style(style.BgFn(bg.Get))),
			p(Class("w-40"), Text("a paragraph")))
	}
	tr := newTestTree(t, 300, 200, Component(comp), nil)
	dl, _ := tr.Frame()
	if !hasFill(dl, red) || hasFill(dl, gfx.RGB8(0, 255, 0)) {
		t.Fatalf("fills %v, want the Fn's red", fills(dl))
	}
	s0 := tr.Stats()
	bg.Set(blue)
	if !tr.ApplyUpdates() {
		t.Fatal("a Value read by BgFn invalidated nothing")
	}
	dl, _ = tr.Frame()
	if d := work(s0, tr.Stats()); d != paintOnly {
		t.Errorf("work %+v, want a paint only", d)
	}
	if !hasFill(dl, blue) || hasFill(dl, red) || builds != 1 {
		t.Errorf("after the change: fills %v, builds %d", fills(dl), builds)
	}
}

// TestTextColorFn: the text of the element and the text that inherits its
// color follow the Fn when painted; text with a color of its own does not;
// a change only repaints.
func TestTextColorFn(t *testing.T) {
	var c *Value[gfx.Color]
	red, blue, green := gfx.RGB8(255, 0, 0), gfx.RGB8(0, 0, 255), gfx.RGB8(0, 160, 0)
	comp := func() Node {
		c = UseValue(red)
		return div(Class("p-4"),
			p(Key("p"), Class("text-black"), Style(style.TextColorFn(c.Get)),
				Text("Status: "), strong(Text("online ")), span(Class("text-[#00a000]"), Text("ok"))),
			div(Style(style.TextColorFn(c.Get)), p(Text("child paragraph"))),
			p(Text("plain")))
	}
	tr := newTestTree(t, 400, 300, Component(comp), nil)
	dl, _ := tr.Frame()
	// Runs: the P's own text, the bold Strong (another face) inheriting
	// it, the span with its own color, the child paragraph, the plain one.
	want := []gfx.Paint{red, red, green, red, gfx.Black}
	if got := glyphPaints(dl); !samePaints(got, want) {
		t.Fatalf("glyph paints %v, want %v", got, want)
	}
	s0 := tr.Stats()
	c.Set(blue)
	tr.ApplyUpdates()
	dl, _ = tr.Frame()
	if d := work(s0, tr.Stats()); d != paintOnly {
		t.Errorf("work %+v, want a paint only", d)
	}
	want = []gfx.Paint{blue, blue, green, blue, gfx.Black}
	if got := glyphPaints(dl); !samePaints(got, want) {
		t.Errorf("after the change: %v, want %v", got, want)
	}
	if tr.textColorFns != 2 {
		t.Errorf("%d boxes with TextColorFn counted, want 2", tr.textColorFns)
	}
	tr.Unmount()
	if tr.textColorFns != 0 {
		t.Errorf("%d after unmount", tr.textColorFns)
	}
}

// TestTextColorFnInline: a Span with TextColorFn in a paragraph recolors
// only its text, and a change of its Value repaints.
func TestTextColorFnInline(t *testing.T) {
	var c *Value[gfx.Color]
	var bg *Value[gfx.Color]
	comp := func() Node {
		c = UseValue(gfx.RGB8(255, 0, 0))
		bg = UseValue(gfx.Transparent)
		return p(Key("p"), Text("a "), span(Style(style.TextColorFn(c.Get), style.BgFn(bg.Get)), Text("span")), Text(" b"))
	}
	tr := newTestTree(t, 300, 100, Component(comp), nil)
	dl, _ := tr.Frame()
	if got := glyphPaints(dl); !samePaints(got, []gfx.Paint{gfx.Black, gfx.RGB8(255, 0, 0), gfx.Black}) {
		t.Fatalf("glyph paints %v", got)
	}
	s0 := tr.Stats()
	c.Set(gfx.RGB8(0, 0, 255))
	bg.Set(gfx.RGB8(255, 255, 0))
	if !tr.ApplyUpdates() {
		t.Fatal("the inline element's Values invalidated nothing")
	}
	dl, _ = tr.Frame()
	if d := work(s0, tr.Stats()); d != paintOnly {
		t.Errorf("work %+v, want a paint only", d)
	}
	if got := glyphPaints(dl); !samePaints(got, []gfx.Paint{gfx.Black, gfx.RGB8(0, 0, 255), gfx.Black}) {
		t.Errorf("after the change: %v", got)
	}
	if !hasFill(dl, gfx.RGB8(255, 255, 0)) {
		t.Errorf("the span's BgFn background is not painted: %v", fills(dl))
	}
}

func samePaints(a, b []gfx.Paint) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// TestTextColorFnContents: the TextColorFn of a display: contents element
// colors its children's text, and a change repaints it.
func TestTextColorFnContents(t *testing.T) {
	var c *Value[gfx.Color]
	comp := func() Node {
		c = UseValue(gfx.RGB8(255, 0, 0))
		return div(Class("contents"), Style(style.TextColorFn(c.Get)), p(Text("inside")))
	}
	tr := newTestTree(t, 300, 100, Component(comp), nil)
	dl, _ := tr.Frame()
	if got := glyphPaints(dl); !samePaints(got, []gfx.Paint{gfx.RGB8(255, 0, 0)}) {
		t.Fatalf("glyph paints %v", got)
	}
	s0 := tr.Stats()
	c.Set(gfx.RGB8(0, 0, 255))
	if !tr.ApplyUpdates() {
		t.Fatal("the change invalidated nothing")
	}
	dl, _ = tr.Frame()
	if d := work(s0, tr.Stats()); d != paintOnly {
		t.Errorf("work %+v, want a paint only", d)
	}
	if got := glyphPaints(dl); !samePaints(got, []gfx.Paint{gfx.RGB8(0, 0, 255)}) {
		t.Errorf("after the change: %v", got)
	}
}
