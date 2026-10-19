package core

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/kuronosu/tela/internal/anim"
	"github.com/kuronosu/tela/internal/gfx"
)

// Explicit animations of Values (ARCHITECTURE section 12, ADR-080):
// UseSpring and UseTween follow a target that changes between builds, and
// Animate moves any UseValue from handlers, effects or goroutines. They
// all drive a Value with an anim.Motion advanced by the Tree's ticker in
// the animation phase, so a Value read in a deferred style closure
// (TranslateFn, OpacityFn, WidthFn) animates without building anything.
//
// # Retargeting
//
// A Value has at most one running animation (valueAnim). A new target
// replaces it: the new motion starts at the position and, for springs,
// with the velocity the old one has at the time the new one starts, so
// the value and its speed stay continuous (Motion's and Compose's
// interruption). A Set or Update of the Value stops the animation; the
// value is the one set.
//
// # When an animation starts
//
// A motion started while a frame runs (a build, the Values applied at the
// start of a Frame) starts at that frame's time, which is the time of the
// value shown. One started between frames (an event handler's Animate
// applied by the discrete event's flush, a goroutine's Animate applied on
// a wake) starts at the next frame, from where the running one is then.
//
// # Reduced motion and trees without a clock
//
// With reduced motion, or in a tree that no frame clock drives (Render),
// a new target is applied at once and running animations jump to their
// targets: no frames are spent. An animation that ends, or comes to rest,
// leaves the ticker, so an app is idle once its Values stop moving.

// Point is a point in logical pixels (tela.Point).
type Point struct{ X, Y float32 }

// Size is a size in logical pixels (tela.Size).
type Size struct{ Width, Height float32 }

// Animatable is the set of types UseSpring, UseTween and Animate
// interpolate: numbers, colors (as CSS transitions interpolate them),
// points, sizes and rectangles (component by component).
type Animatable interface {
	float32 | gfx.Color | Point | Size | Rect
}

// ---------------------------------------------------------------------
// Options.

type optKind uint8

const (
	optStiffness optKind = iota + 1
	optDamping
	optBounce
	optDuration
	optEase
	optSpring
)

// AnimOption configures an animation: its spring or its tween
// (tela.AnimationOption). The zero AnimOption changes nothing.
type AnimOption struct{ o *animOpt }

type animOpt struct {
	kind  optKind
	f     float64
	d     time.Duration
	curve anim.Curve
	// err says why the string given to Ease is not an easing function.
	err string
	// sub are the options of Spring.
	sub []AnimOption
}

// Stiffness sets the stiffness of a spring (default 100).
func Stiffness(k float64) AnimOption { return AnimOption{&animOpt{kind: optStiffness, f: k}} }

// Damping sets the damping of a spring (default 10).
func Damping(c float64) AnimOption { return AnimOption{&animOpt{kind: optDamping, f: c}} }

// Bounce sets the bounciness of a spring defined by its duration.
func Bounce(b float64) AnimOption { return AnimOption{&animOpt{kind: optBounce, f: b}} }

// Duration sets the duration of a tween, or of a spring defined by its
// duration.
func Duration(d time.Duration) AnimOption { return AnimOption{&animOpt{kind: optDuration, d: d}} }

// Ease sets the easing curve of a tween from a CSS <easing-function>.
func Ease(css string) AnimOption {
	c, err := anim.ParseEasing(css)
	o := &animOpt{kind: optEase, curve: c}
	if err != nil {
		o.err = easeError(css, err)
	}
	return AnimOption{o}
}

// Spring makes the animation a spring with the options given.
func Spring(opts ...AnimOption) AnimOption {
	return AnimOption{&animOpt{kind: optSpring, sub: opts}}
}

var easeKeywords = []string{"linear", "ease", "ease-in", "ease-out", "ease-in-out", "step-start", "step-end"}

// easeError is the message for a string that is not a CSS easing
// function, with what to write instead.
func easeError(css string, err error) string {
	s := strings.ToLower(strings.TrimSpace(css))
	tw := map[string]string{"in": "cubic-bezier(0.4, 0, 1, 1)", "out": "cubic-bezier(0, 0, 0.2, 1)", "in-out": "cubic-bezier(0.4, 0, 0.2, 1)"}
	if c, ok := tw[s]; ok {
		return fmt.Sprintf("Ease(%q): %q is Tailwind's easing token, not a CSS easing function; use Ease(%q) for CSS's curve, or Ease(%q) for Tailwind's ease-%s", css, s, "ease-"+s, c, s)
	}
	msg := "Ease(" + strconv.Quote(css) + "): " + err.Error()
	if !strings.Contains(s, "(") {
		// The longest keyword the string starts with ("ease-out-cubic"),
		// else the closest one ("ease-iout").
		sug := ""
		for _, k := range easeKeywords {
			if strings.HasPrefix(s, k) && len(k) > len(sug) {
				sug = k
			}
		}
		if sug == "" {
			sug = closestName(s, easeKeywords)
		}
		if sug != "" && sug != s {
			msg += fmt.Sprintf("; did you mean %q?", sug)
		}
	}
	return msg
}

