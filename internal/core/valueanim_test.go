package core

import (
	"math"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/anim"
	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/style"
)

// Tests of UseSpring, UseTween and Animate (valueanim.go) on the frame
// clock of a headless window.

// motionProbe is a component whose box is translated by a Value that
// follows a target with UseSpring or UseTween.
type motionProbe struct {
	set    Setter[float32]
	v      *Value[float32]
	builds int
}

func (m *motionProbe) comp(tween bool, opts ...AnimOption) Node {
	return Component(func() Node {
		m.builds++
		target, set := UseState(float32(0))
		m.set = set
		if tween {
			m.v = UseTween(float32(0), target, opts...)
		} else {
			m.v = UseSpring(float32(0), target, opts...)
		}
		return div(Class("p-4"), div(Key("card"), Class("size-10 bg-sky-500"),
			Style(style.TranslateFn(func() (float32, float32) { return m.v.Get(), 0 }))))
	})
}

func newHarness(t *testing.T, root Node, reduced bool) *inputHarness {
	t.Helper()
	b := headless.New(headless.Options{Size: platform.Size{W: 300, H: 200},
		Settings: platform.SystemSettings{ReducedMotion: reduced}})
	host, err := NewHost(b, Options{Config: Config{Fonts: testFonts}, Root: root})
	if err != nil {
		t.Fatal(err)
	}
	b.Drain(host)
	x := &inputHarness{t: t, b: b, h: host, w: host.Window().(*headless.Window)}
	t.Cleanup(host.stopTimer)
	x.settle()
	return x
}

// quiet reports whether the app asks for no frame and runs no animation.
func (x *inputHarness) quiet() bool {
	x.b.Drain(x.h)
	return !x.w.FrameRequested() && !x.tree().Animating() && !x.tree().Dirty()
}

// trace runs frames for d and returns the probe's value at each.
func (x *inputHarness) trace(v *Value[float32], d time.Duration) []float32 {
	var out []float32
	end := x.b.Now() + d
	for x.b.Now() < end {
		x.frame()
		out = append(out, v.Get())
	}
	return out
}

func TestUseSpringFollowsTarget(t *testing.T) {
	var m motionProbe
	x := newHarness(t, m.comp(false, Stiffness(300), Damping(30)), false)
	if m.v.Get() != 0 || !x.quiet() {
		t.Fatalf("at rest: %v, idle %v", m.v.Get(), x.quiet())
	}
	m.set.Set(100)
	x.frame() // builds with the new target: the spring starts at this frame
	s0, b0 := x.tree().Stats(), m.builds
	vals := x.trace(m.v, 150*time.Millisecond)
	for i := 1; i < len(vals); i++ {
		if vals[i] <= vals[i-1] {
			t.Fatalf("not moving toward the target: %v", vals)
		}
	}
	// The frames of the animation only repaint: no build, style or layout.
	if d := work(s0, x.tree().Stats()); d.Builds != 0 || d.Styles != 0 || d.Layouts != 0 || m.builds != b0 {
		t.Errorf("animation frames did more than paint: %+v (builds %d)", d, m.builds-b0)
	}
	x.settle()
	if m.v.Get() != 100 {
		t.Fatalf("settled at %v", m.v.Get())
	}
	if !x.quiet() {
		t.Error("the app is not idle after the spring settled")
	}
	// Idle: a second of clock runs no frame.
	frames := x.tree().Stats().Frames
	x.b.Advance(time.Second)
	x.b.Drain(x.h)
	if x.tree().Stats().Frames != frames {
		t.Error("frames ran after the spring settled")
	}
}

// TestSpringRetargetKeepsVelocity: a new target while the spring moves
// starts the new spring where the old one is at that frame, with its
// velocity.
func TestSpringRetargetKeepsVelocity(t *testing.T) {
	var m motionProbe
	x := newHarness(t, m.comp(false, Stiffness(300), Damping(30)), false)
	m.set.Set(200)
	x.frame()
	a := m.v.anim.(*valueAnim[float32])
	old := a.cur // the first spring
	x.runFor(100 * time.Millisecond)
	before := m.v.Get()
	m.set.Set(-100)
	x.frame()
	now := x.tree().anim.tk.Now()
	pos, vel, _ := old.At(now)
	if got := m.v.Get(); got != float32(pos.C[0]) {
		t.Errorf("at the retarget frame the value is %v, want the old spring's %v", got, pos.C[0])
	}
	if a.cur.Start != now || a.cur.From.C[0] != pos.C[0] || a.cur.Vel.C[0] != vel.C[0] || vel.C[0] <= 0 {
		t.Errorf("the new spring starts at %v from %v at %v/s; want %v, %v, %v/s", a.cur.Start, a.cur.From.C[0], a.cur.Vel.C[0], now, pos.C[0], vel.C[0])
	}
	// It keeps going up for a while: the velocity was kept.
	x.frame()
	if after := m.v.Get(); after <= float32(pos.C[0]) || float32(pos.C[0]) <= before {
		t.Errorf("velocity lost: %v, %v, then %v", before, pos.C[0], after)
	}
	x.settle()
	if m.v.Get() != -100 || !x.quiet() {
		t.Errorf("settled at %v, idle %v", m.v.Get(), x.quiet())
	}
}

