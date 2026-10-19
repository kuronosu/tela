package core

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// Tests of transitions and keyframe animations (anim.go) on the frame
// clock of a headless window: the clock moves only when the test says so.

// bg returns the background color of the box with key k, as the pipeline
// uses it (the animated value).
func (x *inputHarness) bg(k string) gfx.Color { return keyed(x.tree(), k).box.cs.BackgroundColor }

func (x *inputHarness) cs(k string) *Box { return keyed(x.tree(), k).box }

// runFor advances the clock frame by frame for d.
func (x *inputHarness) runFor(d time.Duration) {
	end := x.b.Now() + d
	for x.b.Now() < end {
		x.frame()
	}
}

// settleFinite delivers frames until only infinite animations (or nothing)
// ask for more.
func (x *inputHarness) settleFinite() {
	x.t.Helper()
	x.b.Drain(x.h)
	for i := 0; i < 400 && x.w.FrameRequested() && !x.tree().OnlyEndlessAnimations(); i++ {
		x.frame()
	}
	if x.w.FrameRequested() && !x.tree().OnlyEndlessAnimations() {
		x.t.Fatal("frames never stopped")
	}
}

// advanceTo moves the clock to the absolute time at (not before now) and
// delivers the frame due.
func (x *inputHarness) advanceTo(at time.Duration) {
	x.b.Advance(max(at-x.b.Now(), 0))
	x.b.Drain(x.h)
}

func near32(a, b, eps float32) bool { return math.Abs(float64(a-b)) <= float64(eps) }

const hoverBox = `h-[100px] w-[100px] bg-[#ffffff] hover:bg-[#000000] transition-colors duration-200 ease-linear`

func TestTransitionRunsOnHover(t *testing.T) {
	x := newInput(t, div(div(Key("b"), Class(hoverBox))), 200, 200)
	x.settle()
	if x.bg("b") != gfx.White {
		t.Fatalf("initial %v", x.bg("b"))
	}
	x.send(move(50, 50))
	x.frame() // the frame that restyles starts the transition
	start := x.cs("b").anim.trans[0].Start
	// Linear, sRGB: a quarter of the way is a quarter gray.
	x.advanceTo(start + 50*time.Millisecond)
	if c := x.bg("b"); !near32(c.R, 0.75, 1e-5) || c.R != c.G || c.A != 1 {
		t.Fatalf("after 50 ms: %v, want 0.75 gray", c)
	}
	x.advanceTo(start + 150*time.Millisecond)
	if c := x.bg("b"); !near32(c.R, 0.25, 1e-5) {
		t.Fatalf("after 150 ms: %v, want 0.25 gray", c)
	}
	x.settle()
	if x.bg("b") != gfx.Black {
		t.Fatalf("final %v", x.bg("b"))
	}
	if x.cs("b").anim != nil {
		t.Error("the box keeps animation state after its transition")
	}
	// And back: leaving the box transitions to white again.
	x.send(move(150, 150))
	x.runFor(100 * time.Millisecond)
	if c := x.bg("b"); c.R < 0.1 || c.R > 0.9 {
		t.Errorf("mid-way back: %v", c)
	}
	x.settle()
	if x.bg("b") != gfx.White {
		t.Fatalf("back %v", x.bg("b"))
	}
}

func TestNoTransitionOnMount(t *testing.T) {
	x := newInput(t, div(div(Key("b"), Class("bg-red-500 transition-colors duration-500"))), 200, 200)
	x.settle()
	if x.cs("b").anim != nil || x.tree().Animating() || x.w.FrameRequested() {
		t.Error("a box that mounts transitions")
	}
	// A state restyle at mount time (the root, and boxes mounted later)
	// does not transition either.
	var set Setter[bool]
	y := newInput(t, Component(func() Node {
		on, s := UseState(false)
		set = s
		var kid Node
		if on {
			kid = div(Key("late"), Class("h-4 bg-blue-500 transition-colors duration-500"))
		}
		return div(kid)
	}), 200, 200)
	set.Set(true)
	y.settle()
	if keyed(y.tree(), "late").box.anim != nil {
		t.Error("a box mounted later transitions")
	}
}

