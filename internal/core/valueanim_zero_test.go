package core

import (
	"testing"
	"time"
)

// TestDurationZeroIsImmediate: Duration(0) stores the target at once, for
// tweens and for springs defined by their duration, like an immediate
// target: no frame of animation, OnDone queued right after.
func TestDurationZeroIsImmediate(t *testing.T) {
	cases := []struct {
		name  string
		tween bool
		opts  []AnimOption
	}{
		{"UseTween", true, []AnimOption{Duration(0)}},
		{"UseTween with Ease", true, []AnimOption{Duration(0), Ease("linear")}},
		{"UseSpring", false, []AnimOption{Duration(0)}},
		{"UseSpring with Bounce", false, []AnimOption{Duration(0), Bounce(0.5)}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			var m motionProbe
			done := 0
			opts := append(c.opts, OnDone(func() { done++ }))
			x := newHarness(t, m.comp(c.tween, opts...), false)
			m.set.Set(100)
			x.frame() // builds with the new target
			if got := m.v.Get(); got != 100 {
				t.Errorf("after the build that set the target: %v, want 100 at once", got)
			}
			if m.v.anim != nil && m.v.anim.(*valueAnim[float32]).running {
				t.Error("an animation runs")
			}
			x.frame() // runs the queued OnDone
			if done != 1 {
				t.Errorf("OnDone ran %d times, want 1", done)
			}
			if !x.quiet() {
				t.Error("not idle")
			}
		})
	}
	animates := []struct {
		name string
		opts []AnimOption
	}{
		{"Animate tween", []AnimOption{Duration(0), Ease("ease-in")}},
		{"Animate spring", []AnimOption{Spring(Duration(0))}},
		{"Animate spring with Bounce", []AnimOption{Spring(Duration(0), Bounce(0.2))}},
	}
	for _, c := range animates {
		t.Run(c.name, func(t *testing.T) {
			var v *Value[float32]
			x := newHarness(t, Component(func() Node {
				v = UseValue(float32(0))
				return div()
			}), false)
			done := 0
			Animate(v, 50, append(c.opts, OnDone(func() { done++ }))...)
			x.b.Drain(x.h) // the frame that applies it
			if got := v.Get(); got != 50 {
				t.Errorf("after the frame that applied it: %v, want 50", got)
			}
			x.settle()
			if done != 1 || v.Get() != 50 || !x.quiet() {
				t.Errorf("OnDone ran %d times, value %v, idle %v", done, v.Get(), x.quiet())
			}
		})
	}
	// A positive duration still animates.
	var m motionProbe
	x := newHarness(t, m.comp(true, Duration(100*time.Millisecond)), false)
	m.set.Set(100)
	x.frame()
	x.frame()
	if got := m.v.Get(); got <= 0 || got >= 100 {
		t.Errorf("a 100 ms tween after a frame: %v, want between 0 and 100", got)
	}
}
