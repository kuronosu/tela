package core

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/style"
)

// work is the pipeline work of a frame: the difference of two Stats.
func work(s0, s1 Stats) Stats {
	return Stats{
		Frames: s1.Frames - s0.Frames, Builds: s1.Builds - s0.Builds, Skips: s1.Skips - s0.Skips,
		Mounts: s1.Mounts - s0.Mounts, Unmounts: s1.Unmounts - s0.Unmounts,
		Styles: s1.Styles - s0.Styles, Structures: s1.Structures - s0.Structures,
		Shapes: s1.Shapes - s0.Shapes, Breaks: s1.Breaks - s0.Breaks,
		Layouts: s1.Layouts - s0.Layouts, Paints: s1.Paints - s0.Paints,
	}
}

// paintOnly is the work of a frame that only repaints.
var paintOnly = Stats{Frames: 1, Paints: 1}

// transforms returns the PushTransform matrices of a display list.
func transforms(dl *gfx.DisplayList) []gfx.Affine {
	var out []gfx.Affine
	for _, c := range dl.All() {
		if t, ok := c.(gfx.PushTransform); ok {
			out = append(out, t.Affine)
		}
	}
	return out
}

// layerOpacities returns the opacities of the PushLayer commands of a
// display list.
func layerOpacities(dl *gfx.DisplayList) []float32 {
	var out []float32
	for _, c := range dl.All() {
		if l, ok := c.(gfx.PushLayer); ok {
			out = append(out, l.Opacity)
		}
	}
	return out
}

// frameCounter counts the frames a tree requests.
func frameCounter(tr *Tree) *int {
	n := new(int)
	tr.SetRequestFrame(func() { *n++ })
	return n
}

func TestValueReadInBuildRebuilds(t *testing.T) {
	var v *Value[int]
	builds := 0
	comp := func() Node {
		builds++
		v = UseValue(1)
		return div(Textf("v=%d", v.Get()))
	}
	tr := newTestTree(t, 200, 100, Component(comp), nil)
	tr.Frame()
	first := v
	requests := frameCounter(tr)
	s0 := tr.Stats()
	v.Set(2)
	if v.Get() != 1 {
		t.Errorf("Get before the update applies = %d, want 1 (the applied value)", v.Get())
	}
	if !tr.ApplyUpdates() || *requests != 1 {
		t.Errorf("a Value read in a build did not request a frame (%d requests)", *requests)
	}
	tr.Frame()
	if d := work(s0, tr.Stats()); d.Builds != 1 || builds != 2 {
		t.Errorf("work %+v, %d builds; want one rebuild", d, builds)
	}
	if got := texts(tr); got != "v=2" {
		t.Errorf("text %q, want v=2", got)
	}
	if v != first {
		t.Error("UseValue returned another *Value on the second build")
	}
	if n := len(v.subs); n != 1 {
		t.Errorf("%d subscriptions, want 1 (rebuilds keep it)", n)
	}
}

func TestValueReadInLayoutRelayoutsOnly(t *testing.T) {
	var w *Value[float32]
	builds := 0
	comp := func() Node {
		builds++
		w = UseValue[float32](40)
		return div(Class("flex"),
			div(Key("bar"), Class("h-4 bg-sky-500"), Style(style.WidthFn(func() float32 { return w.Get() }))),
			div(Key("next"), Class("size-4 bg-pink-500")),
		)
	}
	tr := newTestTree(t, 300, 100, Component(comp), nil)
	tr.Frame()
	if got := mustFind(t, tr, "bar").Rect.Width(); got != 40 {
		t.Fatalf("bar width %g, want 40", got)
	}
	s0 := tr.Stats()
	w.Set(100)
	tr.Frame()
	d := work(s0, tr.Stats())
	if d != (Stats{Frames: 1, Layouts: 1, Paints: 1}) {
		t.Errorf("work %+v, want one layout and one paint (no build, style or structure)", d)
	}
	if builds != 1 {
		t.Errorf("%d builds, want 1", builds)
	}
	bar, next := mustFind(t, tr, "bar"), mustFind(t, tr, "next")
	if bar.Rect.Width() != 100 || next.Rect.X0 != 100 {
		t.Errorf("bar width %g, next x %g; want 100 and 100", bar.Rect.Width(), next.Rect.X0)
	}
	// An equal value does nothing.
	s0 = tr.Stats()
	w.Set(100)
	if tr.ApplyUpdates() || tr.Dirty() {
		t.Error("an equal value invalidated the layout")
	}
	if _, changed := tr.Frame(); changed || work(s0, tr.Stats()) != (Stats{}) {
		t.Errorf("an equal value ran a frame: %+v", work(s0, tr.Stats()))
	}
}

