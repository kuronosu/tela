package cocoa

import (
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

func TestResizedFrame(t *testing.T) {
	f := [4]float64{100, 100, 300, 200} // x, y (bottom-left, y up), w, h
	lo, hi := [2]float64{100, 80}, [2]float64{500, noMaxSize}
	for _, tc := range []struct {
		edge   platform.Edge
		dx, dy float64
		want   [4]float64
	}{
		{platform.EdgeRight, 50, 0, [4]float64{100, 100, 350, 200}},
		{platform.EdgeLeft, 50, 0, [4]float64{150, 100, 250, 200}}, // the right side stays
		{platform.EdgeTop, 0, 30, [4]float64{100, 100, 300, 230}},  // y up: the top rises
		{platform.EdgeBottom, 0, 30, [4]float64{100, 130, 300, 170}},
		{platform.EdgeBottom | platform.EdgeRight, 20, -20, [4]float64{100, 80, 320, 220}},
		{platform.EdgeLeft, 250, 0, [4]float64{300, 100, 100, 200}},  // clamped to the minimum width
		{platform.EdgeRight, 400, 0, [4]float64{100, 100, 500, 200}}, // and the maximum
		{platform.EdgeTop | platform.EdgeLeft, -10, 10, [4]float64{90, 100, 310, 210}},
	} {
		if got := resizedFrame(f, tc.edge, tc.dx, tc.dy, lo, hi); got != tc.want {
			t.Errorf("resizedFrame(%v, %v, %v) = %v, want %v", tc.edge, tc.dx, tc.dy, got, tc.want)
		}
	}
}

// The resize loop's limits are frame sizes: the content limits plus the
// frame's decoration, and minResizeSide on an axis without MinSize, so a
// window is never shrunk past grabbing (a zero-width frame before).
func TestResizeLimits(t *testing.T) {
	for _, c := range []struct {
		name     string
		min, max platform.Size
		deco     [2]float64
		lo, hi   [2]float64
	}{
		{"no limits, frameless", platform.Size{}, platform.Size{}, [2]float64{},
			[2]float64{minResizeSide, minResizeSide}, [2]float64{noMaxSize, noMaxSize}},
		{"limits, frameless", platform.Size{W: 100, H: 80}, platform.Size{W: 400, H: 300}, [2]float64{},
			[2]float64{100, 80}, [2]float64{400, 300}},
		{"limits, titled (a 28 pt title bar)", platform.Size{W: 100, H: 80}, platform.Size{W: 400, H: 300}, [2]float64{0, 28},
			[2]float64{100, 108}, [2]float64{400, 328}},
		// The app's own minimum stands, even below minResizeSide.
		{"a small MinSize on one axis", platform.Size{W: 10}, platform.Size{W: 20}, [2]float64{},
			[2]float64{10, minResizeSide}, [2]float64{20, noMaxSize}},
	} {
		lo, hi := resizeLimits(c.min, c.max, c.deco)
		if lo != c.lo || hi != c.hi {
			t.Errorf("%s: %v, %v; want %v, %v", c.name, lo, hi, c.lo, c.hi)
		}
	}
}

func TestCaptionWatch(t *testing.T) {
	var c captionWatch
	if c.end([2]float64{}, [2]float64{}) {
		t.Error("a click without a press")
	}
	c.start([2]float64{10, 10}, [2]float64{100, 100})
	c.moved([2]float64{14, 13})
	if !c.end([2]float64{12, 12}, [2]float64{100, 100}) {
		t.Error("a still press is no click")
	}
	c.start([2]float64{10, 10}, [2]float64{100, 100})
	c.moved([2]float64{30, 10}) // dragged away, then back
	if c.end([2]float64{10, 10}, [2]float64{100, 100}) {
		t.Error("a drag there and back is a click")
	}
	c.start([2]float64{10, 10}, [2]float64{100, 100})
	if c.end([2]float64{10, 10}, [2]float64{120, 100}) {
		t.Error("a press that moved the window is a click")
	}
	if c.active {
		t.Error("still watching after the end")
	}
}
