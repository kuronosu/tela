package platform

import (
	"image"
	"image/color"
	"strings"
	"testing"
	"time"
)

func expectPanic(t *testing.T, contains string, f func()) {
	t.Helper()
	defer func() {
		t.Helper()
		r := recover()
		if r == nil {
			t.Errorf("no panic, want one containing %q", contains)
		} else if s, _ := r.(string); !strings.Contains(s, contains) {
			t.Errorf("panic %v does not contain %q", r, contains)
		}
	}()
	f()
}

func TestHitTest(t *testing.T) {
	regions := []HitRegion{
		{Kind: HitCaption, Rect: Rect{W: 400, H: 30}},
		{Kind: HitClient, Rect: Rect{X: 100, W: 50, H: 30}},
		{Kind: HitMinimize, Rect: Rect{X: 310, W: 30, H: 30}},
		{Kind: HitMaximize, Rect: Rect{X: 340, W: 30, H: 30}},
		{Kind: HitClose, Rect: Rect{X: 370, W: 30, H: 30}},
	}
	for _, c := range []struct {
		p    Point
		want HitKind
	}{
		{Point{0, 0}, HitCaption},
		{Point{99.5, 29.9}, HitCaption},
		{Point{100, 10}, HitClient}, // the hole is listed later
		{Point{150, 10}, HitCaption},
		{Point{345, 5}, HitMaximize},
		{Point{399.9, 0}, HitClose},
		{Point{400, 0}, HitClient}, // right edge excluded
		{Point{10, 30}, HitClient}, // bottom edge excluded
		{Point{-1, 5}, HitClient},
	} {
		if got := HitTest(regions, c.p); got != c.want {
			t.Errorf("HitTest(%v) = %v, want %v", c.p, got, c.want)
		}
	}
	if HitTest(nil, Point{}) != HitClient {
		t.Errorf("HitTest without regions")
	}
	if !EqualHitRegions(nil, []HitRegion{}) || EqualHitRegions(regions, regions[:2]) || !EqualHitRegions(regions, append([]HitRegion(nil), regions...)) {
		t.Errorf("EqualHitRegions")
	}
	CheckHitRegions(regions)
	expectPanic(t, "invalid kind", func() { CheckHitRegions([]HitRegion{{Kind: HitClose + 1}}) })
	expectPanic(t, "negative size", func() { CheckHitRegions([]HitRegion{{Rect: Rect{H: -1}}}) })
	expectPanic(t, "finite", func() { CheckHitRegions([]HitRegion{{Rect: Rect{X: 1e9}}}) })
	if HitMaximize.String() != "maximize" || HitKind(77).String() != "HitKind(77)" {
		t.Errorf("HitKind names")
	}
}

func TestCaptionClicks(t *testing.T) {
	const ms = time.Millisecond
	var c CaptionClicks
	set := func(d time.Duration) SystemSettings { return SystemSettings{DoubleClickInterval: d} }
	steps := []struct {
		t        time.Duration
		pos      Point
		interval time.Duration
		want     bool
	}{
		{0, Point{}, 0, false},
		{500 * ms, Point{X: 3, Y: 4}, 0, true},           // the default, at the slop
		{600 * ms, Point{X: 3, Y: 4}, 0, false},          // a double click is not recorded
		{1101 * ms, Point{X: 3, Y: 4}, 0, false},         // too late: starts over
		{1200 * ms, Point{X: 8.1, Y: 4}, 0, false},       // too far
		{1300 * ms, Point{X: 8.1, Y: 4}, 50 * ms, false}, // the setting wins
		{1350 * ms, Point{X: 8.1, Y: 4}, -1, true},       // negative is unknown too
		{1400 * ms, Point{}, 0, false},
		{1300 * ms, Point{}, 0, false}, // the clock going back is not a double click
	}
	for i, s := range steps {
		if got := c.Press(s.t, s.pos, set(s.interval)); got != s.want {
			t.Errorf("press %d: double click %v, want %v", i, got, s.want)
		}
	}
	c.Press(2*time.Second, Point{}, SystemSettings{})
	c.Reset()
	if c.Press(2*time.Second+ms, Point{}, SystemSettings{}) {
		t.Error("a press after Reset completes a double click")
	}
}

func TestDoubleClickTime(t *testing.T) {
	for _, c := range []struct{ in, want time.Duration }{
		{0, DefaultDoubleClickInterval}, {-1, DefaultDoubleClickInterval}, {300 * time.Millisecond, 300 * time.Millisecond},
	} {
		if got := (SystemSettings{DoubleClickInterval: c.in}).DoubleClickTime(); got != c.want {
			t.Errorf("DoubleClickTime with %v = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestIcons(t *testing.T) {
	img := func(w, h int) image.Image { return image.NewRGBA(image.Rect(0, 0, w, h)) }
	icon := []image.Image{img(32, 32), img(16, 16), img(64, 64), img(48, 24)}
	for _, c := range []struct{ side, want int }{{16, 16}, {17, 48}, {24, 48}, {25, 32}, {33, 64}, {256, 64}, {1, 16}} {
		if got := PickIcon(icon, c.side).Bounds().Dx(); got != c.want {
			t.Errorf("PickIcon(%d) has width %d, want %d", c.side, got, c.want)
		}
	}
	if PickIcon(nil, 16) != nil {
		t.Errorf("PickIcon of no images")
	}
	CheckIcon("x", icon)
	CheckIcon("x", nil)
	expectPanic(t, "image 1 is nil", func() { CheckIcon("x", []image.Image{img(1, 1), nil}) })
	expectPanic(t, "empty", func() { CheckIcon("x", []image.Image{img(0, 4)}) })
	expectPanic(t, "at most", func() { CheckIcon("x", []image.Image{img(MaxIconSide+1, 1)}) })

	// IconNRGBA un-premultiplies and moves the origin to (0, 0).
	src := image.NewRGBA(image.Rect(10, 10, 12, 12))
	src.SetRGBA(10, 10, color.RGBA{0x40, 0, 0, 0x80})
	n := IconNRGBA(src)
	if n.Rect != image.Rect(0, 0, 2, 2) {
		t.Fatalf("IconNRGBA bounds %v", n.Rect)
	}
	if c := n.NRGBAAt(0, 0); c.A != 0x80 || c.R < 0x7f || c.R > 0x80 {
		t.Errorf("IconNRGBA pixel %v, want about {0x80 0 0 0x80}", c)
	}
	cp := CopyIcon([]image.Image{src})
	src.SetRGBA(10, 10, color.RGBA{})
	if cp[0].(*image.NRGBA).NRGBAAt(0, 0).A != 0x80 || CopyIcon(nil) != nil {
		t.Errorf("CopyIcon does not copy")
	}
}

func TestWindowStateString(t *testing.T) {
	if s := (WindowState{}).String(); s != "{}" {
		t.Errorf("zero state = %q", s)
	}
	s := WindowState{Maximized: true, Fullscreen: true, Minimized: true, ClientSideDecorations: true, HasPosition: true, Position: image.Pt(-3, 4)}
	if got := s.String(); got != "{maximized fullscreen minimized csd at -3,4}" {
		t.Errorf("String = %q", got)
	}
}
