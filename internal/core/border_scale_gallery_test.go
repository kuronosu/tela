package core

import (
	"math"
	"slices"
	"testing"
)

// The border-scale gallery compares border, outline and outline-offset
// widths at fractional display scales with Chromium (ADR-273): Blink
// snaps them "as a border width" when it computes the style, at the
// device scale factor (a width of at least one device pixel floored to
// whole device pixels, a smaller one made one device pixel), so at x1.5 a
// 1px border is 0.667 px wide in layout, and the boxes inside it and
// after it move. Measured in Chromium 155 at --force-device-scale-factor
// 1.25 and 1.5; the comparison at 1 and 2 is the other galleries'.
var borderScaleGallery = &cssGallery{
	name:   "borderscale",
	width:  400,
	scales: []float32{1.25, 1.5},
	// Text anti-aliasing, and inline fragments whose edges round to
	// another device pixel: Chromium's text widths are in 1/64 px, and
	// summed along a row they move edges by up to 0.04 px. (Before
	// gfx.RoundDevice, boxes and baselines that Chromium places on exact
	// half device pixels and Tela a float32 hair below them were one pixel
	// apart: inline x1.25 4.60, nested x1.5 1.58.)
	mean: [2]float64{2, 2},
	geom: [2]float64{0.05, 0.05},
	rows: []cssRow{
		{name: "widths", height: 64, class: "gap-2", kids: []inode{
			iel("div", "w-14 h-12 border border-zinc-900 p-2 text-sm", itext("1")),
			iel("div", "w-14 h-12 border-2 border-zinc-900 p-2 text-sm", itext("2")),
			iel("div", "w-14 h-12 border-[3px] border-zinc-900 p-2 text-sm", itext("3")),
			iel("div", "w-14 h-12 border-[0.5px] border-zinc-900 p-2 text-sm", itext(".5")),
			iel("div", "w-14 h-12 border-[1.5px] border-zinc-900 p-2 text-sm", itext("1.5")),
			iel("div", "w-14 h-12 border-[2.5px] border-zinc-900 p-2 text-sm", itext("2.5")),
		}},
		{name: "nested", height: 56, kids: []inode{
			iel("div", "border border-zinc-700 p-[3px]",
				iel("div", "border border-sky-600 p-[3px]",
					iel("div", "border border-rose-500 px-1 text-sm", itext("deep")))),
			iel("div", "ml-3 border-2 border-zinc-700 p-[3px]",
				iel("div", "border border-sky-600 p-[3px]",
					iel("div", "border-[1.5px] border-rose-500 px-1 text-sm", itext("mixed")))),
		}},
		{name: "chain", height: 44, kids: []inode{
			iel("div", "shrink-0 border border-zinc-400 px-2 py-1 text-sm", itext("one")),
			iel("div", "shrink-0 border border-zinc-400 px-2 py-1 text-sm", itext("two")),
			iel("div", "shrink-0 border border-zinc-400 px-2 py-1 text-sm", itext("three")),
			iel("div", "shrink-0 border border-zinc-400 px-2 py-1 text-sm", itext("four")),
			iel("div", "shrink-0 border border-zinc-400 px-2 py-1 text-sm", itext("five")),
			iel("div", "shrink-0 border border-zinc-400 px-2 py-1 text-sm", itext("six")),
		}},
		{name: "sides", height: 56, class: "gap-3", kids: []inode{
			iel("div", "w-24 h-10 border-t border-l-2 border-r-[3px] border-b-4 border-zinc-800 text-xs", itext("t l r b")),
			iel("div", "w-24 h-10 border-x border-y-2 border-emerald-600 text-xs", itext("x y")),
			iel("div", "h-10 border-s border-e-[1.5px] border-violet-600 px-2 text-xs", itext("s e")),
		}},
		{name: "inline", height: 56, kids: []inode{
			iel("p", "w-[360px] text-sm",
				itext("Text with "),
				iel("span", "border border-amber-500 px-1", itext("one")),
				itext(" and "),
				iel("span", "border-2 border-sky-500 px-0.5", itext("two "), iel("span", "border border-rose-500", itext("nested"))),
				itext(" inline borders."),
			),
		}},
		{name: "outline", height: 64, class: "gap-5", kids: []inode{
			iel("div", "size-10 bg-zinc-100 outline outline-sky-600"),
			iel("div", "size-10 bg-zinc-100 outline outline-offset-1 outline-sky-600"),
			iel("div", "size-10 bg-zinc-100 outline-2 outline-offset-2 outline-sky-600"),
			iel("div", "size-10 bg-zinc-100 outline outline-offset-[1.5px] outline-sky-600"),
			iel("div", "size-10 bg-zinc-100 outline -outline-offset-1 outline-sky-600"),
			iel("div", "size-10 rounded-md bg-zinc-100 outline outline-offset-1 outline-sky-600"),
		}},
		{name: "field", height: 56, class: "gap-3", kids: []inode{
			iel("div", "flex h-9 w-48 items-center rounded-md border border-zinc-300 px-3 py-1 text-sm shadow-xs", itext("An input-like box")),
			iel("div", "rounded-md border border-zinc-300 px-4 py-2 text-sm font-medium", itext("Button")),
		}},
	},
}