// animMode is what kind of animation a caller asks for.
type animMode uint8

const (
	animAuto   animMode = iota // Animate: a spring unless tween options are given
	animSpring                 // UseSpring
	animTween                  // UseTween
)

// Defaults: Motion's spring (stiffness 100, damping 10, mass 1; 800 ms and
// bounce 0.3 for a spring defined by its duration) and a 300 ms tween
// with CSS's ease, as CSS transitions default to.
const (
	defaultSpringDuration = 800 * time.Millisecond
	defaultBounce         = 0.3
	defaultTween          = 300 * time.Millisecond
)

// resolveMotion turns the options of hook into the motion they describe.
// Mistakes (options that contradict each other or the hook, invalid
// values) panic in dev mode with what to write instead; otherwise the
// offending option is ignored, as Motion does.
func (t *Tree) resolveMotion(hook string, mode animMode, opts []AnimOption) anim.MotionSpec {
	problem := func(format string, args ...any) {
		if t.dev {
			panic("tela: " + hook + ": " + fmt.Sprintf(format, args...))
		}
	}
	var (
		stiff, damp, bounce, top []*animOpt
		dur, springDur           *time.Duration
		curve                    anim.Curve
		spring                   bool
	)
	_ = top
	for _, o := range opts {
		op := o.o
		if op == nil {
			continue
		}
		switch op.kind {
		case optStiffness:
			stiff = append(stiff, op)
		case optDamping:
			damp = append(damp, op)
		case optBounce:
			bounce = append(bounce, op)
		case optDuration:
			d := op.d
			dur = &d
		case optEase:
			if op.err != "" {
				problem("%s", op.err)
				continue
			}
			curve = op.curve
		case optSpring:
			spring = true
			for _, so := range op.sub {
				sp := so.o
				if sp == nil {
					continue
				}
				switch sp.kind {
				case optStiffness:
					stiff = append(stiff, sp)
				case optDamping:
					damp = append(damp, sp)
				case optBounce:
					bounce = append(bounce, sp)
				case optDuration:
					d := sp.d
					springDur = &d
				case optEase:
					problem("Ease does not apply to a spring, whose motion comes from Stiffness and Damping, or Duration and Bounce; remove it, or use a tween (Duration and Ease without Spring)")
				case optSpring:
					problem("Spring inside Spring: pass the spring options to one Spring")
				}
			}
		}
	}
	springOpts := spring || len(stiff) > 0 || len(damp) > 0 || len(bounce) > 0
	if mode == animTween && springOpts {
		problem("UseTween takes Duration and Ease; Stiffness, Damping, Bounce and Spring are for springs (use UseSpring)")
		springOpts = false
	}
	springy := mode == animSpring || springOpts || (mode == animAuto && dur == nil && curve == nil)
	if mode == animTween {
		springy = false
	}
	if !springy {
		d := defaultTween
		if dur != nil {
			d = *dur
		}
		if d < 0 {
			problem("Duration(%v) is negative; pass a duration of 0 or more, such as 300*time.Millisecond", d)
			d = 0
		}
		if curve == nil {
			curve = anim.Ease
		}
		return anim.MotionSpec{Duration: d, Curve: curve}
	}
	if curve != nil {
		problem("Ease applies to tweens; a spring's motion comes from Stiffness and Damping, or Duration and Bounce: remove Ease, or the spring options for a tween")
	}
	if springDur == nil {
		springDur = dur
	}
	last := func(os []*animOpt) (float64, bool) {
		if len(os) == 0 {
			return 0, false
		}
		return os[len(os)-1].f, true
	}
	k, hasK := last(stiff)
	c, hasC := last(damp)
	b, hasB := last(bounce)
	if hasK || hasC {
		if springDur != nil || hasB {
			problem("Stiffness and Damping set a spring's physics, Duration and Bounce derive it: use one pair or the other")
		}
		if !hasK {
			k = 100
		} else if !(k > 0) || math.IsInf(k, 0) {
			problem("Stiffness(%v): the stiffness must be a positive number, such as 300", k)
			k = 100
		}
		if !hasC {
			c = 10
		} else if !(c >= 0) || math.IsInf(c, 0) {
			problem("Damping(%v): the damping must be 0 or more, such as 30", c)
			c = 10
		}
		return anim.MotionSpec{Spring: true, Physics: anim.Spring{Stiffness: k, Damping: c, Mass: 1}}
	}
	if springDur != nil || hasB {
		d := defaultSpringDuration
		if springDur != nil {
			d = *springDur
		}
		if d < 0 {
			problem("Duration(%v) is negative; pass a duration of 0 or more, such as 500*time.Millisecond", d)
			d = defaultSpringDuration
		}
		if !hasB {
			b = defaultBounce
		} else if math.IsNaN(b) || math.IsInf(b, 0) {
			problem("Bounce(%v): the bounce must be a number, from 0 (no bounce) toward 1 (very bouncy)", b)
			b = defaultBounce
		}
		// SpringForDuration clamps the duration to Motion's 10 ms to 10 s.
		md := min(max(d, 10*time.Millisecond), 10*time.Second)
		return anim.MotionSpec{Spring: true, Physics: anim.SpringForDuration(d, b), MinDuration: md}
	}
	return anim.MotionSpec{Spring: true, Physics: anim.DefaultSpring()}
}

