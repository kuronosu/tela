package tela_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// exampleT stands in for the *testing.T of a test in the examples, which
// have none: the examples mount apps with telatest to show the values
// over time. close unmounts them.
type exampleT struct {
	testing.TB // nil: the methods telatest uses are below
	cleanups   []func()
}

func (e *exampleT) Helper()                      {}
func (e *exampleT) Name() string                 { return "example" }
func (e *exampleT) Cleanup(f func())             { e.cleanups = append(e.cleanups, f) }
func (e *exampleT) Logf(string, ...any)          {}
func (e *exampleT) Errorf(f string, args ...any) { panic(fmt.Sprintf(f, args...)) }
func (e *exampleT) Fatalf(f string, args ...any) { panic(fmt.Sprintf(f, args...)) }
func (e *exampleT) Failed() bool                 { return false }
func (e *exampleT) close() {
	for i := len(e.cleanups) - 1; i >= 0; i-- {
		e.cleanups[i]()
	}
}

func ExampleUseSpring() {
	t := &exampleT{}
	defer t.close()
	var x *tela.Value[float32]
	panel := func() tela.Node {
		open, setOpen := tela.UseState(false)
		target := float32(-320)
		if open {
			target = 0
		}
		x = tela.UseSpring(target, tela.Stiffness(300), tela.Damping(30))
		return Div(Class("relative h-full"),
			Button(OnClick(func(tela.MouseEvent) { setOpen.Set(true) }), Text("Open")),
			Div(Class("absolute inset-y-0 left-0 w-80 bg-white shadow-xl"),
				Style(tela.TranslateFn(func() (float32, float32) { return x.Get(), 0 }))))
	}
	// NoSettle: the click does not run the spring to its end.
	app := telatest.Mount(t, tela.C(panel), telatest.NoSettle())
	app.Click(telatest.ByRole("button", "Open"))
	for range 3 {
		app.Advance(100 * time.Millisecond)
		fmt.Printf("%.1f\n", x.Get())
	}
	app.Advance(time.Second) // Advance moves the clock: the spring comes to rest
	fmt.Println(x.Get())
	// Output:
	// -140.5
	// -24.7
	// -0.1
	// 0
}

func ExampleUseTween() {
	t := &exampleT{}
	defer t.close()
	var op *tela.Value[float32]
	toast := func() tela.Node {
		shown, setShown := tela.UseState(false)
		target := float32(0)
		if shown {
			target = 1
		}
		op = tela.UseTween(target, tela.Duration(200*time.Millisecond), tela.Ease("linear"))
		return Div(
			Button(OnClick(func(tela.MouseEvent) { setShown.Set(true) }), Text("Save")),
			Div(Class("rounded-lg bg-slate-900 p-3 text-white"), Style(tela.OpacityFn(op.Get)), Text("Saved")))
	}
	app := telatest.Mount(t, tela.C(toast), telatest.NoSettle())
	app.Click(telatest.ByRole("button", "Save"))
	app.Advance(50 * time.Millisecond)
	fmt.Printf("%.2f\n", op.Get())
	app.Advance(150 * time.Millisecond)
	fmt.Printf("%.2f\n", op.Get())
	// Output:
	// 0.25
	// 1.00
}

func ExampleAnimate() {
	t := &exampleT{}
	defer t.close()
	var y *tela.Value[float32]
	ball := func() tela.Node {
		y = tela.UseValue[float32](0)
		return Div(
			Button(OnClick(func(tela.MouseEvent) { tela.Animate(y, 200, tela.Spring(tela.Bounce(0.25))) }), Text("Drop")),
			Button(OnClick(func(tela.MouseEvent) {
				tela.Animate(y, 0, tela.Duration(300*time.Millisecond), tela.Ease("ease-in-out"))
			}), Text("Lift")),
			Div(Class("size-8 rounded-full bg-indigo-500"), Style(tela.TranslateFn(func() (float32, float32) { return 0, y.Get() }))))
	}
	app := telatest.Mount(t, tela.C(ball))
	app.Click(telatest.ByRole("button", "Drop")) // settles: the spring ends
	fmt.Println(y.Get())
	app.Click(telatest.ByRole("button", "Lift"))
	fmt.Println(y.Get())
	// Output:
	// 200
	// 0
}

func ExamplePoint() {
	t := &exampleT{}
	defer t.close()
	var pos *tela.Value[tela.Point]
	card := func() tela.Node {
		pos = tela.UseValue(tela.Point{})
		return Div(Class("size-24 rounded-xl bg-white shadow"),
			Style(tela.TranslateFn(func() (float32, float32) { p := pos.Get(); return p.X, p.Y })),
			// Released: back to the origin on a bouncy spring.
			OnDragEnd(func(tela.DragEvent) { tela.Animate(pos, tela.Point{}, tela.Spring(tela.Bounce(0.4))) }),
			OnDrag(func(e tela.DragEvent) { pos.Set(tela.Point{X: e.OffsetX, Y: e.OffsetY}) }))
	}
	app := telatest.Mount(t, tela.C(card), telatest.NoSettle())
	// Called between frames (like from a goroutine), Animate starts at the
	// next frame: 50 ms later the tween has run for 33 ms of its 100.
	tela.Animate(pos, tela.Point{X: 40, Y: 30}, tela.Duration(100*time.Millisecond), tela.Ease("linear"))
	app.Advance(50 * time.Millisecond)
	fmt.Printf("%.0f %.0f\n", pos.Get().X, pos.Get().Y)
	// Output: 13 10
}

