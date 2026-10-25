package tela

import (
	"strconv"
	"time"

	"github.com/kuronosu/tela/internal/core"
)

// Point is a point in logical pixels: X to the right, Y down. It is one of
// the [Animatable] types, for a position that moves as a whole, such as a
// dragged card's offset.
type Point = core.Point

// Size is a width and a height in logical pixels. It is one of the
// [Animatable] types.
type Size = core.Size

// Animatable lists the types that [UseSpring], [UseTween] and [Animate]
// animate:
//
//   - float32: a number (an offset, a scale, an opacity, a progress);
//   - [Color]: interpolated as CSS transitions interpolate colors,
//     premultiplied by alpha, in sRGB between two colors written as hex,
//     rgb() or named colors, and in Oklab otherwise (the Oklch palette
//     of Tailwind, [OKLCH]); read it in a [BgFn] or [TextColorFn] to
//     animate a color by repainting only;
//   - [Point], [Size] and [Rect]: component by component, each with its
//     own velocity.
type Animatable = core.Animatable

// AnimationOption configures an animation of [UseSpring], [UseTween] or
// [Animate]: [Stiffness], [Damping], [Bounce], [Duration], [Ease],
// [Velocity] and [Spring], which groups spring options, [OnDone], and
// [Initial] for the start value of UseSpring and UseTween. The zero
// AnimationOption changes nothing.
//
// Springs (Motion's model) are defined either by their physics or by how
// long they take:
//
//	tela.Stiffness(300), tela.Damping(30)        // physics; mass 1
//	tela.Spring(tela.Duration(500*time.Millisecond), tela.Bounce(0.25))
//
// Without options a spring has stiffness 100 and damping 10 (Motion's
// default, slightly bouncy); Bounce without a Duration takes 800 ms, and a
// Duration without Bounce a bounce of 0.3. Tweens take a [Duration]
// (default 300 ms) and an [Ease] (default CSS's "ease").
//
// Options that contradict each other or the hook (Ease on a spring,
// Stiffness and Duration together, spring options on UseTween), invalid
// numbers and invalid easing strings panic in dev mode (see [App].Dev) with
// what to write instead; otherwise they are ignored.
type AnimationOption = core.AnimOption

// UseSpring returns a [Value] that follows target with a spring: on the
// first build it holds the [Initial] option's value (target itself when
// there is none, so nothing moves) and, if target differs, starts moving
// to it; whenever a build passes a target that differs from the previous
// one, the spring retargets from where the value is, keeping its
// velocity, so an interrupted motion turns smoothly instead of stopping
// (Compose's animate*AsState, Motion's useSpring). T is the type of
// target, so a float32 target needs a typed value (float32(0), a float32
// variable). It is a Value like [UseValue]'s: read it in a deferred style
// closure ([TranslateFn], [OpacityFn], [WidthFn]...) and the element
// animates by repainting or laying out only, without rebuilding the
// component; the same *Value is returned on every build.
//
// The options are [AnimationOption]s: [Stiffness] and [Damping], or
// [Duration] and [Bounce]; without them, Motion's default spring
// (stiffness 100, damping 10). The spring ends when it comes to rest (its
// speed and its distance to the target are below Motion's thresholds), at
// exactly the target, and from then on the app requests no frames.
//
// A panel that slides in when open changes, and in from the left when it
// mounts open:
//
//	target := float32(-320)
//	if open {
//	    target = 0
//	}
//	x := tela.UseSpring(target, tela.Initial(float32(-320)), tela.Stiffness(300), tela.Damping(30))
//	return Div(Class("fixed inset-y-0 left-0 w-80 bg-white shadow-xl"),
//	    Style(tela.TranslateFn(func() (float32, float32) { return x.Get(), 0 })))
//
// With reduced motion ([App], or the system setting), or in a tree that
// no frame clock drives (telatest.Render, tela snapshot), the value jumps
// to each new target. Calling Set on the Value stops the spring; the next
// target change starts from the value set.
func UseSpring[T Animatable](target T, opts ...AnimationOption) *Value[T] {
	return core.UseSpring(target, opts...)
}