func TestValueReadInLayoutKeepsSpaceMargins(t *testing.T) {
	// A box mapped again alone (Tree.remapOne) keeps the space-y margin
	// its parent gives it.
	var h *Value[float32]
	comp := func() Node {
		h = UseValue[float32](10)
		return div(Class("space-y-4"),
			div(Key("a"), Class("h-5")),
			div(Key("b"), Style(style.HeightFn(func() float32 { return h.Get() }))),
			div(Key("c"), Class("h-5")),
		)
	}
	tr := newTestTree(t, 200, 200, Component(comp), nil)
	tr.Frame()
	h.Set(30)
	tr.Frame()
	got := mustFind(t, tr, "c").Rect.Y0
	// Fresh tree with the same height.
	ref := newTestTree(t, 200, 200, div(Class("space-y-4"),
		div(Key("a"), Class("h-5")), div(Key("b"), Class("h-[30px]")), div(Key("c"), Class("h-5"))), nil)
	ref.Frame()
	if want := mustFind(t, ref, "c").Rect.Y0; got != want {
		t.Errorf("c at y %g after the change, want %g", got, want)
	}
}

func TestValueReadInPaintRepaintsOnly(t *testing.T) {
	var x, op *Value[float32]
	builds := 0
	comp := func() Node {
		builds++
		x = UseValue[float32](0)
		op = UseValue[float32](1)
		return div(Class("p-4"),
			div(Key("card"), Class("size-10 bg-sky-500"), Style(
				style.TranslateFn(func() (float32, float32) { return x.Get(), 0 }),
				style.OpacityFn(func() float32 { return op.Get() }),
			)),
			p(Class("w-40"), Text("a paragraph that is broken into a few lines of text")),
		)
	}
	tr := newTestTree(t, 300, 200, Component(comp), nil)
	dl, _ := tr.Frame()
	if len(transforms(dl)) != 0 || len(layerOpacities(dl)) != 0 {
		t.Fatalf("identity translate and opacity 1 painted a transform or a layer:\n%s", dl)
	}
	card := mustFind(t, tr, "card").Rect
	requests := frameCounter(tr)
	s0 := tr.Stats()
	x.Set(12)
	if !tr.ApplyUpdates() || !tr.Dirty() || *requests != 1 {
		t.Fatalf("a Value read in paint did not request a frame (%d requests)", *requests)
	}
	dl, changed := tr.Frame()
	if d := work(s0, tr.Stats()); d != paintOnly || !changed {
		t.Errorf("work %+v (changed %v), want a paint only", d, changed)
	}
	if ts := transforms(dl); len(ts) != 1 || ts[0][4] != 12 || ts[0][5] != 0 {
		t.Errorf("transforms %v, want one translation by (12, 0)", ts)
	}
	if got := mustFind(t, tr, "card").Rect; got != card {
		t.Errorf("the card moved in layout: %v, was %v", got, card)
	}
	s0 = tr.Stats()
	op.Set(0.5)
	x.Update(func(v float32) float32 { return v + 1 })
	dl, _ = tr.Frame()
	if d := work(s0, tr.Stats()); d != paintOnly {
		t.Errorf("work %+v, want a paint only", d)
	}
	if ops := layerOpacities(dl); !slices.Equal(ops, []float32{0.5}) {
		t.Errorf("layer opacities %v, want [0.5]", ops)
	}
	if ts := transforms(dl); len(ts) != 1 || ts[0][4] != 13 {
		t.Errorf("transforms %v, want a translation by 13", ts)
	}
	if builds != 1 {
		t.Errorf("%d builds, want 1", builds)
	}
	// Idle afterwards: no frame.
	if _, changed := tr.Frame(); changed {
		t.Error("a frame after the repaint had work")
	}
}