// ---------------------------------------------------------------------
// Values of the animatable types as components.

// vecOf returns the components of x; colors in the space oklab says
// (anim.ColorVec).
func vecOf[T Animatable](x T, oklab bool) anim.Vec {
	switch x := any(x).(type) {
	case float32:
		return anim.Vec{N: 1, C: [4]float64{float64(x)}}
	case gfx.Color:
		return anim.ColorVec(x, oklab)
	case Point:
		return anim.Vec{N: 2, C: [4]float64{float64(x.X), float64(x.Y)}}
	case Size:
		return anim.Vec{N: 2, C: [4]float64{float64(x.Width), float64(x.Height)}}
	case Rect:
		return anim.Vec{N: 4, C: [4]float64{float64(x.X), float64(x.Y), float64(x.Width), float64(x.Height)}}
	}
	panic("unreachable")
}

// valueOf converts components back to a T (the inverse of vecOf).
func valueOf[T Animatable](v anim.Vec, oklab bool) T {
	var zero T
	var out any
	switch any(zero).(type) {
	case float32:
		out = float32(v.C[0])
	case gfx.Color:
		out = anim.VecColor(v, oklab)
	case Point:
		out = Point{float32(v.C[0]), float32(v.C[1])}
	case Size:
		out = Size{float32(v.C[0]), float32(v.C[1])}
	case Rect:
		out = Rect{float32(v.C[0]), float32(v.C[1]), float32(v.C[2]), float32(v.C[3])}
	}
	return out.(T) // perf: boxes the value once per frame
}

// sameValue reports whether a and b are the same value, a NaN component
// being the same as itself (so a NaN target does not restart its
// animation on every build).
func sameValue[T Animatable](a, b T) bool {
	if a == b {
		return true
	}
	return rawVec(a).Same(rawVec(b))
}

// rawVec returns the components of x as stored (colors not premultiplied).
func rawVec[T Animatable](x T) anim.Vec {
	if c, ok := any(x).(gfx.Color); ok {
		return anim.Vec{N: 4, C: [4]float64{float64(c.R), float64(c.G), float64(c.B), float64(c.A)}}
	}
	return vecOf(x, false)
}

// colorSpace returns, for colors, whether a motion from a to b moves in
// Oklab (anim.ColorsOklab); false for the other types.
func colorSpace[T Animatable](a, b T) bool {
	ca, ok := any(a).(gfx.Color)
	if !ok {
		return false
	}
	return anim.ColorsOklab(ca, any(b).(gfx.Color))
}

// ---------------------------------------------------------------------
// The animation of a Value.

// valueAnimator is the running animation of a Value, held by the Value
// (whose type parameter is not constrained to Animatable).
type valueAnimator interface {
	anim.Animator
	// stop ends the animation where it is (a Set, an unmount).
	stop()
}

// valueAnim animates one Value. It lives on the UI thread.
type valueAnim[T Animatable] struct {
	v *Value[T]
	// cur is the motion in progress (when running), toward toT, in the
	// color space oklab.
	cur     anim.Motion
	running bool
	toT     T
	oklab   bool
	// next is a motion that starts at the next frame (or at once inside
	// a frame), from wherever cur is then.
	next    *anim.MotionSpec
	nextTo  T
	hasNext bool
}

// animateTo starts an animation of v toward target with spec, replacing
// the running one, on the UI thread.
func animateTo[T Animatable](v *Value[T], target T, spec anim.MotionSpec) {
	t := v.el.tree
	if !v.el.mounted {
		return
	}
	if !t.anim.driven || t.cx.ReducedMotion {
		// No clock (Render) or reduced motion: the target at once.
		v.stopAnim()
		v.storeUI(target)
		return
	}
	a, _ := v.anim.(*valueAnim[T])
	if a == nil {
		a = &valueAnim[T]{v: v}
		v.anim = a
	}
	s := spec
	a.next, a.nextTo, a.hasNext = &s, target, true
	if t.inFrame && t.anim.advanced {
		// Inside a frame the ticker's time is the frame's: start now.
		a.begin(t.anim.tk.Now())
		if !a.running {
			return
		}
	}
	t.anim.tk.Add(a)
	if !t.inFrame {
		t.requestFrame()
	}
}