// UseTween is [UseSpring] with a tween: the value moves to each new
// target in a fixed [Duration] along an [Ease] curve (default 300 ms,
// CSS's "ease"), like a CSS transition of the value. A new target starts a
// new tween from where the value is (tweens have no velocity to keep).
// Like UseSpring, it starts at the [Initial] option's value, or at target.
//
//	op := tela.UseTween(target, tela.Duration(200*time.Millisecond), tela.Ease("ease-out"))
//	Div(Class("rounded-lg bg-slate-900 p-3 text-white"),
//	    Style(tela.OpacityFn(op.Get)), Text("Saved"))
//
// It takes only Duration, Ease, Initial and OnDone; spring options panic
// in dev mode.
func UseTween[T Animatable](target T, opts ...AnimationOption) *Value[T] {
	return core.UseTween(target, opts...)
}

// Initial sets the value [UseSpring] and [UseTween] hold on their first
// build, from which they move to their target: an entrance animation.
// Without it they start at the target, and nothing moves until the target
// changes. Later builds ignore it.
//
//	scale := tela.UseSpring(float32(1), tela.Initial(float32(0.8))) // pops in
//
// Its type must be the target's. AnimationOption is not generic, so the
// compiler cannot check that: a mismatch, such as
// tela.UseSpring(tela.Point{}, tela.Initial(float32(0))), panics in dev mode
// (see [App].Dev) and is ignored otherwise (the value starts at the
// target). It does not apply to [Animate], which starts where the Value
// is (dev mode panics), nor inside [Spring].
func Initial[T Animatable](v T) AnimationOption { return core.Initial(v) }

// Animate moves v to target: with a spring by default (Motion's default,
// or as [Stiffness], [Damping], [Bounce] or [Spring] say), or with a tween
// when the options are [Duration] and [Ease] only. v is any [Value] of an
// [Animatable] type, from [UseValue], [UseSpring] or [UseTween].
//
//	y := tela.UseValue[float32](0)
//	Button(OnClick(func(tela.MouseEvent) { tela.Animate(y, 200, tela.Spring(tela.Bounce(0.25))) }),
//	    Text("Drop"))
//	tela.Animate(y, 0, tela.Duration(300*time.Millisecond), tela.Ease("ease-in-out"))
//
// A new Animate on the same Value interrupts the running animation: the
// new one starts where the value is and, for a spring, at the speed it
// has, so a fling continues smoothly; a Set or Update of the Value stops
// the animation. Like Set, Animate is safe from any goroutine: the
// animation starts when the UI thread applies it, in order with the
// Value's Set and Update calls (an animation applied between two frames
// starts at the next one). Value.Latest does not anticipate animations:
// it returns the value they have reached.
//
// With reduced motion, or in a tree that no frame clock drives, the value
// jumps to target. An animation of a Value whose component unmounts ends.
func Animate[T Animatable](v *Value[T], target T, opts ...AnimationOption) {
	core.Animate(v, target, opts...)
}

// Stiffness sets the stiffness of a spring: how strongly it pulls the
// value to its target (default 100). Higher is faster. With [Damping] it
// defines the spring by its physics, so it cannot be combined with
// [Duration] or [Bounce]. It makes [Animate] use a spring.
func Stiffness(k float64) AnimationOption { return core.Stiffness(k) }

// Damping sets the damping of a spring: how strongly it resists motion
// (default 10). Low values bounce around the target; damping =
// 2*sqrt(stiffness) is critically damped (no overshoot), so Stiffness(300)
// with Damping(35) settles without bouncing. It makes [Animate] use a
// spring.
func Damping(c float64) AnimationOption { return core.Damping(c) }

// Bounce sets how bouncy a spring defined by its duration is (Motion's
// bounce): 0 does not overshoot, 0.25 bounces a little, values toward 1
// oscillate longer. Without a [Duration] the spring takes 800 ms. It makes
// [Animate] use a spring.
func Bounce(b float64) AnimationOption { return core.Bounce(b) }