func TestValueReadElsewhereSubscribesNothing(t *testing.T) {
	var v *Value[int]
	var seen []int
	comp := func() Node {
		v = UseValue(0)
		UseEffect(func() func() { seen = append(seen, v.Get()); return nil })
		UseLayoutEffect(func() func() { seen = append(seen, v.Get()); return nil })
		return div(Text("static"))
	}
	tr := newTestTree(t, 200, 100, Component(comp), nil)
	tr.Frame()
	tr.FlushEffects()
	_ = v.Get() // an event handler on the UI thread
	got := make(chan int)
	go func() { got <- v.Get() }()
	<-got
	if n := len(v.subs); n != 0 {
		t.Fatalf("%d subscriptions after reads in effects, a handler and a goroutine; want 0", n)
	}
	requests := frameCounter(tr)
	s0 := tr.Stats()
	v.Set(5)
	if tr.ApplyUpdates() || tr.Dirty() || *requests != 0 {
		t.Errorf("a Value nobody tracked requested work (%d requests)", *requests)
	}
	if _, changed := tr.Frame(); changed || work(s0, tr.Stats()) != (Stats{}) {
		t.Errorf("work %+v, want none", work(s0, tr.Stats()))
	}
	if v.Get() != 5 {
		t.Errorf("Get = %d after the update, want 5", v.Get())
	}
	go func() { got <- v.Get() }()
	if g := <-got; g != 5 {
		t.Errorf("Get from a goroutine = %d, want 5", g)
	}
	if !slices.Equal(seen, []int{0, 0}) {
		t.Errorf("effects saw %v, want [0 0]", seen)
	}
}

func TestValueNonComparableAlwaysChanges(t *testing.T) {
	var v *Value[[]int]
	builds := 0
	comp := func() Node {
		builds++
		v = UseValue([]int{1})
		return Textf("%v", v.Get())
	}
	tr := newTestTree(t, 200, 100, Component(comp), nil)
	tr.Frame()
	v.Set([]int{1})
	tr.Frame()
	if builds != 2 {
		t.Errorf("%d builds, want 2 (slices cannot be compared)", builds)
	}
}

func TestValueUpdatesInQueueOrder(t *testing.T) {
	var v *Value[int]
	var set Setter[int]
	var built []string
	comp := func() Node {
		n, s := UseState(0)
		set = s
		v = UseValue(0)
		built = append(built, fmt.Sprintf("%d %d", n, v.Get()))
		return div()
	}
	tr := newTestTree(t, 200, 100, Component(comp), nil)
	tr.Frame()
	var seen []int
	look := func() { tr.enqueue(update{post: func() { seen = append(seen, v.Get()) }}) }
	set.Set(1)
	v.Update(func(x int) int { return x + 10 })
	look()
	v.Set(3)
	set.Update(func(n int) int { return n + 1 })
	v.Update(func(x int) int { return x * 2 })
	look()
	tr.Frame()
	if !slices.Equal(seen, []int{10, 6}) {
		t.Errorf("values seen in queue order %v, want [10 6]", seen)
	}
	if want := []string{"0 0", "2 6"}; !slices.Equal(built, want) {
		t.Errorf("builds %q, want %q", built, want)
	}
}

// TestValueFromGoroutines updates and reads a Value from many goroutines
// while the UI thread applies the updates and paints (the race detector
// needs cgo, so this checks the values instead).
func TestValueFromGoroutines(t *testing.T) {
	var v *Value[int]
	painted := 0
	comp := func() Node {
		v = UseValue(0)
		return div(Class("size-4"), Style(style.OpacityFn(func() float32 { painted = v.Get(); return 1 })))
	}
	tr := newTestTree(t, 200, 100, Component(comp), nil)
	tr.Frame()
	wakes := make(chan struct{}, 1)
	tr.SetWake(func() {
		select {
		case wakes <- struct{}{}:
		default:
		}
	})
	s0 := tr.Stats()
	const workers, per = 8, 200
	vv := v
	var wg sync.WaitGroup
	errs := make(chan string, workers)
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			last := 0
			for i := 0; i < per; i++ {
				vv.Update(func(x int) int { return x + 1 })
				// Applied values only grow: a goroutine never sees one
				// go back.
				if g := vv.Get(); g < last {
					errs <- fmt.Sprintf("Get went back from %d to %d", last, g)
					return
				} else {
					last = g
				}
			}
		}()
	}
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	for running := true; running; {
		select {
		case <-wakes:
			tr.Frame()
		case <-done:
			running = false
		}
	}
	tr.Frame()
	close(errs)
	for e := range errs {
		t.Error(e)
	}
	if v.Get() != workers*per || painted != workers*per {
		t.Errorf("value %d, painted %d; want %d", v.Get(), painted, workers*per)
	}
	if d := work(s0, tr.Stats()); d.Builds != 0 || d.Styles != 0 || d.Layouts != 0 || d.Paints == 0 {
		t.Errorf("work %+v, want paints only", d)
	}
}

