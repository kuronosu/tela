//go:build linux

package wayland

import (
	"image"
	"strings"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/wayland/protocols"
)

func TestSizeChoice(t *testing.T) {
	floating := toplevelStates{activated: true}
	tiled := toplevelStates{tiled: true}
	maximized := toplevelStates{maximized: true}
	pt := func(x, y int) image.Point { return image.Point{X: x, Y: y} }

	c := sizeChoice{current: pt(800, 600), stateless: pt(800, 600), min: pt(200, 100)}
	steps := []struct {
		w, h int32
		st   toplevelStates
		want image.Point
	}{
		{0, 0, floating, pt(800, 600)},       // client decides: requested size
		{1000, 700, floating, pt(1000, 700)}, // suggestion while floating
		{100, 50, floating, pt(200, 100)},    // raised to MinSize
		{1920, 1080, maximized, pt(1920, 1080)},
		{0, 0, floating, pt(200, 100)},   // unmaximize: back to the stateless size
		{150, 80, tiled, pt(150, 80)},    // imposed: not raised
		{0, 0, tiled, pt(150, 80)},       // constrained 0x0 keeps the current size
		{0, 0, floating, pt(200, 100)},   // floating again
		{640, 0, floating, pt(640, 100)}, // one axis decided by the client
	}
	for i, s := range steps {
		if got := c.choose(s.w, s.h, s.st); got != s.want {
			t.Errorf("step %d: choose(%d, %d, %+v) = %v, want %v", i, s.w, s.h, s.st, got, s.want)
		}
	}

	fixed := sizeChoice{current: pt(300, 300), stateless: pt(300, 300), fixed: true}
	if got := fixed.choose(500, 500, floating); got != pt(300, 300) {
		t.Errorf("FixedSize floating suggestion = %v, want the fixed size", got)
	}
	if got := fixed.choose(500, 500, tiled); got != pt(500, 500) {
		t.Errorf("FixedSize tiled = %v, want the imposed size", got)
	}
}

func TestDecodeStates(t *testing.T) {
	var raw []byte
	for _, s := range []protocols.XdgToplevelState{protocols.XdgToplevelStateActivated, protocols.XdgToplevelStateTiledLeft, 99} {
		raw = ne.AppendUint32(raw, uint32(s))
	}
	st := decodeStates(raw)
	if !st.activated || !st.tiled || st.maximized || !st.constrained() {
		t.Errorf("decodeStates = %+v", st)
	}
}

func TestLogicalPoint(t *testing.T) {
	if got := logicalPoint(platform.Size{W: 99.5, H: 0.2}); got != (image.Point{X: 100, Y: 1}) {
		t.Errorf("logicalPoint = %v", got)
	}
	if got := ceilPoint(platform.Size{W: 99.1, H: 0}); got != (image.Point{X: 100, Y: 0}) {
		t.Errorf("ceilPoint = %v", got)
	}
}

func TestScaleFor(t *testing.T) {
	o1, o2 := &output{scale: 1}, &output{scale: 3}
	cases := []struct {
		pref    int32
		entered []*output
		initial int32
		want    int32
	}{
		{0, nil, 2, 2},               // not on any output yet: the guess
		{0, []*output{o1}, 2, 1},     // the output's scale
		{0, []*output{o1, o2}, 1, 3}, // the densest output
		{2, []*output{o1, o2}, 1, 2}, // the compositor's preference wins
	}
	for _, c := range cases {
		if got := scaleFor(c.pref, c.entered, c.initial); got != c.want {
			t.Errorf("scaleFor(%d, %d outputs, %d) = %d, want %d", c.pref, len(c.entered), c.initial, got, c.want)
		}
	}
}

