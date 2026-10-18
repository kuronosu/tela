package anim

import (
	"time"
)

// Timing is the timing of a transition: transition-duration,
// transition-delay and transition-timing-function.
type Timing struct {
	Duration time.Duration
	Delay    time.Duration
	Curve    Curve
}

// Runs reports whether a transition with this timing starts at all: CSS
// Transitions Level 1 requires a combined duration (the duration, not
// below 0, plus the delay) above 0.
func (t Timing) Runs() bool { return max(t.Duration, 0)+t.Delay > 0 }

// Transition is one running CSS transition of a property group, in the
// terms of CSS Transitions Level 1, section 3: the time it starts
// interpolating (after its delay), its duration and timing function, and
// the reversing shortening factor that makes a reversed transition shorter.
// The values it interpolates between belong to the caller (the style
// before the change and the style it transitions to).
type Transition struct {
	Prop Prop
	// Start is when the interpolation starts: the time of the style change
	// plus the delay. Before it the transition holds its start value.
	Start    time.Duration
	Duration time.Duration
	Curve    Curve
	// ReverseFactor is the reversing shortening factor, in (0, 1]; 1 for a
	// transition that does not reverse another.
	ReverseFactor float64
}

// Begin returns the transition that starts for prop at the style change
// event of time now with the given timing.
func Begin(prop Prop, t Timing, now time.Duration) Transition {
	return Transition{Prop: prop, Start: now + t.Delay, Duration: max(t.Duration, 0), Curve: curveOrLinear(t.Curve), ReverseFactor: 1}
}

func curveOrLinear(c Curve) Curve {
	if c == nil {
		return Linear{}
	}
	return c
}

// End returns the time the transition finishes.
func (tr *Transition) End() time.Duration { return tr.Start + tr.Duration }

// Progress returns the eased progress of the transition at now (0 before it
// starts, 1 when it ends) and whether it has finished.
func (tr *Transition) Progress(now time.Duration) (eased float64, done bool) {
	if now >= tr.Start+tr.Duration {
		return 1, true
	}
	if now <= tr.Start {
		return 0, false
	}
	x := float64(now-tr.Start) / float64(tr.Duration)
	return tr.Curve.At(x), false
}

// Reverse returns the transition that replaces tr, which is still running
// at now, when the style changes back to the value tr started from
// (CSS Transitions Level 1, 3.3, "reversing of transitions"): it starts
// from the current value and takes only as long as the way back, so that a
// hover that ends halfway through returns in half the time.
func (tr *Transition) Reverse(t Timing, now time.Duration) Transition {
	eased, _ := tr.Progress(now)
	factor := float64(eased*tr.ReverseFactor) + (1 - tr.ReverseFactor)
	factor = min(max(abs(factor), 0), 1)
	start := now + t.Delay
	if t.Delay < 0 {
		start = now + time.Duration(float64(factor)*float64(t.Delay))
	}
	return Transition{
		Prop:          tr.Prop,
		Start:         start,
		Duration:      time.Duration(float64(factor) * float64(max(t.Duration, 0))),
		Curve:         curveOrLinear(t.Curve),
		ReverseFactor: factor,
	}
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
}