func TestValueSubscriptionsFollowReads(t *testing.T) {
	var a, b *Value[float32]
	var useA *Value[bool]
	comp := func() Node {
		a = UseValue[float32](1)
		b = UseValue[float32](2)
		useA = UseValue(true)
		return div(Class("size-4 bg-sky-500"), Style(style.TranslateFn(func() (float32, float32) {
			if useA.Get() {
				return a.Get() + a.Get(), 0 // two reads, one subscription
			}
			return b.Get(), 0
		})))
	}
	tr := newTestTree(t, 200, 100, Component(comp), nil)
	tr.Frame()
	if len(a.subs) != 1 || len(b.subs) != 0 || len(useA.subs) != 1 {
		t.Fatalf("subscriptions a %d, b %d, useA %d; want 1, 0, 1", len(a.subs), len(b.subs), len(useA.subs))
	}
	useA.Set(false)
	tr.Frame()
	if len(a.subs) != 0 || len(b.subs) != 1 || len(useA.subs) != 1 {
		t.Fatalf("after switching: subscriptions a %d, b %d, useA %d; want 0, 1, 1", len(a.subs), len(b.subs), len(useA.subs))
	}
	s0 := tr.Stats()
	a.Set(5)
	if tr.ApplyUpdates() {
		t.Error("a Value the closure stopped reading invalidated it")
	}
	if _, changed := tr.Frame(); changed {
		t.Error("a Value the closure stopped reading ran a frame")
	}
	b.Set(7)
	dl, _ := tr.Frame()
	if d := work(s0, tr.Stats()); d != paintOnly {
		t.Errorf("work %+v, want a paint only", d)
	}
	if ts := transforms(dl); len(ts) != 1 || ts[0][4] != 7 {
		t.Errorf("transforms %v, want a translation by 7", ts)
	}

	// A layout closure that switches Values.
	var wa, wb *Value[float32]
	var useWA *Value[bool]
	sized := func() Node {
		wa = UseValue[float32](10)
		wb = UseValue[float32](20)
		useWA = UseValue(true)
		return div(Key("sized"), Class("h-4"), Style(style.WidthFn(func() float32 {
			if useWA.Get() {
				return wa.Get()
			}
			return wb.Get()
		})))
	}
	tr = newTestTree(t, 200, 100, Component(sized), nil)
	tr.Frame()
	useWA.Set(false)
	tr.Frame()
	if len(wa.subs) != 0 || len(wb.subs) != 1 || mustFind(t, tr, "sized").Rect.Width() != 20 {
		t.Fatalf("layout switch: subscriptions a %d, b %d, width %g; want 0, 1, 20",
			len(wa.subs), len(wb.subs), mustFind(t, tr, "sized").Rect.Width())
	}
	wa.Set(50)
	if tr.ApplyUpdates() {
		t.Error("a Value the layout closure stopped reading invalidated it")
	}
	s0 = tr.Stats()
	wb.Set(30)
	tr.Frame()
	if d := work(s0, tr.Stats()); d != (Stats{Frames: 1, Layouts: 1, Paints: 1}) || mustFind(t, tr, "sized").Rect.Width() != 30 {
		t.Errorf("work %+v, width %g; want a layout to 30", d, mustFind(t, tr, "sized").Rect.Width())
	}

	// A build that stops reading.
	var v *Value[int]
	var setShow Setter[bool]
	builds := 0
	reader := func() Node {
		builds++
		show, s := UseState(true)
		setShow = s
		v = UseValue(0)
		if show {
			return Textf("%d", v.Get())
		}
		return Text("hidden")
	}
	tr = newTestTree(t, 200, 100, Component(reader), nil)
	tr.Frame()
	setShow.Set(false)
	tr.Frame()
	v.Set(1)
	tr.Frame()
	if builds != 2 || len(v.subs) != 0 {
		t.Errorf("%d builds, %d subscriptions; want 2 and 0", builds, len(v.subs))
	}
}

