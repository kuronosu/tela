package win32

import (
	"image"
	"testing"
)

// The visible frame drops the invisible resize borders of Windows 10 and
// 11 on the left, right and bottom: (7, 0, 7, 7) inside the outer
// rectangle at 96 DPI, 11 at 144 DPI (SM_CXSIZEFRAME 6 plus
// SM_CXPADDEDBORDER 6, less the 1-pixel border).
func TestVisibleFrame(t *testing.T) {
	client := image.Rect(100, 50, 900, 650)
	for _, tt := range []struct {
		name        string
		outer, want image.Rectangle
	}{
		{"decorated, 96 DPI", image.Rect(92, 19, 908, 658), image.Rect(99, 19, 901, 651)},
		{"decorated, 144 DPI", image.Rect(88, 5, 912, 662), image.Rect(99, 5, 901, 651)},
		{"frameless", client, client},
		{"TitleBarOverlay: no caption", image.Rect(92, 50, 908, 658), image.Rect(99, 50, 901, 651)},
	} {
		if got := visibleFrame(tt.outer, client); got != tt.want {
			t.Errorf("%s: visibleFrame = %v, want %v", tt.name, got, tt.want)
		}
	}
}

// fitVisible lets the visible frame touch a work area's edge, where
// fitting the outer rectangle kept the window 7 pixels further in.
func TestFitVisible(t *testing.T) {
	works := []image.Rectangle{image.Rect(0, 0, 1920, 1040)}
	// A decorated window at 96 DPI whose outer rectangle starts at p.
	at := func(p image.Point) (outer, client image.Rectangle) {
		return image.Rect(0, 0, 816, 639).Add(p), image.Rect(8, 31, 808, 631).Add(p)
	}
	for _, tt := range []struct {
		name string
		at   image.Point
		want image.Point
	}{
		{"on screen", image.Pt(100, 100), image.Point{}},
		{"visible frame at the left edge", image.Pt(-7, 100), image.Point{}},
		{"visible frame at the bottom edge", image.Pt(100, 1040-632), image.Point{}},
		{"visible frame at the right edge", image.Pt(1920-809, 100), image.Point{}},
		{"off the right edge, gone monitor", image.Pt(3000, 100), image.Pt(1920-809-3000, 0)},
		{"above the work area", image.Pt(100, -20), image.Pt(0, 20)},
		{"below the work area", image.Pt(100, 1500), image.Pt(0, 1040-632-1500)},
	} {
		outer, client := at(tt.at)
		if got := fitVisible(outer, client, works); got != tt.want {
			t.Errorf("%s: fitVisible = %v, want %v", tt.name, got, tt.want)
		}
	}
}