func TestIdleAfterTransition(t *testing.T) {
	x := newInput(t, div(div(Key("b"), Class(hoverBox))), 200, 200)
	x.settle()
	x.send(move(50, 50))
	x.runFor(500 * time.Millisecond)
	if x.bg("b") != gfx.Black {
		t.Fatalf("%v", x.bg("b"))
	}
	x.b.Drain(x.h)
	if x.w.FrameRequested() || x.tree().Animating() || x.tree().Dirty() {
		t.Fatalf("not idle: frame requested %v, animating %v, dirty %v", x.w.FrameRequested(), x.tree().Animating(), x.tree().Dirty())
	}
	// A second of clock with nothing to do runs no frame at all.
	frames, presents := x.tree().Stats().Frames, x.h.Presents()
	x.b.Advance(time.Second)
	x.b.Drain(x.h)
	if x.tree().Stats().Frames != frames || x.h.Presents() != presents || x.w.FrameRequested() {
		t.Errorf("idle app ran frames: %d -> %d", frames, x.tree().Stats().Frames)
	}
}

func TestTransitionReversalIsShorter(t *testing.T) {
	x := newInput(t, div(div(Key("b"), Class(hoverBox))), 200, 200)
	x.settle()
	x.send(move(50, 50))
	x.frame()
	first := x.cs("b").anim.trans[0]
	x.advanceTo(first.Start + 80*time.Millisecond) // 40 % of the way
	// Leave: the new transition starts from the current gray and goes back
	// to white in the time it had run (80 ms), not in 200 ms.
	x.send(move(150, 150))
	x.frame()
	tr := x.cs("b").anim.trans[0]
	// The restyle ran at the frame after the pointer left.
	ran := x.tree().anim.tk.Now() - first.Start
	want := float64(ran) / float64(200*time.Millisecond)
	if !near32(float32(tr.ReverseFactor), float32(want), 0.01) || want < 0.35 || want > 0.55 {
		t.Errorf("reversing factor %v, want %v", tr.ReverseFactor, want)
	}
	if d := time.Duration(want * float64(200*time.Millisecond)); tr.Duration < d-2*time.Millisecond || tr.Duration > d+2*time.Millisecond {
		t.Errorf("reversed duration %v, want %v", tr.Duration, d)
	}
	x.settle()
	if x.bg("b") != gfx.White {
		t.Errorf("end %v", x.bg("b"))
	}
}

func TestTransitionRetarget(t *testing.T) {
	// A box that transitions to a new target while one runs starts from the
	// current value.
	var set Setter[int]
	classes := []string{"bg-[#ff0000]", "bg-[#00ff00]", "bg-[#0000ff]"}
	x := newInput(t, Component(func() Node {
		i, s := UseState(0)
		set = s
		return div(div(Key("b"), Class("size-10 transition-colors duration-200 ease-linear "+classes[i])))
	}), 100, 100)
	x.settle()
	set.Set(1)
	x.b.Drain(x.h)
	x.frame()
	start := x.cs("b").anim.trans[0].Start
	x.advanceTo(start + 100*time.Millisecond)
	mid := x.bg("b")
	if !near32(mid.R, 0.5, 1e-4) || !near32(mid.G, 0.5, 1e-4) {
		t.Fatalf("half way red to green: %v", mid)
	}
	set.Set(2)
	x.b.Drain(x.h)
	x.frame()
	if c := x.bg("b"); !near32(c.R, mid.R, 0.1) || !near32(c.G, mid.G, 0.1) {
		t.Errorf("retarget jumped: %v, was %v", c, mid)
	}
	x.settle()
	if c := x.bg("b"); c != (gfx.Color{B: 1, A: 1}) {
		t.Errorf("final %v", c)
	}
}