func TestValueRestyleDropsSubscriptions(t *testing.T) {
	// A rebuild that removes the deferred props of a box ends its
	// subscriptions; one that keeps them evaluates them again.
	var v *Value[float32]
	var setMode Setter[int]
	comp := func() Node {
		mode, s := UseState(0)
		setMode = s
		v = UseValue[float32](20)
		var props []style.Op
		switch mode {
		case 0:
			props = append(props, style.WidthFn(func() float32 { return v.Get() }),
				style.TranslateFn(func() (float32, float32) { return v.Get(), 0 }))
		case 1:
			props = append(props, style.TranslateFn(func() (float32, float32) { return v.Get(), 0 }))
		}
		return div(Key("box"), Class("h-4"), Style(props...))
	}
	tr := newTestTree(t, 200, 100, Component(comp), nil)
	tr.Frame()
	if n := len(v.subs); n != 2 {
		t.Fatalf("%d subscriptions, want 2 (layout and paint)", n)
	}
	setMode.Set(1)
	tr.Frame()
	if n := len(v.subs); n != 1 || v.subs[0].obs.kind != obsPaint {
		t.Fatalf("after removing WidthFn: %d subscriptions, want the paint one", n)
	}
	setMode.Set(2)
	tr.Frame()
	if n := len(v.subs); n != 0 {
		t.Fatalf("after removing every deferred prop: %d subscriptions, want 0", n)
	}
	v.Set(50)
	if tr.ApplyUpdates() {
		t.Error("a Value no prop reads invalidated the box")
	}
}

func TestValueUnmountRemovesSubscriptions(t *testing.T) {
	// The readers unmount.
	var v *Value[float32]
	var setShow Setter[bool]
	reader := func(v *Value[float32]) Node { return Textf("%g", v.Get()) }
	owner := func() Node {
		v = UseValue[float32](1)
		show, s := UseState(true)
		setShow = s
		return div(If(show, Fragment(
			ComponentProps(reader, v),
			div(Class("h-4"), Style(style.WidthFn(func() float32 { return v.Get() * 10 }))),
			div(Class("size-4"), Style(style.TranslateFn(func() (float32, float32) { return v.Get(), 0 }))),
		)))
	}
	tr := newTestTree(t, 200, 100, Component(owner), nil)
	tr.Frame()
	if n := len(v.subs); n != 3 {
		t.Fatalf("%d subscriptions, want 3 (build, layout, paint)", n)
	}
	setShow.Set(false)
	tr.Frame()
	if n := len(v.subs); n != 0 {
		t.Errorf("%d subscriptions after the readers unmounted, want 0", n)
	}
	v.Set(2)
	if tr.ApplyUpdates() {
		t.Error("a change invalidated unmounted readers")
	}

	// The owner unmounts: the readers that remain lose their
	// subscriptions to it, and its queued updates are dropped.
	var shared *Value[int]
	var readerEl *Element
	var setOwner Setter[bool]
	readerBuilds := 0
	ownerComp := func() Node { shared = UseValue(1); return Text("owner") }
	readerComp := func() Node {
		readerBuilds++
		readerEl = current
		return Textf("r%d", shared.Get())
	}
	page := func() Node {
		on, s := UseState(true)
		setOwner = s
		return div(If(on, Component(ownerComp)), Component(readerComp))
	}
	tr = newTestTree(t, 200, 100, Component(page), nil)
	tr.Frame()
	if readerEl.vobs == nil || len(readerEl.vobs.subs) != 1 {
		t.Fatal("the reader did not subscribe to the Value of its sibling")
	}
	dead := shared
	setOwner.Set(false)
	tr.Frame() // the reader builds with its parent and reads the dead Value
	if n := len(readerEl.vobs.subs); n != 0 || len(dead.subs) != 0 {
		t.Errorf("after the owner unmounted: reader has %d subscriptions, Value %d; want 0 and 0", n, len(dead.subs))
	}
	builds := readerBuilds
	dead.Set(9)
	if tr.ApplyUpdates() || tr.Dirty() {
		t.Error("an update of an unmounted Value was applied")
	}
	tr.Frame()
	if readerBuilds != builds || dead.Get() != 1 {
		t.Errorf("reader builds %d (was %d), value %d; want no build and the old value", readerBuilds, builds, dead.Get())
	}
}

