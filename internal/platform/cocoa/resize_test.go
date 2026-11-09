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