func TestUseTween(t *testing.T) {
	var m motionProbe
	x := newHarness(t, m.comp(true, Duration(200*time.Millisecond), Ease("linear")), false)
	m.set.Set(100)
	x.frame()
	start := m.v.anim.(*valueAnim[float32]).cur.Start
	x.advanceTo(start + 50*time.Millisecond)
	if v := m.v.Get(); !near32(v, 25, 1e-3) {
		t.Errorf("a quarter in: %v, want 25", v)
	}
	// A new target starts a new tween from where the value is.
	m.set.Set(0)
	x.frame()
	a := m.v.anim.(*valueAnim[float32])
	if float32(a.cur.From.C[0]) != m.v.Get() || a.cur.Vel.C[0] != 0 {
		t.Errorf("the second tween starts from %v (velocity %v), the value is %v", a.cur.From.C[0], a.cur.Vel.C[0], m.v.Get())
	}
	x.settle()
	if m.v.Get() != 0 || !x.quiet() {
		t.Errorf("ended at %v", m.v.Get())
	}
	// The default tween is 300 ms with CSS's ease.
	tr := NewTree(Config{Fonts: testFonts})
	spec := tr.resolveMotion("UseTween", animTween, "float32", nil).spec
	if spec.Spring || spec.Duration != 300*time.Millisecond || spec.Curve != anim.Ease {
		t.Errorf("default tween %+v", spec)
	}
}

// animTarget mounts a component with a Value v of type T read in paint.
func animValue[T Animatable](t *testing.T, initial T, reduced bool) (*inputHarness, **Value[T]) {
	t.Helper()
	var v *Value[T]
	root := Component(func() Node {
		v = UseValue(initial)
		return div(Key("box"), Class("size-10 bg-sky-500"), Style(style.OpacityFn(func() float32 {
			_ = v.Get()
			return 1
		})))
	})
	return newHarness(t, root, reduced), &v
}

func TestAnimateInterrupts(t *testing.T) {
	x, vp := animValue[float32](t, 0, false)
	v := *vp
	Animate(v, 100, Stiffness(200), Damping(20))
	x.b.Drain(x.h) // applied between frames: starts at the next frame
	if v.Get() != 0 || !x.w.FrameRequested() {
		t.Fatalf("Animate did not ask for a frame: %v", v.Get())
	}
	x.runFor(100 * time.Millisecond)
	mid := v.Get()
	if !(mid > 0 && mid < 100) {
		t.Fatalf("after 100 ms: %v", mid)
	}
	// A tween interrupts the spring from where it is.
	Animate(v, 0, Duration(300*time.Millisecond), Ease("ease-in-out"))
	x.frame()
	a := v.anim.(*valueAnim[float32])
	if a.cur.Spec.Spring || a.cur.Spec.Duration != 300*time.Millisecond {
		t.Fatalf("not a tween: %+v", a.cur.Spec)
	}
	if d := math.Abs(float64(v.Get()) - float64(mid)); d > 10 {
		t.Errorf("the value jumped from %v to %v", mid, v.Get())
	}
	// Set stops it.
	x.runFor(50 * time.Millisecond)
	v.Set(42)
	x.frame()
	x.runFor(100 * time.Millisecond)
	if v.Get() != 42 || !x.quiet() {
		t.Errorf("Set did not stop the animation: %v, idle %v", v.Get(), x.quiet())
	}
	// Animate without options is a spring; it settles exactly.
	Animate(v, 7)
	x.settle()
	if v.Get() != 7 || !x.quiet() {
		t.Errorf("default spring ended at %v", v.Get())
	}
	// Set and Update after Animate in the same goroutine win, in order.
	Animate(v, 100)
	v.Update(func(f float32) float32 { return f + 1 })
	x.settle()
	if v.Get() != 8 {
		t.Errorf("Update after Animate: %v, want 8", v.Get())
	}
}