// Duration sets how long a tween takes, or, for a spring ([UseSpring], or
// inside [Spring]), the time a spring found from it settles in (Motion's
// duration/bounce springs; it ends no earlier). A Duration of 0 jumps to
// the target: the value is stored at once, a tween's or a spring's, and an
// [OnDone] is queued right after, as for any target that applies at once.
func Duration(d time.Duration) AnimationOption { return core.Duration(d) }

// Easing is a CSS <easing-function> as a string: one of the constants
// [EaseLinear], [EaseIn], [EaseOut] and [EaseInOut], the keywords "ease",
// "step-start" and "step-end", [CubicBezier], or "steps(n[, position])".
// It is a string type, so a literal such as "ease-out" keeps compiling
// where an Easing is wanted.
type Easing string

// The easing keywords of CSS Easing Functions Level 1 that Tailwind and
// Motion also name. They are CSS's curves: Tailwind's ease-out utility is
// CubicBezier(0, 0, 0.2, 1), which is not EaseOut. The default of a tween
// is CSS's "ease", which has no constant.
const (
	// EaseLinear is "linear": constant speed.
	EaseLinear Easing = "linear"
	// EaseIn is "ease-in", cubic-bezier(0.42, 0, 1, 1): starts slow.
	EaseIn Easing = "ease-in"
	// EaseOut is "ease-out", cubic-bezier(0, 0, 0.58, 1): ends slow.
	EaseOut Easing = "ease-out"
	// EaseInOut is "ease-in-out", cubic-bezier(0.42, 0, 0.58, 1): slow at
	// both ends.
	EaseInOut Easing = "ease-in-out"
)

// CubicBezier returns the easing cubic-bezier(x1, y1, x2, y2): a Bezier
// curve from (0, 0) to (1, 1) with those control points. x1 and x2 must
// be from 0 to 1 (y1 and y2 may overshoot, for a bounce); [Ease] panics
// in dev mode otherwise.
//
//	tela.Ease(tela.CubicBezier(0, 0, 0.2, 1)) // Tailwind's ease-out
func CubicBezier(x1, y1, x2, y2 float32) Easing {
	f := func(v float32) string { return strconv.FormatFloat(float64(v), 'g', -1, 32) }
	return Easing("cubic-bezier(" + f(x1) + ", " + f(y1) + ", " + f(x2) + ", " + f(y2) + ")")
}

// Ease sets the easing curve of a tween, an [Easing]: a CSS
// <easing-function>, one of the keywords linear, ease, ease-in, ease-out,
// ease-in-out, step-start and step-end, [CubicBezier] or
// steps(n[, position]). The keywords are CSS's curves: Tailwind's
// ease-out utility is Ease(CubicBezier(0, 0, 0.2, 1)). An invalid string
// panics in dev mode with a suggestion (Ease("ease-out-cubic"): did you
// mean "ease-out"?); otherwise the tween uses "ease".
func Ease(e Easing) AnimationOption { return core.Ease(string(e)) }

// OnDone calls fn when the animation comes to rest at its target: once,
// on the UI thread, queued with the state updates like [Post], so it runs
// before the next build and state it sets is built in the frame that
// shows the target. It is not called when the animation is interrupted:
// by another Animate, Set or Update of the Value, by a new target of
// [UseSpring] or [UseTween], or by the unmount of the Value's component.
// When the target applies at once (reduced motion, a tree that no frame
// clock drives, [Duration](0), a value already at its target and at rest,
// a NaN target) fn is queued right after it applies. For UseSpring and
// UseTween it is the OnDone of the build that set the target that runs.
//
//	tela.Animate(x, 0, tela.Spring(), tela.OnDone(func() { setOpen.Set(false) }))
//
// It is how a component inside an h.Presence child ends its own exit:
// see [UsePresence].
func OnDone(fn func()) AnimationOption { return core.OnDone(fn) }

