package tela

import (
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
// [Velocity] and [Spring], which groups spring options, and [OnDone]. The
// zero AnimationOption changes nothing.
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
// first build it holds initial and, if target differs, starts moving to it;
// whenever a build passes a target that differs from the previous one, the
// spring retargets from where the value is, keeping its velocity, so an
// interrupted motion turns smoothly instead of stopping (Compose's
// animate*AsState, Motion's useSpring). It is a Value like [UseValue]'s:
// read it in a deferred style closure ([TranslateFn], [OpacityFn],
// [WidthFn]...) and the element animates by repainting or laying out
// only, without rebuilding the component; the same *Value is returned on
// every build.
//
// The options are [AnimationOption]s: [Stiffness] and [Damping], or
// [Duration] and [Bounce]; without them, Motion's default spring
// (stiffness 100, damping 10). The spring ends when it comes to rest (its
// speed and its distance to the target are below Motion's thresholds), at
// exactly the target, and from then on the app requests no frames.
//
// A panel that slides in when open changes:
//
//	target := float32(-320)
//	if open {
//	    target = 0
//	}
//	x := tela.UseSpring(float32(-320), target, tela.Stiffness(300), tela.Damping(30))
//	return Div(Class("fixed inset-y-0 left-0 w-80 bg-white shadow-xl"),
//	    Style(tela.TranslateFn(func() (float32, float32) { return x.Get(), 0 })))
//
// With reduced motion ([App], or the system setting), or in a tree that
// no frame clock drives (telatest.Render, tela snapshot), the value jumps
// to each new target. Calling Set on the Value stops the spring; the next
// target change starts from the value set.
func UseSpring[T Animatable](initial, target T, opts ...AnimationOption) *Value[T] {
	return core.UseSpring(initial, target, opts...)
}

// UseTween is [UseSpring] with a tween: the value moves to each new
// target in a fixed [Duration] along an [Ease] curve (default 300 ms,
// CSS's "ease"), like a CSS transition of the value. A new target starts a
// new tween from where the value is (tweens have no velocity to keep).
//
//	op := tela.UseTween(float32(0), target, tela.Duration(200*time.Millisecond), tela.Ease("ease-out"))
//	Div(Class("rounded-lg bg-slate-900 p-3 text-white"),
//	    Style(tela.OpacityFn(op.Get)), Text("Saved"))
//
// It takes only Duration and Ease; spring options panic in dev mode.
func UseTween[T Animatable](initial, target T, opts ...AnimationOption) *Value[T] {
	return core.UseTween(initial, target, opts...)
}

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

// Ease sets the easing curve of a tween, a CSS <easing-function>: one of
// the keywords linear, ease, ease-in, ease-out, ease-in-out, step-start
// and step-end, cubic-bezier(x1, y1, x2, y2) or steps(n[, position]).
// The keywords are CSS's curves: Tailwind's ease-out utility is
// Ease("cubic-bezier(0, 0, 0.2, 1)"). An invalid string panics in dev
// mode with a suggestion (Ease("ease-out-cubic"): did you mean
// "ease-out"?); otherwise the tween uses "ease".
func Ease(css string) AnimationOption { return core.Ease(css) }

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
//	    out := tela.UseTween(float32(0), target, tela.Duration(200*time.Millisecond), tela.OnDone(done))
//	    return Div(Class("rounded-lg bg-slate-900 px-4 py-2 text-white"),
//	        Style(tela.TranslateFn(func() (float32, float32) { return 0, 40 * out.Get() }),
//	            tela.OpacityFn(func() float32 { return 1 - out.Get() })),
//	        Text(msg))
//	}
//
//	Presence(If(saved, tela.C(func() tela.Node { return Toast("Saved") })))
func UsePresence() (present bool, done func()) { return core.UsePresence() }
