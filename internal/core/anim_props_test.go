package core

import (
	"math"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/anim"
)

// duration-*, ease-*, repeat-*, direction-* and fill-mode-* also act on
// animate-* animations, and the arbitrary animation longhands do (ADR-256).

func TestAnimationLonghandsReachTheSpec(t *testing.T) {
	x := newInput(t, div(
		div(Key("base"), Class("size-8 animate-spin")),
		div(Key("dur"), Class("size-8 animate-spin duration-300")),
		div(Key("animdur"), Class("size-8 animate-spin animation-duration-[2s]")),
		div(Key("arbdur"), Class("size-8 animate-spin [animation-duration:0.5s]")),
		div(Key("ease"), Class("size-8 animate-spin ease-in-out")),
		div(Key("arbease"), Class("size-8 animate-spin [animation-timing-function:cubic-bezier(0.1,0.2,0.3,0.4)]")),
		div(Key("repeat"), Class("size-8 animate-spin repeat-3")),
		div(Key("inf"), Class("size-8 animate-[spin_1s_linear_2] repeat-infinite")),
		div(Key("arbrepeat"), Class("size-8 animate-spin [animation-iteration-count:5]")),
		div(Key("dir"), Class("size-8 animate-spin direction-alternate-reverse")),
		div(Key("arbdir"), Class("size-8 animate-spin [animation-direction:reverse]")),
		div(Key("fill"), Class("size-8 animate-spin fill-mode-both")),
		div(Key("arbfill"), Class("size-8 animate-spin [animation-fill-mode:forwards]")),
		div(Key("initial"), Class("size-8 animate-[spin_1s_linear_infinite_reverse_both] repeat-initial direction-initial fill-mode-initial")),
	), 200, 200)
	x.frame()
	spec := func(k string) anim.Spec { return x.cs(k).anim.kf.spec }
	base := spec("base")
	if base.Duration != time.Second || !math.IsInf(base.Iterations, 1) {
		t.Fatalf("animate-spin is %+v", base)
	}
	if d := spec("dur").Duration; d != 300*time.Millisecond {
		t.Errorf("duration-300: %v", d)
	}
	if d := spec("animdur").Duration; d != 2*time.Second {
		t.Errorf("animation-duration-[2s]: %v", d)
	}
	if d := spec("arbdur").Duration; d != 500*time.Millisecond {
		t.Errorf("[animation-duration:0.5s]: %v", d)
	}
	if c := spec("ease").Curve; c == nil || c == base.Curve {
		t.Errorf("ease-in-out: curve %v, was %v", c, base.Curve)
	}
	if c, ok := spec("arbease").Curve.(anim.CubicBezier); !ok || !near32(float32(c.X1), 0.1, 1e-6) || !near32(float32(c.Y2), 0.4, 1e-6) {
		t.Errorf("[animation-timing-function:cubic-bezier(...)]: %v", spec("arbease").Curve)
	}
	if n := spec("repeat").Iterations; n != 3 {
		t.Errorf("repeat-3: %v", n)
	}
	if n := spec("inf").Iterations; !math.IsInf(n, 1) {
		t.Errorf("repeat-infinite over a shorthand with 2 iterations: %v", n)
	}
	if n := spec("arbrepeat").Iterations; n != 5 {
		t.Errorf("[animation-iteration-count:5]: %v", n)
	}
	if d := spec("dir").Direction; d != anim.DirectionAlternateReverse {
		t.Errorf("direction-alternate-reverse: %v", d)
	}
	if d := spec("arbdir").Direction; d != anim.DirectionReverse {
		t.Errorf("[animation-direction:reverse]: %v", d)
	}
	if f := spec("fill").Fill; f != anim.FillBoth {
		t.Errorf("fill-mode-both: %v", f)
	}
	if f := spec("arbfill").Fill; f != anim.FillForwards {
		t.Errorf("[animation-fill-mode:forwards]: %v", f)
	}
	if s := spec("initial"); s.Iterations != 1 || s.Direction != anim.DirectionNormal || s.Fill != anim.FillNone {
		t.Errorf("repeat-initial, direction-initial and fill-mode-initial: %+v", s)
	}
}

