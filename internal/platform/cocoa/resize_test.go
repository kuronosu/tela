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
// frame's decoration, and the resize floor on an axis without MinSize
// (ADR-261), so a window is never shrunk past grabbing its handles again.
func TestResizeLimits(t *testing.T) {
	// A 400x300 window whose card sits in a 60 pt shadow margin: the
	// floor keeps 32 pt of handles and the margin (152 by 152).
	floor := platform.ResizeFloor(platform.Size{W: 400, H: 300}, []platform.HitRegion{
		{Kind: platform.HitResizeTopLeft, Rect: platform.Rect{X: 60, Y: 60, W: 16, H: 5}},
		{Kind: platform.HitResizeBottomRight, Rect: platform.Rect{X: 324, Y: 235, W: 16, H: 5}},
	})
	for _, c := range []struct {
		name     string
		min, max platform.Size
		deco     [2]float64
		floor    platform.Size
		lo, hi   [2]float64
	}{
		{"no limits, frameless", platform.Size{}, platform.Size{}, [2]float64{}, platform.Size{W: 32, H: 32},
			[2]float64{32, 32}, [2]float64{noMaxSize, noMaxSize}},
		{"no limits, a card in a shadow margin", platform.Size{}, platform.Size{}, [2]float64{}, floor,
			[2]float64{152, 152}, [2]float64{noMaxSize, noMaxSize}},
		{"limits, frameless", platform.Size{W: 100, H: 80}, platform.Size{W: 400, H: 300}, [2]float64{}, floor,
			[2]float64{100, 80}, [2]float64{400, 300}},
		{"limits, titled (a 28 pt title bar)", platform.Size{W: 100, H: 80}, platform.Size{W: 400, H: 300}, [2]float64{0, 28}, floor,
			[2]float64{100, 108}, [2]float64{400, 328}},
		{"titled, no limits", platform.Size{}, platform.Size{}, [2]float64{0, 28}, platform.Size{W: 32, H: 32},
			[2]float64{32, 60}, [2]float64{noMaxSize, noMaxSize}},
		// The app's own minimum stands, even below the floor.
		{"a small MinSize on one axis", platform.Size{W: 10}, platform.Size{W: 20}, [2]float64{}, floor,
			[2]float64{10, 152}, [2]float64{20, noMaxSize}},
	} {
		lo, hi := resizeLimits(c.min, c.max, c.deco, c.floor)
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
