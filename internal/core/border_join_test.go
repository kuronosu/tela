package core

import (
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// The border-join gallery: boxes whose adjacent borders differ in color
// or width, down to the CSS triangle (zero-sized boxes whose borders
// meet in mitered joins, some of them transparent), with and without
// radius, compared with Chromium by cssGallery.

func shapes(classes ...string) []inode {
	out := make([]inode, len(classes))
	for i, c := range classes {
		out[i] = iel("div", c)
	}
	return out
}

var borderJoinGallery = &cssGallery{
	name:  "borderjoin",
	width: 520,
	rows: []cssRow{
		{name: "triangle-up", height: 64, class: "gap-6", kids: shapes(
			"w-0 h-0 border-x-8 border-x-transparent border-b-8 border-b-slate-900",
			"w-0 h-0 border-x-[20px] border-x-transparent border-b-[34px] border-b-indigo-600",
			"w-0 h-0 border-x-[7px] border-x-transparent border-b-[12px] border-b-rose-500",
			"h-0 w-0 border-x-[15px] border-b-[26px] border-x-transparent border-b-emerald-500",
		)},
		{name: "triangle-dirs", height: 64, class: "gap-6", kids: shapes(
			"w-0 h-0 border-x-[20px] border-x-transparent border-t-[34px] border-t-indigo-600",
			"w-0 h-0 border-y-[20px] border-y-transparent border-l-[34px] border-l-rose-600",
			"w-0 h-0 border-y-[20px] border-y-transparent border-r-[34px] border-r-amber-500",
			"w-0 h-0 border-[16px] border-transparent border-b-sky-600",
			"w-0 h-0 border-[16px] border-transparent border-l-violet-600",
		)},
		{name: "pinwheel", height: 64, class: "gap-6", kids: shapes(
			"w-0 h-0 border-[20px] border-t-red-500 border-r-green-500 border-b-blue-500 border-l-yellow-500",
			"size-10 border-t-[6px] border-r-[12px] border-b-[18px] border-l-[3px] border-t-red-500 border-r-green-500 border-b-blue-500 border-l-yellow-500",
			"size-10 border-[10px] border-t-slate-900 border-r-slate-400 border-b-slate-900 border-l-slate-400",
			"w-0 h-0 border-y-[20px] border-y-red-500 border-l-[30px] border-l-blue-500",
		)},
		{name: "rounded", height: 64, class: "gap-6", kids: shapes(
			"w-0 h-0 border-x-[20px] border-x-transparent border-b-[34px] border-b-indigo-600 rounded-lg",
			"w-0 h-0 border-x-[20px] border-x-transparent border-b-[34px] border-b-indigo-600 rounded-full",
			"w-0 h-0 border-y-[20px] border-y-transparent border-l-[34px] border-l-rose-600 rounded-md",
			"w-0 h-0 border-[20px] border-t-red-500 border-r-green-500 border-b-blue-500 border-l-yellow-500 rounded-full",
		)},
		{name: "rounded-box", height: 64, class: "gap-6", kids: shapes(
			"size-10 rounded-xl border-[6px] border-t-red-500 border-r-green-500 border-b-blue-500 border-l-yellow-500",
			"size-10 rounded-full border-[6px] border-x-sky-500 border-y-slate-800",
			"size-10 rounded-lg border-t-[10px] border-b-[3px] border-t-fuchsia-600 border-b-amber-400",
			"size-10 rounded-2xl border-l-[8px] border-l-emerald-600 border-t-[2px] border-t-slate-300 bg-slate-100",
		)},
		{name: "alpha", height: 64, class: "gap-6", kids: shapes(
			"w-0 h-0 border-x-[20px] border-x-transparent border-b-[34px] border-b-indigo-600/50",
			"size-10 border-[8px] border-t-red-500/50 border-r-green-500/50 border-b-blue-500/50 border-l-yellow-500/50 bg-slate-200",
			"w-0 h-0 border-[16px] border-slate-900/20 border-b-slate-900",
			"w-0 h-0 border-x-[20px] border-x-white border-b-[34px] border-b-slate-900 bg-red-500",
		)},
		{name: "bubble", height: 72, class: "gap-10", kids: []inode{
			iel("div", "relative rounded-lg bg-slate-900 px-3 py-2 text-xs text-white",
				itext("Tooltip"),
				iel("div", "absolute top-full left-1/2 h-0 w-0 -translate-x-1/2 border-x-[6px] border-t-[6px] border-x-transparent border-t-slate-900")),
			iel("div", "relative size-10 rounded-md bg-indigo-600",
				iel("div", "absolute -right-[10px] top-3 h-0 w-0 border-y-[7px] border-l-[10px] border-y-transparent border-l-indigo-600")),
		}},
	},
	// Text is anti-aliased differently (the tooltip's label, up to 132
	// levels); everything else is within 63 levels of Chromium's pixels
	// (the anti-aliased seams of the joins).
	mean: [2]float64{0.3, 0.3},
	maxd: [2]int{140, 140},
	geom: [2]float64{0.02, 0.02},
}

func TestChromiumBorderJoin(t *testing.T) { borderJoinGallery.compare(t) }

func TestBorderJoinGallery(t *testing.T) { borderJoinGallery.golden(t) }

// TestSidePolygonStopsAtTheInnerEdge: with widths that differ a lot the
// wedge between the two diagonals of a side reaches past the inner edge
// (here the bottom side's, which would paint over the top side).
func TestSidePolygonStopsAtTheInnerEdge(t *testing.T) {
	o := gfx.Rect{X0: 0, Y0: 0, X1: 40, Y1: 40}
	bw := [4]float32{6, 12, 18, 3} // top, right, bottom, left
	path := sidePolygon(o, gfx.RRect{}, bw, 2)
	minY := float32(1e9)
	for el := range path.Elements() {
		for _, p := range el.Pts[:el.Verb.NumPoints()] {
			minY = min(minY, p.Y)
		}
	}
	if minY < 40-18-0.001 {
		t.Errorf("the bottom side reaches y = %g, above its inner edge at 22", minY)
	}
	// A rounded inner corner moves the bound inwards by its radius.
	inner := gfx.RRect{Rect: gfx.Rect{X0: 3, Y0: 6, X1: 28, Y1: 22},
		Radii: [4]gfx.Point{{X: 4, Y: 4}, {X: 4, Y: 4}, {X: 4, Y: 5}, {X: 4, Y: 5}}}
	path = sidePolygon(o, inner, bw, 2)
	minY = 1e9
	for el := range path.Elements() {
		for _, p := range el.Pts[:el.Verb.NumPoints()] {
			minY = min(minY, p.Y)
		}
	}
	if minY < 22-5-0.001 || minY > 22-5+0.001 {
		t.Errorf("the bottom side reaches y = %g, want 17", minY)
	}
}
