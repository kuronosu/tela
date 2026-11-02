package cocoa

import (
	"image"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/platform"
)

func TestOverlayCaptionHeight(t *testing.T) {
	controls := platform.Rect{X: 8, Y: 7, W: 54, H: 16}
	caption := func(x, y, w, h float32) platform.HitRegion {
		return platform.HitRegion{Kind: platform.HitCaption, Rect: platform.Rect{X: x, Y: y, W: w, H: h}}
	}
	cases := []struct {
		name    string
		regions []platform.HitRegion
		h       float32
		ok      bool
	}{
		{"no regions", nil, 0, false},
		{"a caption along the top", []platform.HitRegion{caption(0, 0, 800, 52)}, 52, true},
		{"the top-most wins", []platform.HitRegion{caption(0, 0, 800, 52), caption(0, 0, 400, 40)}, 40, true},
		{"not at the top", []platform.HitRegion{caption(0, 10, 800, 52)}, 0, false},
		{"not over the controls", []platform.HitRegion{caption(100, 0, 700, 52)}, 0, false},
		{"lower than the controls", []platform.HitRegion{caption(0, 0, 800, 12)}, 0, false},
		{"a client region is not a caption", []platform.HitRegion{{Kind: platform.HitClient, Rect: platform.Rect{W: 800, H: 52}}}, 0, false},
		{"a later client hole keeps the caption", []platform.HitRegion{caption(0, 0, 800, 44),
			{Kind: platform.HitClient, Rect: platform.Rect{X: 300, Y: 4, W: 200, H: 36}}}, 44, true},
	}
	for _, c := range cases {
		if h, ok := overlayCaptionHeight(c.regions, controls); h != c.h || ok != c.ok {
			t.Errorf("%s: overlayCaptionHeight = %v, %v; want %v, %v", c.name, h, ok, c.h, c.ok)
		}
	}
}

func TestDesktopPosition(t *testing.T) {
	const primaryTop = 768
	// A 200x150 content area whose top-left is 100 points right of and
	// 40 points below the top-left of the primary screen.
	content := cocoaRect{X: 100, Y: primaryTop - 40 - 150, W: 200, H: 150}
	if got := desktopPosition(content, primaryTop); got != image.Pt(100, 40) {
		t.Errorf("desktopPosition = %v, want (100,40)", got)
	}
	// A screen left of and above the primary one: negative coordinates.
	left := cocoaRect{X: -1500.4, Y: 900, W: 300, H: 100}
	if got := desktopPosition(left, primaryTop); got != image.Pt(-1500, -232) {
		t.Errorf("desktopPosition on a screen up left = %v, want (-1500,-232)", got)
	}
	// Half points round away from zero.
	if got := desktopPosition(cocoaRect{X: 10.5, Y: primaryTop - 20.5 - 10, W: 10, H: 10}, primaryTop); got != image.Pt(11, 21) {
		t.Errorf("desktopPosition of half points = %v, want (11,21)", got)
	}
}

func TestFrameTopLeftRoundTrip(t *testing.T) {
	const primaryTop = 900
	// A titled window: a 28-point title bar above the content.
	frame := cocoaRect{X: 300, Y: 200, W: 400, H: 328}
	content := cocoaRect{X: 300, Y: 200, W: 400, H: 300}
	for _, pos := range []image.Point{{0, 0}, {120, 160}, {-800, 40}, {2000, 1200}} {
		x, y := frameTopLeft(pos, frame, content, primaryTop)
		// The window moved there: its frame's top-left is (x, y).
		moved := cocoaRect{X: x, Y: y - frame.H, W: frame.W, H: frame.H}
		movedContent := cocoaRect{X: moved.X + (content.X - frame.X), Y: moved.Y, W: content.W, H: content.H}
		if got := desktopPosition(movedContent, primaryTop); got != pos {
			t.Errorf("titled window: SetPosition(%v) then the reported position is %v", pos, got)
		}
	}
	// A full-size content view: the content is the frame.
	x, y := frameTopLeft(image.Pt(50, 60), frame, frame, primaryTop)
	if x != 50 || y != primaryTop-60 {
		t.Errorf("full-size content: frame top-left (%v,%v), want (50,%v)", x, y, primaryTop-60)
	}
}

func TestSizeLimits(t *testing.T) {
	cases := []struct {
		min, max platform.Size
		lo, hi   [2]float64
	}{
		{platform.Size{}, platform.Size{}, [2]float64{0, 0}, [2]float64{noMaxSize, noMaxSize}},
		{platform.Size{W: 100, H: 80}, platform.Size{W: 400, H: 300}, [2]float64{100, 80}, [2]float64{400, 300}},
		// The minimum wins where the maximum is below it.
		{platform.Size{W: 400, H: 300}, platform.Size{W: 350, H: 500}, [2]float64{400, 300}, [2]float64{400, 500}},
		// A zero component is no limit on that axis.
		{platform.Size{H: 50}, platform.Size{W: 200}, [2]float64{0, 50}, [2]float64{200, noMaxSize}},
	}
	for _, c := range cases {
		lo, hi := sizeLimits(c.min, c.max)
		if lo != c.lo || hi != c.hi {
			t.Errorf("sizeLimits(%v, %v) = %v, %v; want %v, %v", c.min, c.max, lo, hi, c.lo, c.hi)
		}
	}
}