func ExampleSize() {
	t := &exampleT{}
	defer t.close()
	var size *tela.Value[tela.Size]
	app := telatest.Mount(t, tela.C(func() tela.Node {
		size = tela.UseSpring(tela.Size{Width: 200, Height: 120}, tela.Initial(tela.Size{Width: 100, Height: 40}))
		return Div(Class("rounded-lg bg-sky-500"), Style(
			tela.WidthFn(func() float32 { return size.Get().Width }),
			tela.HeightFn(func() float32 { return size.Get().Height })))
	}))
	// Mount settles: the spring from the initial size to the target ended.
	wBox := app.Bounds(telatest.ByClass("bg-sky-500"))
	w, h := wBox.Width, wBox.Height
	fmt.Println(w, h)
	// Output: 200 120
}

func ExampleAnimatable() {
	t := &exampleT{}
	defer t.close()
	var bg *tela.Value[tela.Color]
	app := telatest.Mount(t, tela.C(func() tela.Node {
		bg = tela.UseValue(tela.Hex("#ffffff"))
		// Read in the paint phase: the animation only repaints the box.
		return Div(Class("size-10"), Style(tela.BgFn(bg.Get)))
	}), telatest.NoSettle())
	// Hex colors interpolate in sRGB, like a CSS transition between them.
	tela.Animate(bg, tela.Hex("#000000"), tela.Duration(100*time.Millisecond), tela.Ease("linear"))
	app.Advance(50 * time.Millisecond) // 33 ms of the tween: see ExamplePoint
	fmt.Println(bg.Get())
	// Output: #aaaaaa
}

func ExampleAnimationOption() {
	// Options for the three kinds of motion.
	physics := []tela.AnimationOption{tela.Stiffness(300), tela.Damping(30)}
	timed := []tela.AnimationOption{tela.Spring(tela.Duration(400*time.Millisecond), tela.Bounce(0.2))}
	tween := []tela.AnimationOption{tela.Duration(250 * time.Millisecond), tela.Ease("cubic-bezier(0, 0, 0.2, 1)")}
	fmt.Println(len(physics), len(timed), len(tween))
	// Output: 2 1 2
}

// springEnd mounts a box that animates from 0 to 100 with opts and
// returns the app time from the Animate call to the end, rounded to 10 ms
// (it starts at the next frame, and ends at a frame).
func springEnd(opts ...tela.AnimationOption) time.Duration {
	t := &exampleT{}
	defer t.close()
	var v *tela.Value[float32]
	app := telatest.Mount(t, tela.C(func() tela.Node {
		v = tela.UseValue[float32](0)
		return Div(Class("size-4 bg-black"), Style(tela.TranslateFn(func() (float32, float32) { return v.Get(), 0 })))
	}), telatest.NoSettle())
	start := app.Now()
	tela.Animate(v, 100, opts...)
	for v.Get() != 100 {
		app.Advance(time.Second / 60)
	}
	return (app.Now() - start).Round(10 * time.Millisecond)
}

func ExampleStiffness() {
	// Four times the stiffness (with the damping that keeps both critically
	// damped) arrives in about half the time.
	fmt.Println("stiffness 100:", springEnd(tela.Stiffness(100), tela.Damping(20)))
	fmt.Println("stiffness 400:", springEnd(tela.Stiffness(400), tela.Damping(40)))
	// Output:
	// stiffness 100: 870ms
	// stiffness 400: 480ms
}

func ExampleDamping() {
	// Damping 2*sqrt(stiffness) is critically damped: no overshoot. Less
	// bounces around the target and takes longer to come to rest.
	fmt.Println("damping 40:", springEnd(tela.Stiffness(400), tela.Damping(40)))
	fmt.Println("damping 10:", springEnd(tela.Stiffness(400), tela.Damping(10)))
	// Output:
	// damping 40: 480ms
	// damping 10: 1.15s
}

func ExampleBounce() {
	// A spring defined by its duration (800 ms without Duration) ends no
	// earlier than it, at rest.
	fmt.Println("bounce 0:", springEnd(tela.Spring(tela.Bounce(0))))
	fmt.Println("bounce 0.5:", springEnd(tela.Spring(tela.Bounce(0.5))))
	// Output:
	// bounce 0: 830ms
	// bounce 0.5: 870ms
}

func ExampleDuration() {
	fmt.Println("tween:", springEnd(tela.Duration(250*time.Millisecond)))
	fmt.Println("spring:", springEnd(tela.Spring(tela.Duration(250*time.Millisecond))))
	// Output:
	// tween: 270ms
	// spring: 320ms
}

