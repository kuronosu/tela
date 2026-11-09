package core

import (
	"testing"
	"time"
)

// animation-play-state (paused, running, [animation-play-state:...]) and a
// duration that changes while the animation runs (ADR-263).

func TestPausedAnimationHoldsAndResumes(t *testing.T) {
	var set Setter[string]
	x := newInput(t, Component(func() Node {
		cls, s := UseState("animate-spin ease-linear")
		set = s
		return div(div(Key("a"), Class("size-8 "+cls)))
	}), 200, 200)
	x.frame()
	t0 := x.cs("a").anim.kf.start
	x.advanceTo(t0 + 250*time.Millisecond)
	if r := x.cs("a").cs.Transform.Rotate; !near32(r, 90, 0.01) {
		t.Fatalf("spin at 250 ms: %v degrees", r)
	}
	set.Set("animate-spin ease-linear paused")
	x.frame()
	held := x.cs("a").cs.Transform.Rotate // the frame of the change moved the clock a little
	if held < 90 || held > 110 {
		t.Fatalf("the spin paused at %v degrees", held)
	}
	// A second later it is where it was, and nothing asks for frames.
	x.advanceTo(t0 + 1250*time.Millisecond)
	if r := x.cs("a").cs.Transform.Rotate; r != held {
		t.Errorf("paused spin moved: %v degrees, was %v", r, held)
	}
	x.b.Drain(x.h)
	if x.tree().Animating() || x.w.FrameRequested() {
		t.Errorf("a paused animation asks for frames: animating %v, requested %v", x.tree().Animating(), x.w.FrameRequested())
	}
	// Running again continues from the 250 ms it had run.
	set.Set("animate-spin ease-linear running")
	x.frame()
	resumed := x.b.Now()
	if r := x.cs("a").cs.Transform.Rotate; !near32(r, held, 0.01) {
		t.Errorf("resumed spin jumped: %v degrees, was %v", r, held)
	}
	x.advanceTo(resumed + 250*time.Millisecond)
	if r := x.cs("a").cs.Transform.Rotate; !near32(r, held+90, 0.01) {
		t.Errorf("resumed spin: %v degrees, want %v", r, held+90)
	}
	if !x.tree().Animating() {
		t.Error("the resumed endless spin does not animate")
	}
}

func TestPausedFromTheStartAndArbitraryProperty(t *testing.T) {
	x := newInput(t, div(
		div(Key("p"), Class("size-8 animate-spin ease-linear paused")),
		div(Key("arb"), Class("size-8 animate-spin ease-linear [animation-play-state:paused]")),
		div(Key("run"), Class("size-8 animate-spin ease-linear [animation-play-state:paused] running")),
	), 200, 200)
	x.frame()
	t0 := x.cs("run").anim.kf.start
	x.advanceTo(t0 + 250*time.Millisecond)
	for _, k := range []string{"p", "arb"} {
		if r := x.cs(k).cs.Transform.Rotate; r != 0 {
			t.Errorf("%s: a paused spin from the start is at %v degrees", k, r)
		}
	}
	if r := x.cs("run").cs.Transform.Rotate; !near32(r, 90, 0.01) {
		t.Errorf("running after paused: %v degrees", r)
	}
}

// Chromium keeps an animation's current time when its timing changes, so a
// new duration changes the progress rather than rescaling it.
func TestAnimationDurationChangeKeepsTheCurrentTime(t *testing.T) {
	var set Setter[string]
	x := newInput(t, Component(func() Node {
		cls, s := UseState("animate-spin ease-linear")
		set = s
		return div(
			div(Key("a"), Class("size-8 "+cls)),
			div(Key("ref"), Class("size-8 animate-spin ease-linear duration-2000")),
		)
	}), 200, 200)
	x.frame()
	t0 := x.cs("a").anim.kf.start
	x.advanceTo(t0 + 250*time.Millisecond)
	if r := x.cs("a").cs.Transform.Rotate; !near32(r, 90, 0.01) {
		t.Fatalf("spin at 250 ms: %v degrees", r)
	}
	set.Set("animate-spin ease-linear duration-2000")
	x.frame()
	// The time it had run over the new 2 s, not the progress it had.
	want := float32((x.b.Now() - t0).Seconds() / 2 * 360)
	if r := x.cs("a").cs.Transform.Rotate; !near32(r, want, 0.01) || want > 60 {
		t.Errorf("after duration-2000: %v degrees, want %v (45 plus the frame of the change)", r, want)
	}
	x.advanceTo(t0 + 750*time.Millisecond)
	if a, ref := x.cs("a").cs.Transform.Rotate, x.cs("ref").cs.Transform.Rotate; !near32(a, ref, 0.01) {
		t.Errorf("at 750 ms: %v degrees, a spin that always lasted 2 s is at %v", a, ref)
	}
}

func TestFinishedAnimationRunsAgainWhenTheNewDurationOutlastsIt(t *testing.T) {
	var set Setter[string]
	x := newInput(t, Component(func() Node {
		cls, s := UseState("animate-pulse repeat-1 duration-500")
		set = s
		return div(div(Key("a"), Class("size-8 "+cls)))
	}), 200, 200)
	x.frame()
	t0 := x.cs("a").anim.kf.start
	x.advanceTo(t0 + 800*time.Millisecond)
	x.b.Drain(x.h)
	if x.tree().Animating() {
		t.Fatal("the 500 ms pulse is still animating at 800 ms")
	}
	set.Set("animate-pulse repeat-1 duration-2000")
	x.frame()
	x.advanceTo(t0 + 1000*time.Millisecond)
	if o := x.cs("a").cs.Opacity; !near32(o, 0.5, 1e-3) {
		t.Errorf("a pulse lengthened to 2 s, at 1 s: opacity %v, want 0.5", o)
	}
	if !x.tree().Animating() {
		t.Error("the lengthened animation is not running")
	}
}
