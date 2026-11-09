package core

import (
	"testing"
	"time"
)

// animation-delay (ADR-248): delay-*, animation-delay-* and
// [animation-delay:...] stagger the animate-* classes.

func TestAnimationDelayStaggersKeyframes(t *testing.T) {
	x := newInput(t, div(
		div(Key("none"), Class("size-8 animate-pulse")),
		div(Key("delay"), Class("size-8 animate-pulse delay-500")),
		div(Key("plugin"), Class("size-8 animate-pulse animation-delay-500")),
		div(Key("arb"), Class("size-8 animate-pulse [animation-delay:500ms]")),
		div(Key("neg"), Class("size-8 animate-pulse [animation-delay:-1s]")),
		div(Key("negclass"), Class("size-8 animate-pulse -animation-delay-1000")),
	), 200, 200)
	x.frame()
	t0 := x.cs("none").anim.kf.start

	// 250 ms in: the delayed ones have not started (no fill: base style),
	// the plain one is under way.
	x.advanceTo(t0 + 250*time.Millisecond)
	for _, k := range []string{"delay", "plugin", "arb"} {
		if o := x.cs(k).cs.Opacity; o != 1 {
			t.Errorf("%s at 250 ms (delay 500 ms): opacity %v, want 1 (not started)", k, o)
		}
	}
	if o := x.cs("none").cs.Opacity; o >= 1 {
		t.Errorf("plain pulse at 250 ms: opacity %v, want below 1", o)
	}

	// 1.5 s in: a delay of 500 ms puts the pulse at 1 s, its lowest opacity.
	x.advanceTo(t0 + 1500*time.Millisecond)
	for _, k := range []string{"delay", "plugin", "arb"} {
		if o := x.cs(k).cs.Opacity; !near32(o, 0.5, 1e-3) {
			t.Errorf("%s at 1.5 s: opacity %v, want 0.5", k, o)
		}
	}
	// A negative delay starts partway: 2 s in, the pulse is 3 s into its
	// 2 s cycle, which is 1 s into the second one.
	x.advanceTo(t0 + 2*time.Second)
	if o := x.cs("neg").cs.Opacity; !near32(o, 0.5, 1e-3) {
		t.Errorf("negative delay at 2 s: opacity %v, want 0.5", o)
	}
	if a, b := x.cs("neg").cs.Opacity, x.cs("negclass").cs.Opacity; a != b {
		t.Errorf("-animation-delay-1000 (%v) differs from [animation-delay:-1s] (%v)", b, a)
	}
}

func TestAnimationDelayOverridesTheShorthand(t *testing.T) {
	x := newInput(t, div(
		div(Key("a"), Class("size-8 animate-[pulse_2s_ease-in-out_1s_infinite] [animation-delay:0s]")),
		div(Key("b"), Class("size-8 animate-[pulse_2s_ease-in-out_1s_infinite]")),
	), 200, 200)
	x.frame()
	t0 := x.cs("a").anim.kf.start
	x.advanceTo(t0 + 500*time.Millisecond)
	if x.cs("a").cs.Opacity == 1 {
		t.Error("animation-delay:0s should replace the 1 s delay of the shorthand")
	}
	if o := x.cs("b").cs.Opacity; o != 1 {
		t.Errorf("the shorthand's own delay: opacity %v at 500 ms", o)
	}
}

func TestAnimationDelayChangeDoesNotRestart(t *testing.T) {
	var set Setter[string]
	x := newInput(t, Component(func() Node {
		cls, s := UseState("animate-pulse delay-1000")
		set = s
		return div(div(Key("a"), Class("size-8 "+cls)))
	}), 200, 200)
	x.frame()
	start := x.cs("a").anim.kf.start
	x.advanceTo(start + 1500*time.Millisecond)
	set.Set("animate-pulse")
	x.frame()
	if got := x.cs("a").anim.kf.start; got != start {
		t.Errorf("changing the delay restarted the animation: start %v, was %v", got, start)
	}
	if d := x.cs("a").anim.kf.spec.Delay; d != 0 {
		t.Errorf("delay after dropping delay-1000: %v, want 0", d)
	}
}