func ExampleEase() {
	fmt.Println("ease-out:", springEnd(tela.Ease("ease-out"))) // 300 ms by default
	fmt.Println("steps:", springEnd(tela.Duration(time.Second), tela.Ease("steps(4, jump-end)")))
	// Output:
	// ease-out: 320ms
	// steps: 1.03s
}

func ExampleSpring() {
	fmt.Println("Motion's default:", springEnd(tela.Spring()))
	fmt.Println("500 ms, bounce 0.3:", springEnd(tela.Spring(tela.Duration(500*time.Millisecond), tela.Bounce(0.3))))
	// Output:
	// Motion's default: 1.07s
	// 500 ms, bounce 0.3: 530ms
}

func ExampleUsePresence() {
	t := &exampleT{}
	defer t.close()
	toast := func() tela.Node {
		present, done := tela.UsePresence()
		target := float32(0)
		if !present {
			target = 1
		}
		// The tween out ends the exit: done does nothing while present.
		out := tela.UseTween(target, tela.Duration(200*time.Millisecond), tela.OnDone(done))
		return Div(Role("status"), Class("rounded-lg bg-slate-900 px-4 py-2 text-white"),
			Style(tela.OpacityFn(func() float32 { return 1 - out.Get() })), Text("Saved"))
	}
	page := func() tela.Node {
		shown, setShown := tela.UseState(true)
		return Div(
			Button(OnClick(func(tela.MouseEvent) { setShown.Set(false) }), Text("Dismiss")),
			Presence(If(shown, tela.C(toast))))
	}
	app := telatest.Mount(t, tela.C(page))
	start := app.Now()
	app.Click(telatest.ByRole("button", "Dismiss")) // waits for the exit
	fmt.Println(app.Count(telatest.ByRole("status", "")), app.Now()-start >= 200*time.Millisecond)
	// Output: 0 true
}

func ExampleOnDone() {
	t := &exampleT{}
	defer t.close()
	var x *tela.Value[float32]
	var log []string
	app := telatest.Mount(t, tela.C(func() tela.Node {
		x = tela.UseValue[float32](0)
		return Div(Class("size-8 bg-black"), Style(tela.TranslateFn(func() (float32, float32) { return x.Get(), 0 })))
	}), telatest.NoSettle())
	tela.Animate(x, 100, tela.OnDone(func() { log = append(log, "first") }))
	app.Advance(50 * time.Millisecond)
	// Interrupted: the first OnDone never runs.
	tela.Animate(x, 0, tela.Duration(100*time.Millisecond), tela.OnDone(func() { log = append(log, "second") }))
	app.Advance(time.Second)
	fmt.Println(log, x.Get())
	// Output: [second] 0
}

func ExampleVelocity() {
	t := &exampleT{}
	defer t.close()
	var x *tela.Value[float32]
	app := telatest.Mount(t, tela.C(func() tela.Node {
		x = tela.UseValue[float32](0)
		return Div(Class("size-8 bg-black"), Style(tela.TranslateFn(func() (float32, float32) { return x.Get(), 0 })))
	}), telatest.NoSettle())
	// A flick at rest: the value is already at its target, the velocity
	// carries it away and the spring brings it back.
	tela.Animate(x, 0, tela.Stiffness(200), tela.Damping(20), tela.Velocity(float32(600)))
	var peak float32
	for range 30 {
		app.Advance(time.Second / 60)
		peak = max(peak, x.Get())
	}
	app.Advance(2 * time.Second)
	fmt.Printf("peak %.0f, rest %v\n", peak, x.Get())
	// Output: peak 19, rest 0
}

func ExampleBgFn() {
	t := &exampleT{}
	defer t.close()
	var bg *tela.Value[tela.Color]
	app := telatest.Mount(t, tela.C(func() tela.Node {
		bg = tela.UseValue(tela.Hex("#ffffff"))
		return Div(Class("size-10"), Style(tela.BgFn(bg.Get)))
	}), telatest.Size(40, 40))
	before := app.Stats()
	bg.Set(tela.Hex("#4f46e5"))
	after := app.Stats() // settles: the frame that shows the change
	// The change repainted the box: no build, style or layout.
	fmt.Println(app.Screenshot().RGBAAt(5, 5), after.Builds-before.Builds, after.Styles-before.Styles, after.Layouts-before.Layouts)
	// Output: {79 70 229 255} 0 0 0
}

func ExampleTextColorFn() {
	t := &exampleT{}
	defer t.close()
	var c *tela.Value[tela.Color]
	app := telatest.Mount(t, tela.C(func() tela.Node {
		c = tela.UseValue(tela.Hex("#0f172a"))
		// The Strong inherits the paragraph's color, so it follows the Fn
		// too; the Span has a color of its own.
		return P(Style(tela.TextColorFn(c.Get)),
			Text("Status: "), Strong(Text("online")), Span(Class("text-slate-400"), Text(" (auto)")))
	}))
	before := app.Stats()
	c.Set(tela.Hex("#16a34a"))
	after := app.Stats()
	// Builds, styles, layouts and paints of the change.
	fmt.Println(after.Builds-before.Builds, after.Styles-before.Styles, after.Layouts-before.Layouts, after.Paints-before.Paints)
	// Output: 0 0 0 1
}