func TestTransitionDelay(t *testing.T) {
	x := newInput(t, div(div(Key("b"), Class("size-10 bg-[#ffffff] hover:bg-[#000000] transition-colors duration-100 delay-300 ease-linear"))), 100, 100)
	x.settle()
	x.send(move(5, 5))
	x.frame()
	start := x.cs("b").anim.trans[0].Start
	x.advanceTo(start - 100*time.Millisecond)
	if x.bg("b") != gfx.White {
		t.Errorf("before the delay ends: %v", x.bg("b"))
	}
	x.advanceTo(start + 50*time.Millisecond)
	if c := x.bg("b"); !near32(c.R, 0.5, 1e-4) {
		t.Errorf("half way after the delay: %v", c)
	}
	x.settle()
	if x.bg("b") != gfx.Black {
		t.Errorf("final %v", x.bg("b"))
	}
}

func TestReducedMotionHasNoTransitions(t *testing.T) {
	b := headless.New(headless.Options{Size: platform.Size{W: 200, H: 200}, Settings: platform.SystemSettings{ReducedMotion: true}})
	host, err := NewHost(b, Options{Config: Config{Fonts: testFonts}, Root: div(div(Key("b"), Class(hoverBox)))})
	if err != nil {
		t.Fatal(err)
	}
	b.Drain(host)
	x := &inputHarness{t: t, b: b, h: host, w: host.Window().(*headless.Window)}
	t.Cleanup(host.stopTimer)
	x.settle()
	x.send(move(50, 50))
	x.frame()
	if x.bg("b") != gfx.Black || x.tree().Animating() {
		t.Errorf("with reduced motion the hover is instant: %v, animating %v", x.bg("b"), x.tree().Animating())
	}
	x.settle()

	// Turning reduced motion on while a transition runs ends it.
	y := newInput(t, div(div(Key("b"), Class(hoverBox))), 200, 200)
	y.settle()
	y.send(move(50, 50))
	y.runFor(50 * time.Millisecond)
	if c := y.bg("b"); c.R <= 0.05 || c.R >= 0.95 {
		t.Fatalf("not mid-flight: %v", c)
	}
	y.b.SetSettings(platform.SystemSettings{ReducedMotion: true})
	y.b.Drain(y.h)
	y.frame()
	if y.bg("b") != gfx.Black {
		t.Errorf("reduced motion did not end the transition: %v", y.bg("b"))
	}
}

func TestMotionReduceVariantWithAnimations(t *testing.T) {
	spinner := func() Node { return div(div(Key("s"), Class("size-8 animate-spin motion-reduce:animate-none"))) }
	x := newInput(t, spinner(), 100, 100)
	x.frame()
	x.frame()
	if !x.tree().Animating() {
		t.Fatal("animate-spin does not animate")
	}
	b := headless.New(headless.Options{Size: platform.Size{W: 100, H: 100}, Settings: platform.SystemSettings{ReducedMotion: true}})
	host, err := NewHost(b, Options{Config: Config{Fonts: testFonts}, Root: spinner()})
	if err != nil {
		t.Fatal(err)
	}
	b.Drain(host)
	t.Cleanup(host.stopTimer)
	for range 3 {
		b.Advance(b.FrameInterval())
		b.Drain(host)
	}
	if host.Tree().Animating() || host.Window().(*headless.Window).FrameRequested() {
		t.Error("motion-reduce:animate-none still animates")
	}
}