func TestAnimateTypes(t *testing.T) {
	lin := []AnimOption{Duration(100 * time.Millisecond), Ease("linear")}
	mid := func(x *inputHarness, a interface{ startTime() time.Duration }) {
		x.advanceTo(a.startTime() + 50*time.Millisecond)
	}
	t.Run("color", func(t *testing.T) {
		x, vp := animValue(t, gfx.RGB8(255, 255, 255), false)
		v := *vp
		Animate(v, gfx.Black, lin...)
		x.frame()
		mid(x, v.anim.(*valueAnim[gfx.Color]))
		if c := v.Get(); !near32(c.R, 0.5, 1e-5) || c.R != c.G || c.A != 1 {
			t.Errorf("halfway white to black: %v (sRGB for legacy colors)", c)
		}
		x.settle()
		if v.Get() != gfx.Black {
			t.Errorf("end %v", v.Get())
		}
		// Oklch colors interpolate in Oklab, like CSS transitions.
		to := gfx.OKLCH(0.637, 0.237, 25.331, 1)
		Animate(v, to, lin...)
		x.frame()
		mid(x, v.anim.(*valueAnim[gfx.Color]))
		if want := anim.LerpColor(gfx.Black, to, 0.5); !nearColor(v.Get(), want) {
			t.Errorf("halfway black to oklch: %v, want %v", v.Get(), want)
		}
		x.settle()
		if v.Get() != to {
			t.Errorf("end %v", v.Get())
		}
	})
	t.Run("point", func(t *testing.T) {
		x, vp := animValue(t, Point{}, false)
		v := *vp
		Animate(v, Point{100, -50}, lin...)
		x.frame()
		mid(x, v.anim.(*valueAnim[Point]))
		if p := v.Get(); !near32(p.X, 50, 1e-3) || !near32(p.Y, -25, 1e-3) {
			t.Errorf("halfway %v", p)
		}
		x.settle()
		if v.Get() != (Point{100, -50}) {
			t.Errorf("end %v", v.Get())
		}
	})
	t.Run("size", func(t *testing.T) {
		x, vp := animValue(t, Size{10, 20}, false)
		v := *vp
		Animate(v, Size{30, 60}, lin...)
		x.frame()
		mid(x, v.anim.(*valueAnim[Size]))
		if s := v.Get(); !near32(s.Width, 20, 1e-3) || !near32(s.Height, 40, 1e-3) {
			t.Errorf("halfway %v", s)
		}
		x.settle()
		if v.Get() != (Size{30, 60}) {
			t.Errorf("end %v", v.Get())
		}
	})
	t.Run("rect", func(t *testing.T) {
		x, vp := animValue(t, Rect{}, false)
		v := *vp
		Animate(v, Rect{10, 20, 30, 40}) // a spring, component by component
		x.runFor(50 * time.Millisecond)
		r := v.Get()
		if !(r.X > 0 && r.Y > r.X && r.Width > r.Y && r.Height > r.Width) {
			t.Errorf("moving %v", r)
		}
		x.settle()
		if v.Get() != (Rect{10, 20, 30, 40}) {
			t.Errorf("end %v", v.Get())
		}
	})
	t.Run("float32 NaN", func(t *testing.T) {
		x, vp := animValue[float32](t, 0, false)
		v := *vp
		Animate(v, float32(math.NaN()))
		x.settle()
		if !math.IsNaN(float64(v.Get())) || !x.quiet() {
			t.Errorf("a NaN target is set at once: %v", v.Get())
		}
	})
}

func (a *valueAnim[T]) startTime() time.Duration { return a.cur.Start }

func nearColor(a, b gfx.Color) bool {
	return near32(a.R, b.R, 1e-4) && near32(a.G, b.G, 1e-4) && near32(a.B, b.B, 1e-4) && near32(a.A, b.A, 1e-4)
}

func TestReducedMotionJumps(t *testing.T) {
	var m motionProbe
	x := newHarness(t, m.comp(false), true)
	m.set.Set(100)
	x.frame()
	if m.v.Get() != 100 || !x.quiet() {
		t.Errorf("UseSpring with reduced motion: %v, idle %v", m.v.Get(), x.quiet())
	}
	y, vp := animValue[float32](t, 0, true)
	Animate(*vp, 50, Duration(time.Second))
	y.b.Drain(y.h)
	if (*vp).Get() != 50 {
		t.Errorf("Animate with reduced motion: %v", (*vp).Get())
	}
	y.frame() // the frame that shows it
	if !y.quiet() {
		t.Errorf("Animate with reduced motion: %v", (*vp).Get())
	}
	// Turning reduced motion on during an animation ends it.
	z, zp := animValue[float32](t, 0, false)
	Animate(*zp, 50, Duration(time.Second))
	z.runFor(100 * time.Millisecond)
	z.tree().SetEnvironment(false, true)
	z.frame()
	if (*zp).Get() != 50 {
		t.Errorf("reduced motion did not end the animation: %v", (*zp).Get())
	}
}