func TestCursorShapeMapping(t *testing.T) {
	// platform.Cursor follows wp_cursor_shape_v1 v1: same names, offset 1.
	for c := platform.CursorDefault; c < platform.CursorNone; c++ {
		shape := cursorShape(c)
		if got, want := shape.String(), strings.ReplaceAll(c.String(), "-", "_"); got != want {
			t.Errorf("cursor %v maps to shape %v", c, got)
		}
	}
	if cursorShape(platform.CursorZoomOut) != protocols.WpCursorShapeDeviceV1ShapeZoomOut {
		t.Errorf("zoom-out mapping is off")
	}
}

func TestPointerButtons(t *testing.T) {
	for code, want := range map[uint32]platform.PointerButton{
		btnLeft: platform.ButtonPrimary, btnRight: platform.ButtonSecondary, btnMiddle: platform.ButtonMiddle,
		btnSide: platform.ButtonBack, btnExtra: platform.ButtonForward, 0x200: platform.ButtonNone,
	} {
		if got := pointerButton(code); got != want {
			t.Errorf("pointerButton(%#x) = %v, want %v", code, got, want)
		}
	}
}

func TestPointerFrameGrouping(t *testing.T) {
	a, b := &Window{}, &Window{}
	var p pointerState
	ms := time.Millisecond

	// Leave and enter in one frame (moving between two windows).
	p.enter(a, 1, 2, 1*ms)
	if got := p.frame(); len(got) != 1 || got[0].w != a || got[0].ev.(platform.PointerEvent).Action != platform.PointerEnter {
		t.Fatalf("enter frame = %+v", got)
	}
	p.leave(2 * ms)
	p.enter(b, 5, 6, 2*ms)
	got := p.frame()
	if len(got) != 2 || got[0].w != a || got[1].w != b ||
		got[0].ev.(platform.PointerEvent).Action != platform.PointerLeave ||
		got[0].ev.(platform.PointerEvent).Pos != (platform.Point{X: 1, Y: 2}) ||
		got[1].ev.(platform.PointerEvent).Action != platform.PointerEnter {
		t.Fatalf("leave+enter frame = %+v", got)
	}

	// Several axis events make one scroll event; touchpads have no steps.
	p.axis(protocols.WlPointerAxisVerticalScroll, 3.5, 10*ms)
	p.axis(protocols.WlPointerAxisVerticalScroll, 1.5, 11*ms)
	p.axis(protocols.WlPointerAxisHorizontalScroll, -2, 11*ms)
	got = p.frame()
	if len(got) != 1 {
		t.Fatalf("scroll frame = %+v", got)
	}
	se := got[0].ev.(platform.ScrollEvent)
	if se.Delta != (platform.Point{X: -2, Y: 5}) || se.Steps != (platform.Point{}) || se.Time != 10*ms || got[0].w != b {
		t.Errorf("touchpad scroll = %+v", se)
	}

	// Legacy discrete steps (seat versions 5-7).
	p.axis(protocols.WlPointerAxisVerticalScroll, -10, 12*ms)
	p.axisDiscrete(protocols.WlPointerAxisVerticalScroll, -1)
	if se := p.frame()[0].ev.(platform.ScrollEvent); se.Steps != (platform.Point{Y: -1}) || se.Delta.Y != -10 {
		t.Errorf("discrete scroll = %+v", se)
	}

	// Buttons: the mask follows presses and releases; unknown buttons and
	// stray releases are dropped.
	p.button(btnRight, true, 13*ms)
	p.button(0x200, true, 13*ms)
	p.button(btnLeft, false, 13*ms)
	p.button(btnRight, false, 14*ms)
	got = p.frame()
	if len(got) != 2 {
		t.Fatalf("button frame = %+v", got)
	}
	if e := got[0].ev.(platform.PointerEvent); e.Action != platform.PointerDown || e.Button != platform.ButtonSecondary || e.Buttons != platform.ButtonSecondary.Mask() {
		t.Errorf("press = %+v", e)
	}
	if e := got[1].ev.(platform.PointerEvent); e.Action != platform.PointerUp || e.Buttons != 0 || e.Time != 14*ms {
		t.Errorf("release = %+v", e)
	}

	// Nothing is reported without focus.
	p.leave(15 * ms)
	p.frame()
	p.motion(1, 1, 16*ms)
	p.axis(protocols.WlPointerAxisVerticalScroll, 1, 16*ms)
	p.button(btnLeft, true, 16*ms)
	if got := p.frame(); len(got) != 0 {
		t.Errorf("events without focus: %+v", got)
	}
}

