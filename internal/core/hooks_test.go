package core

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// newHeadlessHost mounts root in a 400x300 headless window and delivers
// the first configure and frame.
func newHeadlessHost(t *testing.T, root Node) (*headless.Backend, *Host) {
	t.Helper()
	b := headless.New(headless.Options{Size: platform.Size{W: 400, H: 300}})
	h, err := NewHost(b, Options{Config: Config{Fonts: testFonts}, Root: root})
	if err != nil {
		t.Fatal(err)
	}
	b.Drain(h)
	if h.Presents() != 1 {
		t.Fatalf("after the first configure: %d presents, want 1", h.Presents())
	}
	return b, h
}

func TestSetStateFromGoroutine(t *testing.T) {
	var set func(int)
	builds := 0
	counter := func() Node {
		builds++
		n, s := UseState(0)
		set = s
		return div(Textf("count %d", n))
	}
	b, h := newHeadlessHost(t, Component(counter))
	win := h.Window().(*headless.Window)
	tr := h.Tree()
	if builds != 1 {
		t.Fatalf("builds = %d after mount, want 1", builds)
	}
	frames := tr.Stats().Frames

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		set(1)
	}()
	wg.Wait()

	// The setter woke the loop; the update is applied on the WakeEvent,
	// which requests a frame. The window drew at time 0, so the frame is
	// paced to the next refresh.
	b.Drain(h)
	if !win.FrameRequested() {
		t.Fatal("the state update did not request a frame")
	}
	if builds != 1 {
		t.Errorf("the component built before the frame (%d builds)", builds)
	}
	b.Advance(b.FrameInterval())
	b.Drain(h)
	if builds != 2 {
		t.Errorf("builds = %d, want 2 (exactly one rebuild)", builds)
	}
	if got := tr.Stats().Frames - frames; got != 1 {
		t.Errorf("%d frames ran, want 1", got)
	}
	if h.Presents() != 2 {
		t.Errorf("presents = %d, want 2", h.Presents())
	}
	if got := texts(tr); got != "count 1" {
		t.Errorf("text = %q, want count 1", got)
	}

	// Setting the same value does not rebuild.
	set(1)
	b.Drain(h)
	b.Advance(time.Second)
	b.Drain(h)
	if builds != 2 || win.FrameRequested() {
		t.Errorf("setting an equal value rebuilt (builds %d) or requested a frame", builds)
	}
}

func TestIdleNoFrames(t *testing.T) {
	b, h := newHeadlessHost(t, div(Class("p-4"), Text("idle")))
	win := h.Window().(*headless.Window)
	frames := h.Tree().Stats().Frames
	for i := 0; i < 5; i++ {
		b.Advance(time.Second)
		b.Drain(h)
	}
	if win.FrameRequested() {
		t.Error("an idle tree requested a frame")
	}
	if got := h.Tree().Stats().Frames - frames; got != 0 {
		t.Errorf("%d frames while idle, want 0", got)
	}
	if h.Presents() != 1 {
		t.Errorf("presents = %d while idle, want 1", h.Presents())
	}
	// A Wake with no updates does not request a frame either.
	b.Wake()
	b.Drain(h)
	if win.FrameRequested() {
		t.Error("an empty wake requested a frame")
	}
}

func TestRootSizedOnConfigure(t *testing.T) {
	b, h := newHeadlessHost(t, div(Key("page"), Class("h-full bg-white"), Text("x")))
	win := h.Window().(*headless.Window)
	check := func(w, hh float32) {
		t.Helper()
		root := h.Tree().Inspect()
		if root.Rect != (gfx.Rect{X1: w, Y1: hh}) {
			t.Errorf("root rect %v, want 0,0 %gx%g", root.Rect, w, hh)
		}
		page := find(&root, "page")
		if page == nil || page.Rect.Width() != w || page.Rect.Height() != hh {
			t.Errorf("h-full page %+v, want %gx%g", page, w, hh)
		}
	}
	check(400, 300)
	win.Resize(platform.Size{W: 640, H: 480})
	b.Drain(h)
	check(640, 480)
	if f := win.Frame(); f == nil || f.Rect.Dx() != 640 || f.Rect.Dy() != 480 {
		t.Errorf("presented frame %v, want 640x480", f.Bounds())
	}
	// A scale change keeps the logical layout and renders more pixels.
	win.SetScale(2)
	b.Drain(h)
	check(640, 480)
	if f := win.Frame(); f == nil || f.Rect.Dx() != 1280 || f.Rect.Dy() != 960 {
		t.Errorf("presented frame %v at scale 2, want 1280x960", f.Bounds())
	}
}

func TestCloseQuits(t *testing.T) {
	b, h := newHeadlessHost(t, Text("bye"))
	h.Window().(*headless.Window).RequestClose()
	b.Drain(h)
	if !b.Quitting() {
		t.Error("closing the window did not quit")
	}
}

func TestHookMisuse(t *testing.T) {
	defer func() {
		r := recover()
		if r == nil || !strings.Contains(r.(string), "outside a component") {
			t.Errorf("UseState outside a build: recover() = %v", r)
		}
	}()
	UseState(1)
}

func TestHookCountChange(t *testing.T) {
	extra := false
	comp := func() Node {
		UseState(0)
		if extra {
			UseState("x")
		}
		return Text("c")
	}
	tr := newTestTree(t, 100, 100, Component(comp), nil)
	tr.Frame()
	extra = true
	defer func() {
		r := recover()
		if r == nil || !strings.Contains(r.(string), "hooks") {
			t.Errorf("recover() = %v, want a hook-order panic", r)
		}
	}()
	tr.Mount(Component(comp))
}

func TestCustomHookBelongsToCaller(t *testing.T) {
	// A component function called directly is a custom hook: its state
	// lives in the caller's Element.
	var setInner func(int)
	inner := func() Node {
		v, s := UseState(10)
		setInner = s
		return Textf("inner %d", v)
	}
	outerBuilds := 0
	outer := func() Node {
		outerBuilds++
		return div(inner())
	}
	tr := newTestTree(t, 200, 100, Component(outer), nil)
	tr.Frame()
	setInner(11)
	tr.Frame()
	if outerBuilds != 2 {
		t.Errorf("outer built %d times, want 2 (the inlined hook rebuilds its caller)", outerBuilds)
	}
	if got := texts(tr); got != "inner 11" {
		t.Errorf("got %q", got)
	}
}

func TestUnmountedUpdateIgnored(t *testing.T) {
	var set func(int)
	child := func() Node {
		v, s := UseState(0)
		set = s
		return Textf("%d", v)
	}
	tr := newTestTree(t, 100, 100, div(Component(child)), nil)
	tr.Frame()
	rerender(tr, div())
	builds := tr.Stats().Builds
	set(3)
	if tr.ApplyUpdates() {
		t.Error("an update of an unmounted component reported work")
	}
	tr.Frame()
	if tr.Stats().Builds != builds {
		t.Error("an unmounted component was built")
	}
}