func TestCaptionInput(t *testing.T) {
	const ms = time.Millisecond
	regions := []platform.HitRegion{
		{Kind: platform.HitCaption, Rect: platform.Rect{W: 300, H: 40}},
		{Kind: platform.HitClient, Rect: platform.Rect{X: 100, Y: 5, W: 60, H: 30}},
		{Kind: platform.HitClose, Rect: platform.Rect{X: 260, Y: 5, W: 30, H: 30}},
	}
	var s platform.SystemSettings // the default double-click time, 500 ms
	pt := func(x, y float32) platform.Point { return platform.Point{X: x, Y: y} }
	var c captionInput
	steps := []struct {
		btn     platform.PointerButton
		pos     platform.Point
		mods    platform.Modifiers
		t       time.Duration
		content platform.Buttons
		want    captionAction
	}{
		{platform.ButtonPrimary, pt(50, 20), 0, 0, 0, captionDrag},
		{platform.ButtonPrimary, pt(52, 21), 0, 300 * ms, 0, captionDoubleClick},
		{platform.ButtonPrimary, pt(52, 21), 0, 400 * ms, 0, captionDrag},     // a third press starts over
		{platform.ButtonPrimary, pt(120, 20), 0, 500 * ms, 0, captionContent}, // the hole
		{platform.ButtonPrimary, pt(52, 21), 0, 600 * ms, 0, captionDrag},     // the content click broke the pair
		{platform.ButtonPrimary, pt(270, 20), 0, 700 * ms, 0, captionContent}, // the app's close button
		{platform.ButtonPrimary, pt(50, 100), 0, 800 * ms, 0, captionContent}, // below the caption
		{platform.ButtonSecondary, pt(50, 20), 0, 900 * ms, 0, captionSwallow},
		{platform.ButtonMiddle, pt(50, 20), 0, 1000 * ms, 0, captionSwallow},
		{platform.ButtonPrimary, pt(50, 20), platform.ModCtrl, 1100 * ms, 0, captionSwallow}, // a context click
		// A press in the caption while a content button is held belongs
		// to the content sequence.
		{platform.ButtonSecondary, pt(50, 20), 0, 1200 * ms, platform.ButtonPrimary.Mask(), captionContent},
	}
	for i, st := range steps {
		if got := c.press(st.btn, st.pos, st.mods, st.t, regions, st.content, s); got != st.want {
			t.Errorf("press %d (%v at %v): %d, want %d", i, st.btn, st.pos, got, st.want)
		}
	}
	// The caption presses of the secondary and middle buttons are held:
	// their releases are swallowed once.
	if !c.release(platform.ButtonMiddle) || c.release(platform.ButtonMiddle) {
		t.Errorf("the middle release after a caption press is swallowed exactly once")
	}
	if !c.drag(0) {
		t.Errorf("a drag while the primary caption press is held is swallowed")
	}
	if c.drag(platform.ButtonPrimary.Mask()) {
		t.Errorf("a drag of a content sequence is delivered")
	}
	c.moved()
	if c.drag(0) || c.release(platform.ButtonPrimary) || c.release(platform.ButtonSecondary) {
		t.Errorf("a move without buttons ends every caption sequence")
	}
	// A caption press whose release the window server kept, then a press
	// in the content: that sequence's release is delivered.
	c.press(platform.ButtonPrimary, pt(50, 20), 0, 5*time.Second, regions, 0, s)
	c.press(platform.ButtonPrimary, pt(50, 100), 0, 6*time.Second, regions, 0, s)
	if c.release(platform.ButtonPrimary) {
		t.Errorf("the release of a content press after a lost caption release is swallowed")
	}
}

func TestDoubleClickAction(t *testing.T) {
	cases := []struct {
		action    string
		has, mini bool
		want      titleBarAction
	}{
		{"", false, false, titleBarZoom},
		{"", false, true, titleBarMinimize},
		{"Maximize", true, true, titleBarZoom},
		{"Fill", true, false, titleBarZoom},
		{"Minimize", true, false, titleBarMinimize},
		{"None", true, false, titleBarNone},
		{"something new", true, false, titleBarZoom},
	}
	for _, c := range cases {
		if got := doubleClickAction(c.action, c.has, c.mini); got != c.want {
			t.Errorf("doubleClickAction(%q, %v, %v) = %v, want %v", c.action, c.has, c.mini, got, c.want)
		}
	}
}

func TestBackdropMaterial(t *testing.T) {
	for _, c := range []struct {
		b     platform.Backdrop
		m     int
		glass bool
	}{
		{platform.BackdropDefault, 0, false},
		{platform.BackdropNone, 0, false},
		{platform.BackdropMica, materialWindowBackground, true},
		{platform.BackdropMicaAlt, materialUnderWindowBackground, true},
		{platform.BackdropAcrylic, materialSidebar, true},
	} {
		m, ok := backdropMaterial(c.b)
		if m != c.m || ok != c.glass || ok != c.b.ClientBackdrop() {
			t.Errorf("backdropMaterial(%v) = %d, %v; want %d, %v", c.b, m, ok, c.m, c.glass)
		}
	}
}