// begin starts the pending motion at now, from the position and velocity
// of the running one (or the Value's current value, at rest).
func (a *valueAnim[T]) begin(now time.Duration) {
	spec, target := *a.next, a.nextTo
	a.next, a.hasNext = nil, false
	cur := *a.v.cur.Load()
	oklab := colorSpace(cur, target)
	var pos, vel anim.Vec
	if a.running {
		x, v, done := a.cur.At(now)
		switch {
		case done:
			pos = vecOf(a.toT, oklab)
		case oklab != a.oklab:
			// Another color space: the position carries over, the
			// velocity does not.
			pos = anim.ColorVec(anim.VecColor(x, a.oklab), oklab)
		default:
			pos, vel = x, v
		}
	} else {
		pos = vecOf(cur, oklab)
	}
	vel.N = pos.N
	to := vecOf(target, oklab)
	if !spec.Spring {
		vel = anim.Vec{N: pos.N} // tweens do not carry a velocity
	}
	a.running = false
	if !pos.Finite() || !to.Finite() || !vel.Finite() {
		a.v.storeUI(target) // NaN or infinity: nothing to interpolate
		return
	}
	if pos.Same(to) && vel.Same(anim.Vec{N: pos.N}) {
		a.v.storeUI(target) // already there, at rest
		return
	}
	a.cur = anim.NewMotion(spec, pos, to, vel, now)
	a.toT, a.oklab, a.running = target, oklab, true
}

// Advance implements anim.Animator: it moves the Value to its value at
// now.
func (a *valueAnim[T]) Advance(now time.Duration) anim.State {
	v := a.v
	if !v.el.mounted || v.anim != valueAnimator(a) {
		return anim.Done
	}
	if a.hasNext {
		a.begin(now)
	}
	if !a.running {
		return anim.Done
	}
	if v.el.tree.cx.ReducedMotion {
		a.running = false
		v.storeUI(a.toT)
		return anim.Done
	}
	x, _, done := a.cur.At(now)
	if done {
		a.running = false
		v.storeUI(a.toT) // exactly the target
		return anim.Done
	}
	v.storeUI(valueOf[T](x, a.oklab))
	return anim.Running
}

// stop implements valueAnimator.
func (a *valueAnim[T]) stop() {
	a.running, a.next, a.hasNext = false, nil, false
	a.v.el.tree.anim.tk.Remove(a)
}

// stopAnim stops the Value's animation, if any (UI thread).
func (v *Value[T]) stopAnim() {
	if v.anim != nil {
		v.anim.stop()
	}
}

// ---------------------------------------------------------------------
// The API.

// Animate animates v to target (tela.Animate): a spring by default, or as
// the options say. Safe from any goroutine: the animation starts when the
// UI thread applies it, in order with the Value's Set and Update calls.
func Animate[T Animatable](v *Value[T], target T, opts ...AnimOption) {
	v.load("Animate")
	t := v.el.tree
	spec := t.resolveMotion("Animate", animAuto, opts)
	t.enqueue(update{el: v.el, set: func() bool {
		animateTo(v, target, spec)
		return false
	}})
}

// motionSlot is the slot of one UseSpring or UseTween call.
type motionSlot[T Animatable] struct {
	v      *Value[T]
	target T
}

func (s *motionSlot[T]) unmount(t *Tree) { s.v.unmount(t) }

// UseSpring returns a Value that follows target with a spring
// (tela.UseSpring).
func UseSpring[T Animatable](initial, target T, opts ...AnimOption) *Value[T] {
	return useMotion(hookSpring, animSpring, initial, target, opts)
}

// UseTween returns a Value that follows target with a tween
// (tela.UseTween).
func UseTween[T Animatable](initial, target T, opts ...AnimOption) *Value[T] {
	return useMotion(hookTween, animTween, initial, target, opts)
}

func useMotion[T Animatable](kind hookKind, mode animMode, initial, target T, opts []AnimOption) *Value[T] {
	el, i := nextSlot(kind)
	var s *motionSlot[T]
	if i < len(el.hooks) {
		var ok bool
		s, ok = el.hooks[i].(*motionSlot[T])
		if !ok {
			el.tree.slotMismatch(el, i, kind.String()+"["+typeName[T]()+"]")
		}
	} else {
		s = &motionSlot[T]{v: newValue(el, initial), target: initial}
		el.hooks = append(el.hooks, s)
	}
	spec := el.tree.resolveMotion(kind.String(), mode, opts)
	if !sameValue(target, s.target) {
		s.target = target
		animateTo(s.v, target, spec)
	}
	return s.v
}
