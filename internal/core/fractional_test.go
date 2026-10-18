package core

import (
	"fmt"
	"testing"
)

// TestScrollShiftsPixelsFractional is TestScrollShiftsPixels at fractional
// scales: a scroll by whole device pixels shifts the framebuffer even
// though the offset is not a whole number of logical pixels.
func TestScrollShiftsPixelsFractional(t *testing.T) {
	for _, scale := range []float64{1.25, 1.5, 1.75} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			st := &sceneState{refs: make([]ElementRef, 1), setters: make([]Setter[int], 200)}
			x := newDamageHarness(t, rowScene(st, 120, "grow overflow-y-auto bg-white"), 360, 280, scale, Config{InstantScroll: true}, 1)
			st.refs[0].ScrollBy(0, 40)
			x.frame("first scroll")
			for i := range 5 {
				st.refs[0].ScrollBy(0, 53)
				x.frame(fmt.Sprintf("scroll %d", i))
				d := x.tree().LastDamage()
				if d.Full || len(d.Shifts) != 1 {
					t.Fatalf("scroll %d: damage full=%v shifts=%d rects=%d, want a shift", i, d.Full, len(d.Shifts), len(d.Rects))
				}
				area := 0
				for _, r := range d.Rects {
					area += r.Dx() * r.Dy()
				}
				fb := x.w.Metrics().Physical()
				if full := fb.X * fb.Y; area > full/2 {
					t.Errorf("scroll %d: %d px rasterized of %d: %+v", i, area, full, d)
				}
			}
		})
	}
}