type valueProps struct {
	V     *Value[int]
	Label string
}

var valueBuilds = buildCounter{}

func valueLabel(p valueProps) Node {
	valueBuilds[p.Label]++
	return div(Textf("%s %d", p.Label, p.V.Get()))
}

func valueMiddle(v *Value[int]) Node {
	valueBuilds["middle"]++
	return div(ComponentProps(valueLabel, valueProps{v, "deep"}))
}

func TestValueThroughSkippedComponents(t *testing.T) {
	clear(valueBuilds)
	var v *Value[int]
	var setN Setter[int]
	page := func() Node {
		n, s := UseState(0)
		setN = s
		v = UseValue(0)
		valueBuilds["page"]++
		return div(Textf("n %d", n),
			ComponentProps(valueLabel, valueProps{v, "a"}),
			ComponentProps(valueMiddle, v))
	}
	tr := newTestTree(t, 300, 200, Component(page), nil)
	tr.Frame()
	clear(valueBuilds)
	// The *Value is the same pointer on every build: CP skips.
	setN.Set(1)
	tr.Frame()
	if want := (buildCounter{"page": 1}); fmt.Sprint(valueBuilds) != fmt.Sprint(want) {
		t.Errorf("builds %v, want %v", valueBuilds, want)
	}
	// A change builds only the readers, also below a component that
	// does not build.
	clear(valueBuilds)
	v.Set(3)
	tr.Frame()
	if want := (buildCounter{"a": 1, "deep": 1}); fmt.Sprint(valueBuilds) != fmt.Sprint(want) {
		t.Errorf("builds %v, want %v", valueBuilds, want)
	}
	if got := texts(tr); got != "n 1 a 3 deep 3" {
		t.Errorf("texts %q", got)
	}
}

var valueCtx = CreateContext[*Value[float32]](nil)

func TestValueThroughContext(t *testing.T) {
	builds := buildCounter{}
	var v *Value[float32]
	var setN Setter[int]
	buildReader := func() Node {
		builds["build"]++
		return div(Textf("%g", UseContext(valueCtx).Get()))
	}
	paintReader := func() Node {
		builds["paint"]++
		cv := UseContext(valueCtx)
		return div(Key("moving"), Class("size-4 bg-sky-500"), Style(style.TranslateFn(func() (float32, float32) { return cv.Get(), 0 })))
	}
	app := func() Node {
		n, s := UseState(0)
		setN = s
		v = UseValue[float32](0)
		builds["app"]++
		return div(Textf("n %d", n), valueCtx.Provider(v,
			ComponentProps(func(int) Node { return Component(buildReader) }, 0),
			ComponentProps(func(int) Node { return Component(paintReader) }, 0),
		))
	}
	tr := newTestTree(t, 300, 200, Component(app), nil)
	tr.Frame()
	// The provider gets the same *Value: its readers do not build.
	clear(builds)
	setN.Set(1)
	tr.Frame()
	if want := (buildCounter{"app": 1}); fmt.Sprint(builds) != fmt.Sprint(want) {
		t.Errorf("builds %v, want %v", builds, want)
	}
	clear(builds)
	s0 := tr.Stats()
	v.Set(6)
	dl, _ := tr.Frame()
	if want := (buildCounter{"build": 1}); fmt.Sprint(builds) != fmt.Sprint(want) {
		t.Errorf("builds %v, want %v", builds, want)
	}
	if d := work(s0, tr.Stats()); d.Builds != 1 || d.Paints != 1 {
		t.Errorf("work %+v", d)
	}
	if ts := transforms(dl); len(ts) != 1 || ts[0][4] != 6 {
		t.Errorf("transforms %v, want a translation by 6", ts)
	}
	if got := texts(tr); got != "n 1 6" {
		t.Errorf("texts %q", got)
	}
}