func TestKeyframeAnimationsOnTheClock(t *testing.T) {
	x := newInput(t, div(
		div(Key("spin"), Class("size-8 animate-spin")),
		div(Key("pulse"), Class("size-8 animate-pulse")),
		div(Key("ping"), Class("size-8 animate-ping")),
		div(Key("bounce"), Class("size-8 animate-bounce")),
	), 200, 200)
	x.frame()
	start := x.b.Now()
	_ = start
	t0 := x.cs("spin").anim.kf.start
	x.advanceTo(t0 + 250*time.Millisecond)
	if r := x.cs("spin").cs.Transform.Rotate; !near32(r, 90, 0.01) {
		t.Errorf("spin at 250 ms: %v degrees", r)
	}
	// Pulse is at 0.5 opacity at 1 s (half of its 2 s).
	x.advanceTo(t0 + time.Second)
	if o := x.cs("pulse").cs.Opacity; !near32(o, 0.5, 1e-3) {
		t.Errorf("pulse at 1 s: opacity %v", o)
	}
	x.advanceTo(t0 + 1250*time.Millisecond)
	if r := x.cs("spin").cs.Transform.Rotate; !near32(r, 90, 0.01) {
		t.Errorf("spin repeats: %v degrees at 1.25 s", r)
	}
	// Ping ends scaled by 2 and transparent after 75 % of its 1 s.
	x.advanceTo(t0 + 2*time.Second + 900*time.Millisecond)
	if o := x.cs("ping").cs.Opacity; o != 0 {
		t.Errorf("ping at 0.9 of its cycle: opacity %v", o)
	}
	// Bounce rises to translateY(-25%) and is back at 0 at the middle.
	x.advanceTo(t0 + 3*time.Second + 500*time.Millisecond)
	if y := x.cs("bounce").cs.Transform.TranslateY; y.Pct != 0 {
		t.Errorf("bounce at 0.5: %v", y)
	}
	// Infinite animations are frames for the app, not work a test waits for.
	x.b.Drain(x.h)
	if !x.w.FrameRequested() || !x.tree().OnlyEndlessAnimations() {
		t.Errorf("frame requested %v, only endless %v", x.w.FrameRequested(), x.tree().OnlyEndlessAnimations())
	}
	if x.tree().Dirty() {
		t.Error("infinite animations leave the tree dirty after a frame")
	}
}

func TestInfiniteAnimationStopsWhenUnmounted(t *testing.T) {
	var set Setter[bool]
	x := newInput(t, Component(func() Node {
		on, s := UseState(true)
		set = s
		if !on {
			return div()
		}
		return div(div(Key("s"), Class("size-8 animate-spin")))
	}), 100, 100)
	x.runFor(100 * time.Millisecond)
	if !x.tree().Animating() {
		t.Fatal("not animating")
	}
	set.Set(false)
	x.b.Drain(x.h)
	x.runFor(100 * time.Millisecond)
	x.settle()
	if x.tree().Animating() || x.w.FrameRequested() {
		t.Error("the spinner keeps asking for frames after it unmounted")
	}
}

func TestHiddenAnimationsAreDormant(t *testing.T) {
	var set Setter[bool]
	x := newInput(t, Component(func() Node {
		show, s := UseState(false)
		set = s
		cls := "hidden"
		if show {
			cls = "block"
		}
		return div(div(Key("wrap"), Class(cls), div(Key("s"), Class("size-8 animate-spin"))))
	}), 100, 100)
	for range 5 {
		x.frame()
	}
	x.settle()
	if x.tree().Animating() || x.w.FrameRequested() {
		t.Fatalf("a spinner in a hidden box asks for frames (animating %v)", x.tree().Animating())
	}
	// Shown, it starts from the beginning.
	set.Set(true)
	x.b.Drain(x.h)
	for range 3 {
		x.frame()
	}
	if !x.tree().Animating() {
		t.Fatal("the spinner does not start when its box is shown")
	}
	shown := x.cs("s").anim.kf.start
	if shown < x.b.Now()-5*x.b.FrameInterval() {
		t.Errorf("restarted at %v, long before now (%v)", shown, x.b.Now())
	}
	// Hidden again: dormant.
	set.Set(false)
	x.b.Drain(x.h)
	x.runFor(100 * time.Millisecond)
	x.settle()
	if x.tree().Animating() {
		t.Error("hiding the box did not park the animation")
	}
}