// TestRenderJumpsToTarget: a tree without a frame clock (Render, tela
// snapshot) shows the targets.
func TestRenderJumpsToTarget(t *testing.T) {
	var got float32
	_, img := Render(Component(func() Node {
		v := UseSpring(float32(0), 30)
		return div(Class("size-10 bg-black"), Style(style.TranslateFn(func() (float32, float32) {
			got = v.Get()
			return v.Get(), 0
		})))
	}), RenderOptions{Width: 100, Height: 50, Config: Config{Fonts: testFonts}})
	if got != 30 {
		t.Errorf("rendered at %v, want the target 30", got)
	}
	if c := img.RGBAAt(35, 5); c.R != 0 || c.A != 255 {
		t.Errorf("the box is not at x 30: %v", c)
	}
}

// TestSpringUnderSkippedParent: a CP component's spring animates while
// its parent rebuilds and skips it, and its frames neither build nor lay
// out (TranslateFn) or only lay out (WidthFn).
func TestSpringUnderSkippedParent(t *testing.T) {
	type props struct{ Open bool }
	var x, w *Value[float32]
	builds := 0
	card := func(p props) Node {
		builds++
		target := float32(0)
		if p.Open {
			target = 100
		}
		x = UseSpring(float32(0), target)
		w = UseSpring(float32(20), 20+target/2)
		return div(Key("card"), Class("h-10 bg-sky-500"), Style(
			style.TranslateFn(func() (float32, float32) { return x.Get(), 0 }),
			style.WidthFn(func() float32 { return w.Get() })))
	}
	var setOpen Setter[bool]
	var setOther Setter[int]
	root := Component(func() Node {
		open, s := UseState(false)
		other, s2 := UseState(0)
		setOpen, setOther = s, s2
		return div(Textf("%d", other), ComponentProps(card, props{open}))
	})
	h := newHarness(t, root, false)
	setOpen.Set(true)
	h.frame()
	b0, s0 := builds, h.tree().Stats()
	h.runFor(100 * time.Millisecond)
	setOther.Set(1) // the parent builds; the card skips
	h.runFor(100 * time.Millisecond)
	d := work(s0, h.tree().Stats())
	if builds != b0 || d.Skips != 1 {
		t.Errorf("the card built %d times while animating (skips %d)", builds-b0, d.Skips)
	}
	if d.Styles > 2 || d.Layouts < 5 {
		t.Errorf("work while animating %+v: the width lays out, nothing restyles", d)
	}
	h.settle()
	if x.Get() != 100 || w.Get() != 70 || !h.quiet() {
		t.Errorf("ended at %v, %v", x.Get(), w.Get())
	}
}

func TestAnimationUnmount(t *testing.T) {
	var v *Value[float32]
	var setShow Setter[bool]
	child := func() Node {
		v = UseSpring(float32(0), 100)
		return div(Class("size-4"), Style(style.TranslateFn(func() (float32, float32) { return v.Get(), 0 })))
	}
	root := Component(func() Node {
		show, s := UseState(true)
		setShow = s
		return div(If(show, Component(child)))
	})
	x := newHarness(t, root, false)
	// newHarness settles: mount, then the spring from 0 to 100.
	if v.Get() != 100 {
		t.Fatalf("the mount spring ended at %v", v.Get())
	}
	setShow.Set(false)
	x.frame()
	setShow.Set(true)
	x.frame()
	x.runFor(50 * time.Millisecond)
	kept := v
	setShow.Set(false) // unmount while it moves
	x.frame()
	if !x.quiet() || kept.anim.(*valueAnim[float32]).running {
		t.Error("an unmounted component's spring keeps running")
	}
	Animate(kept, 3) // dropped: the component is gone
	x.frame()
	if !x.quiet() {
		t.Error("Animate on an unmounted Value asks for frames")
	}
}

// TestAnimateFromGoroutines: Animate, Set and Get from many goroutines
// (run with -race).
func TestAnimateFromGoroutines(t *testing.T) {
	x, vp := animValue[float32](t, 0, false)
	v := *vp
	var wg sync.WaitGroup
	for g := range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range 50 {
				switch (g + i) % 3 {
				case 0:
					Animate(v, float32(i), Duration(50*time.Millisecond))
				case 1:
					Animate(v, float32(-i), Spring(Bounce(0.2)))
				default:
					v.Set(float32(g))
				}
				_ = v.Get()
				_ = v.Latest()
			}
		}()
	}
	for range 20 {
		x.frame()
	}
	wg.Wait()
	Animate(v, 5)
	x.settle()
	if v.Get() != 5 || !x.quiet() {
		t.Errorf("ended at %v", v.Get())
	}
}