func TestAnimationDurationAndRepeatChangeTheMotion(t *testing.T) {
	x := newInput(t, div(
		div(Key("plain"), Class("size-8 animate-pulse")),
		div(Key("fast"), Class("size-8 animate-pulse duration-1000")),
		div(Key("once"), Class("size-8 animate-pulse repeat-1")),
	), 200, 200)
	x.frame()
	t0 := x.cs("plain").anim.kf.start
	// The pulse is at its lowest opacity half a cycle in: 1 s for the 2 s
	// default, 500 ms with duration-1000.
	x.advanceTo(t0 + 500*time.Millisecond)
	if o := x.cs("fast").cs.Opacity; !near32(o, 0.5, 1e-3) {
		t.Errorf("duration-1000 at 500 ms: opacity %v, want 0.5", o)
	}
	if o := x.cs("plain").cs.Opacity; near32(o, 0.5, 1e-2) {
		t.Errorf("the plain pulse is already at its lowest at 500 ms: %v", o)
	}
	// One iteration of 2 s: over at 2.5 s, while the plain one repeats.
	x.advanceTo(t0 + 2500*time.Millisecond)
	if o := x.cs("once").cs.Opacity; o != 1 {
		t.Errorf("repeat-1 at 2.5 s: opacity %v, want 1 (finished)", o)
	}
	if o := x.cs("plain").cs.Opacity; o >= 1 {
		t.Errorf("the endless pulse at 2.5 s: opacity %v, want under 1", o)
	}
}

func TestAnimationDirectionReverses(t *testing.T) {
	x := newInput(t, div(
		div(Key("fwd"), Class("size-8 animate-spin ease-linear")),
		div(Key("rev"), Class("size-8 animate-spin ease-linear direction-reverse")),
	), 200, 200)
	x.frame()
	t0 := x.cs("fwd").anim.kf.start
	x.advanceTo(t0 + 250*time.Millisecond)
	fwd := x.cs("fwd").cs.Transform
	rev := x.cs("rev").cs.Transform
	if fwd == rev {
		t.Fatalf("direction-reverse did not change the motion: %v", rev)
	}
	// Reversed, 250 ms in is where the normal animation is at 750 ms.
	x.advanceTo(t0 + 750*time.Millisecond)
	if got := x.cs("fwd").cs.Transform; got != rev {
		t.Errorf("reverse at 250 ms is %v, the normal at 750 ms is %v", rev, got)
	}
}

func TestAnimationFillModeBackwardsHoldsTheFirstFrame(t *testing.T) {
	x := newInput(t, div(
		div(Key("none"), Class("size-8 animate-bounce delay-500")),
		div(Key("back"), Class("size-8 animate-bounce delay-500 fill-mode-backwards")),
	), 200, 200)
	x.frame()
	t0 := x.cs("none").anim.kf.start
	x.advanceTo(t0 + 250*time.Millisecond)
	if x.cs("none").cs.Transform == x.cs("back").cs.Transform {
		t.Errorf("during the delay: without a fill %v, with fill-mode-backwards %v (the first keyframe)",
			x.cs("none").cs.Transform, x.cs("back").cs.Transform)
	}
}

func TestAnimationLonghandChangeDoesNotRestart(t *testing.T) {
	var set Setter[string]
	x := newInput(t, Component(func() Node {
		cls, s := UseState("animate-pulse")
		set = s
		return div(div(Key("a"), Class("size-8 "+cls)))
	}), 200, 200)
	x.frame()
	start := x.cs("a").anim.kf.start
	x.advanceTo(start + 300*time.Millisecond)
	set.Set("animate-pulse duration-500 repeat-2")
	x.frame()
	k := x.cs("a").anim.kf
	if k.start != start {
		t.Errorf("changing the longhands restarted the animation: start %v, was %v", k.start, start)
	}
	if k.spec.Duration != 500*time.Millisecond || k.spec.Iterations != 2 {
		t.Errorf("spec after the change: %+v", k.spec)
	}
	set.Set("animate-pulse")
	x.frame()
	if k := x.cs("a").anim.kf; k.spec.Duration != 2*time.Second || !math.IsInf(k.spec.Iterations, 1) {
		t.Errorf("dropping the classes brings back the shorthand's values: %+v", k.spec)
	}
}