func TestValueHostRepaintsWithoutBuilding(t *testing.T) {
	var v *Value[float32]
	builds := 0
	comp := func() Node {
		builds++
		v = UseValue[float32](0)
		return div(Class("size-10 bg-sky-500"), Style(style.TranslateFn(func() (float32, float32) { return v.Get(), 0 })))
	}
	b, h := newHeadlessHost(t, Component(comp))
	win := h.Window().(*headless.Window)
	tr := h.Tree()
	s0 := tr.Stats()
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		v.Set(10)
	}()
	wg.Wait()
	b.Drain(h)
	if !win.FrameRequested() {
		t.Fatal("a Value read in paint did not request a frame")
	}
	b.Advance(b.FrameInterval())
	b.Drain(h)
	if d := work(s0, tr.Stats()); d != paintOnly || builds != 1 {
		t.Errorf("work %+v, %d builds; want a paint only", d, builds)
	}
	if h.Presents() != 2 {
		t.Errorf("presents = %d, want 2", h.Presents())
	}
	// Idle afterwards.
	frames := tr.Stats().Frames
	for i := 0; i < 3; i++ {
		b.Advance(time.Second)
		b.Drain(h)
	}
	if win.FrameRequested() || tr.Stats().Frames != frames {
		t.Error("an idle tree ran frames after the repaint")
	}
	// An equal value requests nothing.
	v.Set(10)
	b.Drain(h)
	if win.FrameRequested() {
		t.Error("an equal value requested a frame")
	}
}

func TestValueHookOrder(t *testing.T) {
	useValue := false
	comp := func() Node {
		if useValue {
			UseValue(0)
		} else {
			UseState(0)
		}
		return Text("c")
	}
	tr := newTestTree(t, 100, 100, Component(comp), nil)
	useValue = true
	msg := panicMessage(func() { tr.Mount(Component(comp)) })
	if !strings.Contains(msg, "UseState[int]") || !strings.Contains(msg, "UseValue[int]") {
		t.Errorf("panic %q, want the old and new hook types", msg)
	}

	useValue = false
	tr = devTree(t, Component(comp), nil)
	useValue = true
	msg = panicMessage(func() { tr.Mount(Component(comp)) })
	if !strings.Contains(msg, "hook 1 is UseValue at core.TestValueHookOrder.func1 (value_test.go:") ||
		!strings.Contains(msg, "but it was UseState at core.TestValueHookOrder.func1 (value_test.go:") {
		t.Errorf("dev panic %q, want both kinds and call sites", msg)
	}
	// The tree still tracks after a build panicked inside tracking.
	if d := tr.trackDepth.Load(); d != 0 {
		t.Errorf("tracking depth %d after a panicking build, want 0", d)
	}
}

func TestValueMisuse(t *testing.T) {
	var nilValue *Value[int]
	for name, f := range map[string]func(){
		"UseValue called outside a component build": func() { UseValue(0) },
		"Get called on a nil *Value":                func() { nilValue.Get() },
		"Set called on a Value not made by":         func() { new(Value[int]).Set(1) },
	} {
		if msg := panicMessage(f); !strings.Contains(msg, name) {
			t.Errorf("panic %q, want %q", msg, name)
		}
	}
	var v *Value[int]
	tr := newTestTree(t, 100, 100, Component(func() Node { v = UseValue(0); return Text("x") }), nil)
	tr.Frame()
	if msg := panicMessage(func() { v.Update(nil) }); !strings.Contains(msg, "Value.Update(nil)") {
		t.Errorf("panic %q", msg)
	}
}

