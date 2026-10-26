package core

import (
	"fmt"
	"image"
	"testing"
)

// hoistedCellsScene is a scroll container holding a grid of positioned
// cells with text (the days of calendars: kit's Day cell is `relative`),
// so every cell is a hoisted participant of the root stacking context,
// painted in a chunk of its own after the container's content and its
// scrollbars.
func hoistedCellsScene(ref *ElementRef, rows int) Node {
	comp := func() Node {
		*ref = UseElementRef()
		weeks := make([]Child, rows)
		for w := range weeks {
			days := make([]Child, 7)
			for d := range days {
				n := w*7 + d
				cls := "relative flex size-8 items-center justify-center text-sm"
				if n%9 == 0 {
					cls += " rounded-md bg-slate-100"
				}
				days[d] = div(Key(d), Class(cls), Textf("%d", n%31+1))
			}
			weeks[w] = div(Key(w), Class("flex gap-1 rounded bg-slate-50"), Fragment(days...))
		}
		return div(Class("flex h-screen flex-col bg-white"),
			div(Class("h-10 shrink-0 bg-slate-800 px-3 text-white"), Text("Header")),
			div(AttachRef(*ref), Class("grow overflow-y-auto bg-white"),
				div(Class("flex flex-col gap-1 p-4"), div(Class("text-sm font-medium"), Text("October 2026")), Fragment(weeks...))))
	}
	return Component(comp)
}

// TestScrollShiftsHoistedCells checks that scrolling a container whose
// content is hundreds of positioned cells shifts their pixels with the
// content (they continue the shifted group across the scrollbars painted
// between them, ADR-161) instead of damaging every cell at its old and
// new position: a few rectangles, a small rasterized area, and the pixels
// of a full rendering.
func TestScrollShiftsHoistedCells(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			var ref ElementRef
			x := newDamageHarness(t, hoistedCellsScene(&ref, 60), 360, 420, scale, Config{InstantScroll: true}, 1)
			ref.ScrollBy(0, 24)
			x.frame("first scroll")
			for i := range 6 {
				ref.ScrollBy(0, 21)
				x.frame(fmt.Sprintf("scroll %d", i))
				d := x.tree().LastDamage()
				if d.Full || len(d.Shifts) != 1 {
					t.Fatalf("scroll %d: damage %+v, want one shift", i, d)
				}
				area := 0
				for _, r := range d.Rects {
					area += r.Dx() * r.Dy()
				}
				// The exposed strip (21 rows of logical px), the scrollbar
				// and its fade, the anti-aliased edges of the scrollport.
				full := int(360*scale) * int(420*scale)
				if area > full/6 {
					t.Errorf("scroll %d: %d device px rasterized of %d (%d rectangles): the cells were damaged instead of shifted", i, area, full, len(d.Rects))
				}
			}
		})
	}
}

// TestMergeRectsManyScattered checks that thousands of scattered damage
// rectangles (glyphs of cells far apart, which no pair merges cheaply)
// are merged into at most maxDamageRects rectangles that cover them all,
// through the grid of clusterRects (ADR-161: merged pair by pair, they
// took hundreds of milliseconds per frame).
func TestMergeRectsManyScattered(t *testing.T) {
	var in []image.Rectangle
	for y := range 60 {
		for x := range 40 {
			in = append(in, image.Rect(x*32+12, y*32+10, x*32+20, y*32+22))
		}
	}
	out := mergeRects(in)
	if len(out) == 0 || len(out) > maxDamageRects {
		t.Fatalf("%d rectangles, want 1 to %d", len(out), maxDamageRects)
	}
	for _, r := range in {
		covered := false
		for _, o := range out {
			if r.In(o) {
				covered = true
				break
			}
		}
		if !covered {
			t.Fatalf("%v is in no merged rectangle", r)
		}
	}
	// Below the threshold the pairwise merge alone runs, as before.
	small := []image.Rectangle{image.Rect(0, 0, 10, 10), image.Rect(10, 0, 20, 10), image.Rect(200, 200, 210, 210)}
	if got := mergeRects(small); len(got) != 2 || got[0] != image.Rect(0, 0, 20, 10) || got[1] != small[2] {
		t.Errorf("small input merged to %v", got)
	}
}