func TestAnimationOptions(t *testing.T) {
	dev := NewTree(Config{Fonts: testFonts, Dev: true, Log: func(string) {}})
	prod := NewTree(Config{Fonts: testFonts})
	cases := []struct {
		name string
		mode animMode
		opts []AnimOption
		want anim.MotionSpec
		msg  string // the dev panic, "" for none
	}{
		{"auto is a spring", animAuto, nil, anim.MotionSpec{Spring: true, Physics: anim.DefaultSpring()}, ""},
		{"physics", animSpring, []AnimOption{Stiffness(300), Damping(30)}, anim.MotionSpec{Spring: true, Physics: anim.Spring{Stiffness: 300, Damping: 30, Mass: 1}}, ""},
		{"stiffness alone", animAuto, []AnimOption{Stiffness(50)}, anim.MotionSpec{Spring: true, Physics: anim.Spring{Stiffness: 50, Damping: 10, Mass: 1}}, ""},
		{"duration spring", animSpring, []AnimOption{Duration(400 * time.Millisecond)},
			anim.MotionSpec{Spring: true, Physics: anim.SpringForDuration(400*time.Millisecond, 0.3), MinDuration: 400 * time.Millisecond}, ""},
		{"bounce alone", animAuto, []AnimOption{Spring(Bounce(0.25))},
			anim.MotionSpec{Spring: true, Physics: anim.SpringForDuration(800*time.Millisecond, 0.25), MinDuration: 800 * time.Millisecond}, ""},
		{"auto tween", animAuto, []AnimOption{Duration(300 * time.Millisecond), Ease("ease-in-out")}, anim.MotionSpec{Duration: 300 * time.Millisecond, Curve: anim.EaseInOut}, ""},
		{"auto tween by ease", animAuto, []AnimOption{Ease("steps(4)")}, anim.MotionSpec{Duration: 300 * time.Millisecond, Curve: anim.Steps{N: 4}}, ""},
		{"bad ease", animTween, []AnimOption{Ease("ease-out-cubic")}, anim.MotionSpec{Duration: 300 * time.Millisecond, Curve: anim.Ease}, `did you mean "ease-out"?`},
		{"tailwind token", animTween, []AnimOption{Ease("out")}, anim.MotionSpec{Duration: 300 * time.Millisecond, Curve: anim.Ease}, `Ease("cubic-bezier(0, 0, 0.2, 1)") for Tailwind's ease-out`},
		{"spring on tween", animTween, []AnimOption{Stiffness(10)}, anim.MotionSpec{Duration: 300 * time.Millisecond, Curve: anim.Ease}, "UseTween takes Duration, Ease and OnDone"},
		{"ease on spring", animSpring, []AnimOption{Ease("linear")}, anim.MotionSpec{Spring: true, Physics: anim.DefaultSpring()}, "Ease applies to tweens"},
		{"ease in Spring", animAuto, []AnimOption{Spring(Ease("linear"))}, anim.MotionSpec{Spring: true, Physics: anim.DefaultSpring()}, "Ease does not apply to a spring"},
		{"physics and duration", animAuto, []AnimOption{Stiffness(10), Duration(time.Second), Bounce(0.1)}, anim.MotionSpec{Spring: true, Physics: anim.Spring{Stiffness: 10, Damping: 10, Mass: 1}}, "use one pair or the other"},
		{"negative stiffness", animSpring, []AnimOption{Stiffness(-1)}, anim.MotionSpec{Spring: true, Physics: anim.DefaultSpring()}, "must be a positive number"},
		{"negative duration", animTween, []AnimOption{Duration(-time.Second)}, anim.MotionSpec{Curve: anim.Ease}, "is negative"},
		{"zero option", animAuto, []AnimOption{{}}, anim.MotionSpec{Spring: true, Physics: anim.DefaultSpring()}, ""},
	}
	for _, c := range cases {
		got := prod.resolveMotion("Animate", c.mode, "float32", c.opts).spec
		if got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
		msg := func() (msg string) {
			defer func() {
				if r := recover(); r != nil {
					msg = r.(string)
				}
			}()
			dev.resolveMotion("Animate", c.mode, "float32", c.opts)
			return ""
		}()
		if (c.msg == "") != (msg == "") || !strings.Contains(msg, c.msg) {
			t.Errorf("%s: dev panic %q, want one containing %q", c.name, msg, c.msg)
		}
	}
}