func TestAnimationLayersAndPaintOnlyFrames(t *testing.T) {
	x := newInput(t, div(
		div(Key("spin"), Class("size-8 bg-red-500 animate-spin")),
		div(Key("fade"), Class("size-8 bg-blue-500 opacity-100 hover:opacity-50 transition-opacity duration-300 ease-linear")),
	), 200, 200)
	x.frame()
	spin := x.cs("spin")
	if !spin.deferPaint || !spin.layerSC || spin.pcs == nil {
		t.Fatalf("the spinning box is not a layer: deferPaint %v layerSC %v", spin.deferPaint, spin.layerSC)
	}
	layers := 0
	for _, l := range x.tree().lay.layers {
		if l.kind == layerDeferred && l.box == spin {
			layers++
		}
	}
	if layers != 1 {
		t.Errorf("%d deferred layers for the spinner", layers)
	}
	// While it spins, frames only paint: no build, restyle or layout.
	before := x.tree().Stats()
	x.runFor(200 * time.Millisecond)
	after := x.tree().Stats()
	if after.Frames <= before.Frames {
		t.Fatal("no frames ran")
	}
	if after.Builds != before.Builds || after.Styles != before.Styles || after.Layouts != before.Layouts || after.Structures != before.Structures {
		t.Errorf("a spinning box costs more than paint: %+v -> %+v", before, after)
	}
	// Only the spinner's area is rasterized.
	d := x.tree().LastDamage()
	area := 0
	for _, r := range d.Rects {
		area += r.Dx() * r.Dy()
	}
	if d.Full || area == 0 || area > 64*64 {
		t.Errorf("damage of a spinner frame: %+v (%d px)", d, area)
	}
	// An opacity transition is the same.
	x.send(move(5, 45))
	x.frame()
	before = x.tree().Stats()
	x.runFor(100 * time.Millisecond)
	after = x.tree().Stats()
	if o := x.cs("fade").cs.Opacity; o >= 1 || o <= 0.5 {
		t.Errorf("opacity %v mid-transition", o)
	}
	if after.Builds != before.Builds || after.Styles != before.Styles || after.Layouts != before.Layouts {
		t.Errorf("an opacity transition costs more than paint: %+v -> %+v", before, after)
	}
	x.settleFinite()
	fade := x.cs("fade")
	if fade.anim != nil || fade.deferPaint {
		t.Error("the fade box keeps animation state after its transition")
	}
	for _, l := range x.tree().lay.layers {
		if l.kind == layerDeferred && l.box == fade {
			t.Error("the fade box is still a layer after its transition")
		}
	}
}

func TestSizeTransitionsRelayout(t *testing.T) {
	x := newInput(t, div(div(Key("b"), Class("h-10 w-10 bg-red-500 hover:w-40 transition-[width] duration-200 ease-linear"))), 200, 100)
	x.settle()
	x.send(move(5, 5))
	x.frame()
	start := x.cs("b").anim.trans[0].Start
	before := x.tree().Stats()
	x.advanceTo(start + 100*time.Millisecond)
	if w := x.cs("b").ld.Layout().Size.W; !near32(w, 100, 0.01) {
		t.Errorf("the width at the middle: %v, want 100", w)
	}
	if x.tree().Stats().Layouts == before.Layouts {
		t.Error("a width transition does not lay out")
	}
	x.settle()
	if w := x.cs("b").ld.Layout().Size.W; w != 160 {
		t.Errorf("final width %v", w)
	}
}

// TestInheritedColorTransition: a text color transition reaches the
// children, which follow it without transitions of their own.
func TestInheritedColorTransition(t *testing.T) {
	x := newInput(t, div(
		div(Key("b"), Class("size-20 text-[#ffffff] hover:text-[#000000] transition-colors duration-200 ease-linear"),
			span(Key("kid"), Class("transition-colors duration-500"), Text("hello"))),
	), 200, 100)
	x.settle()
	x.send(move(5, 5))
	x.frame()
	start := x.cs("b").anim.trans[0].Start
	x.advanceTo(start + 100*time.Millisecond)
	parent, kid := x.cs("b").cs.Text.Color, x.cs("kid").cs.Text.Color
	if !near32(parent.R, 0.5, 1e-3) || kid != parent {
		t.Errorf("parent %v, child %v: the child must follow at 0.5", parent, kid)
	}
	if x.cs("kid").anim != nil {
		t.Error("the child started a transition for an inherited value")
	}
	x.settle()
	if kid := x.cs("kid").cs.Text.Color; kid != gfx.Black {
		t.Errorf("child end color %v", kid)
	}
}

