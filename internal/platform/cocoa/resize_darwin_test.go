//go:build darwin

package cocoa

import (
	"image"
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/darwin"
	"github.com/kuronosu/tela/internal/platform"
)

// captionClicks counts the CaptionClickEvents of a step.
func captionClicks(evs []ctlEvent) int {
	n := 0
	for _, e := range evs {
		if _, ok := e.e.(platform.CaptionClickEvent); ok {
			n++
		}
	}
	return n
}

// TestResizeHandles (ADR-257): a press on one of the app's resize handles
// is not delivered and starts the backend's own resize loop; its drags
// set the frame from the cursor's motion (the real cursor does not move
// with synthetic events, so the size stays), and its release ends it. The
// handle shows the resize cursor. A FixedSize window has no handles.
func TestResizeHandles(t *testing.T) {
	screenInfo(t)
	pt := func(x, y float32) platform.Point { return platform.Point{X: x, Y: y} }
	const (
		lDown, lUp, lDragged, moved = darwin.NSEventTypeLeftMouseDown, darwin.NSEventTypeLeftMouseUp, darwin.NSEventTypeLeftMouseDragged, darwin.NSEventTypeMouseMoved
	)
	ctlRun(t, []platform.WindowOptions{
		{Title: "tela resize", Size: platform.Size{W: 300, H: 200}, Frameless: true, Transparent: true, HasPosition: true, Position: image.Pt(150, 150)},
		{Title: "tela fixed", Size: platform.Size{W: 300, H: 200}, Frameless: true, FixedSize: true, HasPosition: true, Position: image.Pt(500, 150)},
	}, nil,
		ctlStep{name: "a press on a resize handle",
			do: func(c *ctl) {
				for _, w := range c.w {
					w.SetHitRegions([]platform.HitRegion{
						{Kind: platform.HitResizeLeft, Rect: platform.Rect{X: 0, Y: 0, W: 5, H: 200}},
						{Kind: platform.HitResizeBottomRight, Rect: platform.Rect{X: 284, Y: 195, W: 16, H: 5}},
					})
					w.ptr = pointerState{}
				}
				w := c.w[0]
				if w.handleEdge(pt(2, 100)) != platform.EdgeLeft || c.w[1].handleEdge(pt(2, 100)) != 0 {
					t.Errorf("handle edges %v and %v (FixedSize), want left and none", w.handleEdge(pt(2, 100)), c.w[1].handleEdge(pt(2, 100)))
				}
				darwin.Send(w.view, darwin.Sel("mouseMoved:"), c.mouse(0, moved, pt(2, 100), 0, 0))
				if w.ctl.resize.hover != platform.EdgeLeft || w.shownCursor() != platform.CursorWResize {
					t.Errorf("hover %v, shown cursor %v; want left and w-resize", w.ctl.resize.hover, w.shownCursor())
				}
				c.send(c.mouse(0, lDown, pt(2, 100), 20, 1))
				if !w.ctl.resize.held || w.ctl.resize.edge != platform.EdgeLeft {
					t.Errorf("resize state %+v after the press", w.ctl.resize)
				}
				c.send(c.mouse(0, lDragged, pt(10, 100), 40, 1))
				c.send(c.mouse(0, lUp, pt(10, 100), 60, 1))
				if w.ctl.resize.held {
					t.Error("the release did not end the resize")
				}
				d := c.w[1]
				darwin.Send(d.view, darwin.Sel("mouseMoved:"), c.mouse(1, moved, pt(2, 100), 80, 0))
				c.send(c.mouse(1, lDown, pt(2, 100), 100, 1))
				c.send(c.mouse(1, lUp, pt(2, 100), 120, 1))
			},
			until: func(c *ctl) bool { return c.after(300 * time.Millisecond) },
			check: func(c *ctl) {
				for _, e := range c.stepEvents(0) {
					if pe, ok := e.e.(platform.PointerEvent); ok && (pe.Action == platform.PointerDown || pe.Action == platform.PointerUp) {
						t.Errorf("a resize press delivered %s", pointerString(pe))
					}
				}
				var fixed int
				for _, e := range c.stepEvents(1) {
					if pe, ok := e.e.(platform.PointerEvent); ok && (pe.Action == platform.PointerDown || pe.Action == platform.PointerUp) {
						fixed++
					}
				}
				if fixed != 2 {
					t.Errorf("the FixedSize window got %d presses and releases, want 2 (no handles)", fixed)
				}
				t.Logf("frame after the resize: %v", c.w[0].frameOf())
			}},
	)
}