func TestValueClosurePanicKeepsTracking(t *testing.T) {
	var v *Value[float32]
	boom := false
	comp := func() Node {
		v = UseValue[float32](1)
		return div(Class("size-4"), Style(style.TranslateFn(func() (float32, float32) {
			if boom {
				panic("boom")
			}
			return v.Get(), 0
		})))
	}
	tr := newTestTree(t, 100, 100, Component(comp), nil)
	tr.Frame()
	boom = true
	v.Set(2)
	if msg := panicMessage(func() { tr.Frame() }); msg != "boom" {
		t.Fatalf("panic %q, want boom", msg)
	}
	if d := tr.trackDepth.Load(); d != 0 || len(tr.reads) != 0 {
		t.Errorf("tracking depth %d, %d pending reads after a panicking closure; want 0 and 0", d, len(tr.reads))
	}
	if n := len(v.subs); n != 1 {
		t.Errorf("%d subscriptions, want the one of the last evaluation", n)
	}
}

// BenchmarkValuePaint10k changes a Value read by the TranslateFn of one
// box of a 10k-element tree: the frame only repaints.
func BenchmarkValuePaint10k(b *testing.B) {
	var x *Value[float32]
	setters := make([]func(string), benchRows)
	tr := newTestTree(b, 800, 600, Component(func() Node {
		x = UseValue[float32](0)
		return div(
			div(Class("size-4 bg-pink-500"), Style(style.TranslateFn(func() (float32, float32) { return x.Get(), 0 }))),
			ComponentProps(func(int) Node { return bigList(benchRows, setters) }, 0),
		)
	}), nil)
	tr.Frame()
	s0 := tr.Stats()
	i := float32(0)
	b.ReportAllocs()
	for b.Loop() {
		i++
		x.Set(i)
		tr.Frame()
	}
	if d := work(s0, tr.Stats()); d.Builds != 0 || d.Layouts != 0 {
		b.Fatalf("work %+v, want paints only", d)
	}
}

// BenchmarkValueLayout10k changes a Value read by the WidthFn of one box
// next to a 10k-element list: the box is laid out again (the list keeps
// its cached layout) and the tree repainted, without building.
func BenchmarkValueLayout10k(b *testing.B) {
	var w *Value[float32]
	setters := make([]func(string), benchRows)
	tr := newTestTree(b, 800, 600, Component(func() Node {
		w = UseValue[float32](10)
		return div(
			div(Class("h-4 bg-pink-500"), Style(style.WidthFn(func() float32 { return w.Get() }))),
			ComponentProps(func(int) Node { return bigList(benchRows, setters) }, 0),
		)
	}), nil)
	tr.Frame()
	s0 := tr.Stats()
	i := float32(10)
	b.ReportAllocs()
	for b.Loop() {
		i++
		w.Set(i)
		tr.Frame()
	}
	if d := work(s0, tr.Stats()); d.Builds != 0 || d.Styles != 0 || d.Layouts == 0 {
		b.Fatalf("work %+v, want layouts and paints only", d)
	}
}

// BenchmarkStatePaint10k makes the same change with state in the same
// component: it builds (the list below it skips, CP), the moving box is
// restyled, the tree laid out (nothing moved) and repainted.
func BenchmarkStatePaint10k(b *testing.B) {
	var set Setter[float32]
	setters := make([]func(string), benchRows)
	tr := newTestTree(b, 800, 600, Component(func() Node {
		x, s := UseState[float32](0)
		set = s
		return div(
			div(Class("size-4 bg-pink-500"), Style(style.Translate(x, 0))),
			ComponentProps(func(int) Node { return bigList(benchRows, setters) }, 0),
		)
	}), nil)
	tr.Frame()
	i := float32(0)
	b.ReportAllocs()
	for b.Loop() {
		i++
		set.Set(i)
		tr.Frame()
	}
}

// BenchmarkStateRebuildList10k is BenchmarkStatePaint10k with the list
// built by a component that cannot skip (C): the list is built again on
// every change (its rows skip).
func BenchmarkStateRebuildList10k(b *testing.B) {
	var set Setter[float32]
	setters := make([]func(string), benchRows)
	tr := newTestTree(b, 800, 600, Component(func() Node {
		x, s := UseState[float32](0)
		set = s
		return div(
			div(Class("size-4 bg-pink-500"), Style(style.Translate(x, 0))),
			Component(func() Node { return bigList(benchRows, setters) }),
		)
	}), nil)
	tr.Frame()
	i := float32(0)
	b.ReportAllocs()
	for b.Loop() {
		i++
		set.Set(i)
		tr.Frame()
	}
}