func TestUnknownAnimationIsReported(t *testing.T) {
	var logs []string
	tr := newTestTree(t, 100, 100, div(div(Key("x"), Class("animate-[wiggle_1s_ease_infinite]"))), &logs)
	tr.Frame()
	found := false
	for _, l := range logs {
		if len(l) > 0 && contains(l, "unknown keyframes") && contains(l, "wiggle") {
			found = true
		}
	}
	if !found {
		t.Errorf("no message about the unknown keyframes in %q", logs)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// TestAnimationsWithoutAClock: a tree whose frames carry no clock (Render)
// has no transitions, and keyframes show their first state.
func TestAnimationsWithoutAClock(t *testing.T) {
	var set Setter[bool]
	tr := newTestTree(t, 100, 100, Component(func() Node {
		on, s := UseState(false)
		set = s
		c := "bg-red-500"
		if on {
			c = "bg-blue-500"
		}
		return div(
			div(Key("a"), Class("size-8 bg-red-500 animate-spin")),
			div(Key("b"), Class("size-8 transition-colors duration-500 "+c)))
	}), nil)
	tr.Frame()
	if tr.Animating() {
		t.Error("animating without a clock")
	}
	set.Set(true)
	tr.Frame()
	b := keyed(tr, "b").box
	if b.anim != nil || b.cs.BackgroundColor == (gfx.Color{}) || tr.Animating() {
		t.Error("a transition started in a tree that has no clock")
	}
	if a := keyed(tr, "a").box; a.anim == nil || a.cs.Transform.Rotate != 0 {
		t.Errorf("keyframes without a clock: %+v", a.cs.Transform)
	}
}

func TestAnimationsDamageMatchesFullRender(t *testing.T) {
	var set Setter[bool]
	root := Component(func() Node {
		on, s := UseState(false)
		set = s
		w := "w-16"
		if on {
			w = "w-32"
		}
		return div(Class("flex flex-wrap gap-3 p-3"),
			div(Key("spin"), Class("size-12 rounded-full border-4 border-gray-300 border-t-blue-600 animate-spin")),
			div(Key("ping"), Class("size-12 rounded-full bg-sky-400 animate-ping")),
			div(Key("pulse"), Class("size-12 rounded-xl bg-slate-400 animate-pulse")),
			div(Key("bounce"), Class("size-12 rounded-full bg-amber-400 animate-bounce")),
			div(Key("hover"), Class("size-16 rounded-xl bg-white shadow-sm hover:shadow-xl hover:bg-indigo-100 hover:-translate-y-1 transition duration-300 ease-in-out")),
			div(Key("size"), Class("h-12 rounded-md bg-violet-500 transition-[width] duration-300 "+w)),
			div(Key("blur"), Class("size-12 rounded-xl bg-rose-500 hover:blur-md transition-[filter] duration-300")),
		)
	})
	x := newDamageHarness(t, root, 360, 200, 2, Config{}, 9)
	hov := func(px, py float32) {
		x.inject(platform.PointerEvent{Action: platform.PointerMove, Pos: platform.Point{X: px, Y: py}})
	}
	x.frame("start")
	for i := range 12 {
		x.frame(fmt.Sprintf("spinning %d", i))
	}
	hov(284, 44) // over the hover box
	x.frame("hover starts")
	if ba := keyed(x.tree(), "hover").box.anim; ba == nil || len(ba.trans) < 3 {
		t.Fatalf("the hover box did not start its transitions: %+v", ba)
	}
	for i := range 40 {
		x.frame(fmt.Sprintf("hover %d", i))
	}
	set.Set(true)
	x.b.Drain(x.h)
	for i := range 30 {
		x.frame(fmt.Sprintf("width %d", i))
	}
	hov(112, 112) // over the blur box, leaving the hover box
	for i := range 30 {
		x.frame(fmt.Sprintf("blur %d", i))
	}
	hov(300, 190)
	for i := range 30 {
		x.frame(fmt.Sprintf("away %d", i))
	}
	if x.partial == 0 {
		t.Error("no frame was rasterized partially: animations repaint the whole window")
	}
	t.Logf("frames %d: full %d, partial %d, shifted %d, skipped %d", x.frames, x.full, x.partial, x.shifted, x.skipped)
}

// TestOwnColorTransitionsUnderAnInheritedChange: a child restyled because
// its parent's text color changed still transitions the values it sets
// itself in the same restyle (CSS Transitions: its own computed value
// changed); only the values it inherits follow the parent.
func TestOwnColorTransitionsUnderAnInheritedChange(t *testing.T) {
	const kid = `group-hover:text-[#0000ff] group-hover:border-[#ff0000] border-2 transition-colors duration-200 ease-linear`
	x := newInput(t, div(
		div(Key("g"), Class("group size-20 text-[#000000] hover:text-[#ffffff]"),
			div(Key("own"), Class("h-4 "+kid), Text("own")),
			div(Key("inh"), Class("h-4 border-2 transition-colors duration-200 ease-linear"), Text("inherits")),
			div(Key("bd"), Class("h-4 border-2 group-hover:border-[#ff0000] transition-colors duration-200 ease-linear"), Text("border")),
		),
	), 200, 100)
	x.settle()
	x.send(move(5, 5))
	x.frame()
	own := x.cs("own")
	if own.anim == nil || len(own.anim.trans) == 0 {
		t.Fatal("the child's own color snapped: no transition started")
	}
	start := own.anim.trans[0].Start
	x.advanceTo(start + 100*time.Millisecond)
	// Black to blue and black (currentColor) to red, linear in sRGB.
	if c := x.cs("own").cs.Text.Color; !near32(c.B, 0.5, 1e-3) || c.R != 0 || c.G != 0 {
		t.Errorf("own text color at the middle: %v, want half blue", c)
	}
	if c := x.cs("own").cs.Border.Top.Color; !near32(c.R, 0.5, 1e-3) || c.G != 0 || c.B != 0 {
		t.Errorf("own border color at the middle: %v, want half red", c)
	}
	// The inherited text color and the currentColor border that follows it
	// take the parent's value at once.
	if c := x.cs("inh").cs.Text.Color; c != gfx.White {
		t.Errorf("inherited text color %v, want the parent's white", c)
	}
	if c := x.cs("inh").cs.Border.Top.Color; c != gfx.White {
		t.Errorf("currentColor border of an inheriting child %v, want white", c)
	}
	if a := x.cs("inh").anim; a != nil && len(a.trans) > 0 {
		t.Errorf("the inheriting child started %d transitions", len(a.trans))
	}
	// A child that inherits its text color but sets its border color
	// transitions the border only.
	if c := x.cs("bd").cs.Text.Color; c != gfx.White {
		t.Errorf("inherited text color %v, want white", c)
	}
	if c := x.cs("bd").cs.Border.Top.Color; !near32(c.R, 0.5, 1e-3) || c.G != 0 || c.B != 0 {
		t.Errorf("own border color under an inherited text color: %v, want half red", c)
	}
	x.settle()
	if c := x.cs("own").cs.Text.Color; c != (gfx.Color{B: 1, A: 1}) {
		t.Errorf("own end color %v", c)
	}

	// The same for the font size: an own size transitions, an inherited
	// or em size follows.
	y := newInput(t, div(
		div(Key("g"), Class("group size-20 text-[10px] hover:text-[30px]"),
			div(Key("own"), Class("text-[14px] group-hover:text-[18px] transition-[font-size] duration-200 ease-linear"), Text("own")),
			div(Key("em"), Class("text-[1em] transition-[font-size] duration-200 ease-linear"), Text("em")),
		),
	), 200, 100)
	y.settle()
	y.send(move(5, 5))
	y.frame()
	if a := y.cs("own").anim; a == nil || len(a.trans) == 0 {
		t.Fatal("the child's own font size snapped")
	}
	start = y.cs("own").anim.trans[0].Start
	y.advanceTo(start + 100*time.Millisecond)
	if s := y.cs("own").cs.Text.FontSize; !near32(s, 16, 1e-3) {
		t.Errorf("own font size at the middle: %v, want 16", s)
	}
	if s := y.cs("em").cs.Text.FontSize; s != 30 {
		t.Errorf("em font size %v, want the parent's 30 at once", s)
	}
}