// memChain returns a swapchain on plain memory with RGBA byte order.
func memChain(allocs *int) *swapchain {
	return &swapchain{
		alloc: func(size image.Point) (*shmBuffer, error) {
			*allocs++
			return &shmBuffer{data: make([]byte, 4*size.X*size.Y), size: size, stride: 4 * size.X}, nil
		},
		copyRect: copyXBGR,
	}
}

func TestSwapchainNeverWritesBusyBuffers(t *testing.T) {
	allocs := 0
	c := memChain(&allocs)
	size := image.Point{X: 4, Y: 4}
	fb := image.NewRGBA(image.Rectangle{Max: size})
	full := []image.Rectangle{fb.Rect}

	var held []*shmBuffer
	for i := range 3 {
		buf, err := c.acquire(size)
		if err != nil {
			t.Fatal(err)
		}
		for _, h := range held {
			if h == buf {
				t.Fatalf("frame %d: acquire returned a busy buffer", i)
			}
		}
		c.fill(buf, fb, full)
		c.submitted(buf, full)
		held = append(held, buf)
	}
	if allocs != 3 {
		t.Errorf("allocated %d buffers for 3 held frames", allocs)
	}
	c.released(held[1])
	if buf, _ := c.acquire(size); buf != held[1] || allocs != 3 {
		t.Errorf("the released buffer was not reused")
	}

	// A resize retires the old buffers: busy ones die on release.
	destroyed := 0
	for _, b := range c.bufs {
		b.free = func() { destroyed++ }
	}
	if _, err := c.acquire(image.Point{X: 2, Y: 2}); err != nil {
		t.Fatal(err)
	}
	if destroyed != 1 { // held[1] was free
		t.Errorf("free old buffers destroyed = %d, want 1", destroyed)
	}
	c.released(held[0])
	c.released(held[2])
	if destroyed != 3 {
		t.Errorf("old buffers destroyed after release = %d, want 3", destroyed)
	}
	if c.prev != nil {
		t.Errorf("prev survived a resize")
	}
}

func TestSwapchainStaleCollapse(t *testing.T) {
	allocs := 0
	c := memChain(&allocs)
	size := image.Point{X: 64, Y: 1}
	fb := image.NewRGBA(image.Rectangle{Max: size})
	a, _ := c.acquire(size)
	c.fill(a, fb, []image.Rectangle{fb.Rect})
	c.submitted(a, []image.Rectangle{fb.Rect})
	b, _ := c.acquire(size)
	c.fill(b, fb, nil)
	c.submitted(b, nil)
	// a stays busy (held by the compositor) while b is presented again and
	// again: a misses every damage, and its list stays bounded.
	for x := range 40 {
		c.released(b)
		d := []image.Rectangle{image.Rect(x, 0, x+1, 1)}
		buf, _ := c.acquire(size)
		if buf != b {
			t.Fatalf("acquire returned a busy buffer")
		}
		c.fill(b, fb, d)
		c.submitted(b, d)
	}
	if n := len(a.stale); n > maxStaleRects {
		t.Errorf("stale list has %d rects, limit %d", n, maxStaleRects)
	}
	u := image.Rectangle{}
	for _, r := range a.stale {
		u = u.Union(r)
	}
	if u != image.Rect(0, 0, 40, 1) {
		t.Errorf("stale union = %v, want (0,0)-(40,1)", u)
	}
}