// TestChromiumBorderScale compares the border-scale gallery with Chromium
// at x1.25 and x1.5.
func TestChromiumBorderScale(t *testing.T) { borderScaleGallery.compare(t) }

// TestBorderScaleGallery is the gallery's golden, at x1.25 and x1.5.
func TestBorderScaleGallery(t *testing.T) { borderScaleGallery.golden(t) }

// TestBorderScaleRescale checks that a tree whose scale changes lays its
// borders out again: after a frame at one scale and SetScale, every box,
// inline fragment and pixel is what a tree made at the new scale gives
// (bordered boxes are mapped again, paragraphs with bordered inline
// elements assembled again).
func TestBorderScaleRescale(t *testing.T) {
	for _, c := range [][2]float32{{1, 1.5}, {1.5, 1.25}, {1.25, 2}} {
		want, wantRects := borderScaleGallery.render(t, c[1])
		got, gotRects := borderScaleGallery.renderFrom(t, c[0], c[1])
		for k := range wantRects {
			if !slices.Equal(gotRects[k], wantRects[k]) {
				t.Errorf("x%g -> x%g: element %d: %v, want %v", c[0], c[1], k, gotRects[k], wantRects[k])
			}
		}
		if !slices.Equal(got.Pix, want.Pix) {
			t.Errorf("x%g -> x%g: pixels differ from a tree made at x%g", c[0], c[1], c[1])
		}
	}
}

// TestSnapLineWidth checks the snapping of border widths "as a border
// width" against Chromium 155's computed values (getComputedStyle of
// border-top-width and outline-offset at --force-device-scale-factor 1,
// 1.25, 1.5, 1.75 and 2).
func TestSnapLineWidth(t *testing.T) {
	for _, c := range []struct {
		w    float32
		k    float64
		want float64 // CSS px
	}{
		{0, 1.5, 0}, {0.25, 1, 1}, {0.5, 1, 1}, {1.5, 1, 1}, {2.5, 1, 2},
		{0.25, 1.25, 0.8}, {1, 1.25, 0.8}, {1.5, 1.25, 0.8}, {2, 1.25, 1.6}, {3, 1.25, 2.4}, {4, 1.25, 4},
		{0.5, 1.5, 2.0 / 3}, {1, 1.5, 2.0 / 3}, {1.5, 1.5, 4.0 / 3}, {2, 1.5, 2}, {3, 1.5, 8.0 / 3}, {4, 1.5, 4},
		{1, 1.75, 1 / 1.75}, {1.5, 1.75, 2 / 1.75}, {2, 1.75, 3 / 1.75}, {3, 1.75, 5 / 1.75}, {4, 1.75, 7 / 1.75},
		{0.3, 2, 0.5}, {1, 2, 1}, {1.5, 2, 1.5}, {2.5, 2, 2.5},
		{1.3, 1, 1}, // 0.1em of 13px
	} {
		if got := snapLineWidth(c.w, c.k); math.Abs(float64(got)-c.want) > 1e-6 {
			t.Errorf("snapLineWidth(%g, x%g) = %g, want %g", c.w, c.k, got, c.want)
		}
		// Snapping a snapped width keeps it.
		if got := snapLineWidth(snapLineWidth(c.w, c.k), c.k); math.Abs(float64(got)-c.want) > 1e-6 {
			t.Errorf("snapLineWidth twice (%g, x%g) = %g, want %g", c.w, c.k, got, c.want)
		}
	}
	for _, c := range []struct {
		o    float32
		k    float64
		want float64
	}{
		{0, 1.5, 0}, {0.3, 1.5, 2.0 / 3}, {1, 1.5, 2.0 / 3}, {1.5, 1.5, 4.0 / 3}, {2.5, 1.5, 2},
		{-0.3, 1.5, -2.0 / 3}, {-1, 1.5, -2.0 / 3}, {-1.5, 1.5, -4.0 / 3}, {-2, 1.5, -2},
		{2.5, 1, 2}, {-1.5, 1, -1}, {2.5, 2, 2.5},
	} {
		if got := snapLineOffset(c.o, c.k); math.Abs(float64(got)-c.want) > 1e-6 {
			t.Errorf("snapLineOffset(%g, x%g) = %g, want %g", c.o, c.k, got, c.want)
		}
	}
}