// Velocity sets the initial velocity of a spring, in the Value's units per
// second, component by component (a [Point]'s X and Y, a [Rect]'s X, Y,
// Width and Height). It replaces the velocity an interrupted animation
// would pass on: a released drag hands its pointer velocity to the spring
// that brings the element back:
//
//	OnDragEnd(func(e tela.DragEvent) {
//	    tela.Animate(pos, tela.Point{}, tela.Spring(), tela.Velocity(tela.Point{X: e.VelocityX, Y: e.VelocityY}))
//	})
//
// It makes [Animate] use a spring. Its type must be the Value's, and it
// does not apply to tweens ([UseTween], or Duration and Ease alone) nor to
// [Color] values, whose components move in a space (sRGB or Oklab,
// premultiplied) that depends on the two colors: those panic in dev mode
// and are ignored otherwise.
func Velocity[T Animatable](v T) AnimationOption { return core.Velocity(v) }

// Spring makes the animation a spring with the options given:
// [Stiffness] and [Damping], or [Duration] and [Bounce]. Spring() alone is
// Motion's default spring. It is how [Animate] gets a spring defined by
// its duration:
//
//	tela.Animate(y, 200, tela.Spring(tela.Duration(400*time.Millisecond), tela.Bounce(0.3)))
func Spring(opts ...AnimationOption) AnimationOption { return core.Spring(opts...) }

// UsePresence reports whether the component is present, for a component
// inside a child of an h.Presence: present is false while that child
// exits, after it was removed. The component then runs its own exit (an
// [Animate], saving a draft...) and calls done when it is finished; the
// child unmounts once every component that used UsePresence called done
// and the transitions of its h.Exit classes ended. done is the same
// function on every build, safe from any goroutine, and does nothing while
// the component is present (a call from a previous exit that was
// cancelled is ignored). Outside any Presence, present is always true.
//
// A toast that slides down and fades out before it unmounts: the tween to
// its exit position calls done when it arrives ([OnDone]; done does
// nothing while the toast is present, so the entrance may pass it too).
//
//	func Toast(msg string) tela.Node {
//	    present, done := tela.UsePresence()
//	    target := float32(0)
//	    if !present {
//	        target = 1
//	    }
//	    out := tela.UseTween(target, tela.Duration(200*time.Millisecond), tela.OnDone(done))
//	    return Div(Class("rounded-lg bg-slate-900 px-4 py-2 text-white"),
//	        Style(tela.TranslateFn(func() (float32, float32) { return 0, 40 * out.Get() }),
//	            tela.OpacityFn(func() float32 { return 1 - out.Get() })),
//	        Text(msg))
//	}
//
//	Presence(If(saved, tela.C(func() tela.Node { return Toast("Saved") })))
func UsePresence() (present bool, done func()) { return core.UsePresence() }

// UsePresenceTween is the entrance and the exit of a component inside a
// child of an h.Presence as one [Value]: a number that goes from 0 to 1
// when the component mounts and from 1 to 0 while its child exits, and
// that calls [UsePresence]'s done when the exit arrives, so the child
// unmounts then. Read it in deferred style closures and the component
// does not build while it moves.
//
// The options are those of [UseTween]: [Duration] (300 ms by default),
// [Ease], and [OnDone], which runs after each of the two motions, before
// done. With spring options ([Stiffness], [Damping], [Bounce],
// [Velocity], [Spring]) it is a [UseSpring] instead. The entrance starts
// from 0, as with [Initial]; the same *Value is returned on every build.
//
// A toast that slides up and fades in, and out again before it unmounts:
//
//	func Toast() tela.Node { return tela.C(toast) }
//
//	func toast() tela.Node {
//	    shown := tela.UsePresenceTween(tela.Duration(250*time.Millisecond), tela.Ease(tela.EaseOut))
//	    return Div(Class("fixed bottom-8 rounded-xl bg-slate-900 px-5 py-3 text-white"),
//	        Style(tela.TranslateFn(func() (float32, float32) { return 0, 24 * (1 - shown.Get()) }),
//	            tela.OpacityFn(shown.Get)),
//	        Text("Saved"))
//	}
//
//	Presence(If(saved, Toast()))
func UsePresenceTween(opts ...AnimationOption) *Value[float32] {
	return core.UsePresenceTween(opts...)
}
