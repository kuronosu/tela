package core

import (
	"testing"

	"github.com/kuronosu/tela/internal/layout"
	"github.com/kuronosu/tela/internal/style"
)

// The cases of ADR-157 written with classes. The expected boxes are
// Chromium 154's for the same elements with Tailwind's CSS (measured with
// headless Chromium on the equivalent inline styles; the layout package's
// testdata/cases flex_percent_zero, flex_aspect_ratio_min_size and
// flex_fit_content_cross hold them as fixtures).

// TestZeroPercentMapping checks that a zero percentage reaches layout as a
// percentage where that matters (sizes, max sizes, insets, flex-basis,
// grid tracks) and as 0px where CSS gives 0% the meaning of 0 (min sizes,
// margins).
func TestZeroPercentMapping(t *testing.T) {
	zeroPct := style.DimPct(0)
	calc := style.DimLP(style.LP{Px: 10}, true)
	for _, tt := range []struct {
		name string
		got  layout.Dim
		want layout.Dim
	}{
		{"size 0%", dimOf(zeroPct), layout.Percent(0)},
		{"size 0px", dimOf(style.DimPx(0)), layout.Px(0)},
		{"size calc(0% + 10px)", dimOf(calc), layout.Dim{Unit: layout.UnitCalc, Px: 10}},
		{"size 50%", dimOf(style.DimPct(50)), layout.Percent(0.5)},
		{"min size 0%", zeroDimOf(zeroPct), layout.Px(0)},
		{"min size calc(0% + 10px)", zeroDimOf(calc), layout.Px(10)},
		{"min size 50%", zeroDimOf(style.DimPct(50)), layout.Percent(0.5)},
		{"track 0%", breadth(style.TrackBreadth{Kind: style.BreadthLength, ZeroPct: true}), layout.Percent(0)},
		{"track 0px", breadth(style.TrackBreadth{Kind: style.BreadthLength}), layout.Px(0)},
		{"track fit-content(0%)", breadth(style.TrackBreadth{Kind: style.BreadthFitContent, ZeroPct: true}),
			layout.Dim{Unit: layout.UnitFitContentPercent}},
		{"track fit-content(0px)", breadth(style.TrackBreadth{Kind: style.BreadthFitContent}), layout.FitContentPx(0)},
	} {
		if tt.got != tt.want {
			t.Errorf("%s: %+v, want %+v", tt.name, tt.got, tt.want)
		}
	}
}

// TestZeroPercentFlexBasis is the reported case: Tailwind's flex-1 is
// flex: 1 1 0%, and a percentage basis of a column of auto height is
// content-based, so the scrolling item is as tall as its content (88 px:
// 80 and the padding); basis-0 is 0px, a length, and leaves the padding.
func TestZeroPercentFlexBasis(t *testing.T) {
	for _, tt := range []struct {
		class string
		h     float32
	}{
		{"flex-1", 88},
		{"basis-[0%] grow", 88},
		{"flex-[1_1_0%]", 88},
		{"basis-[calc(0%_+_10px)] grow", 88},
		{"basis-1/2 grow", 88},
		{"flex-auto", 88},
		{"basis-0 grow", 8},
		{"flex-[1_1_0px]", 8},
	} {
		t.Run(tt.class, func(t *testing.T) {
			root := div(Key("col"), Class("flex flex-col max-h-[300px] w-[200px]"),
				div(Key("item"), Class(tt.class+" overflow-y-auto p-1"), div(Class("h-20"))))
			tr := newTestTree(t, 800, 600, root, nil)
			tr.Frame()
			wantRect(t, mustFind(t, tr, "item"), 0, 0, 200, tt.h)
			wantRect(t, mustFind(t, tr, "col"), 0, 0, 200, tt.h)
		})
	}
}

// TestZeroPercentOfIndefiniteBasis: a height, max height or inset of 0%
// against a parent of auto height is auto or none; a min height of 0% or
// 50% there counts as 0 (and lets the item shrink in a column); a 0% grid
// row of an auto-height grid is auto while the grid is sized.
func TestZeroPercentOfIndefiniteBasis(t *testing.T) {
	root := div(Key("root"), Class("w-[400px]"),
		div(Key("h"), Class("w-[100px]"), div(Class("h-[0%]"), div(Class("h-[30px]")))),
		div(Key("maxh"), Class("w-[100px]"), div(Class("max-h-[0%]"), div(Class("h-[30px]")))),
		div(Class("w-[100px]"), div(Key("top"), Class("relative top-[0%] bottom-[10px] h-[20px]"))),
		div(Class("flex flex-col max-h-[50px] w-[100px]"), div(Key("minh0"), Class("min-h-[0%]"), div(Class("h-[100px]")))),
		div(Class("flex flex-col max-h-[50px] w-[100px]"), div(Key("minh50"), Class("min-h-[50%]"), div(Class("h-[100px]")))),
		div(Key("grid"), Class("grid grid-rows-[0%_auto] w-[100px]"), div(Class("h-[30px]")), div(Key("row2"), Class("h-[20px]"))),
		div(Key("fit"), Class("w-fit"), div(Key("w0"), Class("w-[0%]"), div(Class("w-[50px] h-[10px]")))),
	)
	tr := newTestTree(t, 800, 600, root, nil)
	tr.Frame()
	wantRect(t, mustFind(t, tr, "h"), 0, 0, 100, 30)
	wantRect(t, mustFind(t, tr, "maxh"), 0, 30, 100, 30)
	wantRect(t, mustFind(t, tr, "top"), 0, 60-10, 100, 20)
	wantRect(t, mustFind(t, tr, "minh0"), 0, 80, 100, 50)
	wantRect(t, mustFind(t, tr, "minh50"), 0, 130, 100, 50)
	wantRect(t, mustFind(t, tr, "grid"), 0, 180, 100, 50)
	wantRect(t, mustFind(t, tr, "row2"), 0, 180, 100, 20)
	wantRect(t, mustFind(t, tr, "fit"), 0, 230, 50, 10)
	wantRect(t, mustFind(t, tr, "w0"), 0, 230, 0, 10)
}

// TestAspectRatioFlexItemsShrink: a flex item with an aspect ratio and a
// definite main size has the content's automatic minimum (a cross size the
// ratio transfers from its own main size is not definite), so a row of
// w-full aspect-square items shares the row and the items stay square.
func TestAspectRatioFlexItemsShrink(t *testing.T) {
	row := func(key, rowClass, itemClass string, n int) Node {
		kids := []Child{Class("flex " + rowClass)}
		for i := 0; i < n; i++ {
			c := []Child{Class(itemClass)}
			if i == 0 {
				c = append(c, Key(key))
			}
			kids = append(kids, div(c...))
		}
		return div(Class("w-[280px]"), div(kids...))
	}
	root := div(
		row("full", "", "w-full aspect-square", 7),
		row("px", "", "w-[280px] aspect-square", 2),
		row("minw", "", "w-full aspect-square min-w-[50px]", 7),
		row("minh", "", "w-full aspect-square min-h-[60px]", 7),
		row("cross", "h-[50px]", "w-full aspect-square", 2),
		row("hpx", "", "w-full h-[100px] aspect-square", 2),
		row("max", "", "w-[100px] max-w-[50px] aspect-square", 1),
	)
	tr := newTestTree(t, 800, 600, root, nil)
	tr.Frame()
	wantRect(t, mustFind(t, tr, "full"), 0, 0, 40, 40)
	wantRect(t, mustFind(t, tr, "px"), 0, 40, 140, 140)
	wantRect(t, mustFind(t, tr, "minw"), 0, 180, 50, 50)
	wantRect(t, mustFind(t, tr, "minh"), 0, 230, 60, 60)
	wantRect(t, mustFind(t, tr, "cross"), 0, 290, 140, 50)
	wantRect(t, mustFind(t, tr, "hpx"), 0, 340, 140, 100)
	wantRect(t, mustFind(t, tr, "max"), 0, 440, 50, 50)
}

// TestFitContentItemInStretchingColumn: a w-fit (or w-min, w-max) item of
// a column is measured at its own width, not at the column's: a box of
// aspect-square cells is as tall in a stretching column as in a block
// (88x56 here, where it used to be 88x412).
func TestFitContentItemInStretchingColumn(t *testing.T) {
	item := func(key, width string) Node {
		cell := func() Node { return div(Class("flex-1"), div(Class("w-full aspect-square min-w-8"))) }
		return div(Key(key), Class(width+" p-3"),
			div(Class("flex"), div(Class("w-8")), div(Class("w-8"))),
			div(Class("flex"), cell(), cell()))
	}
	for _, tt := range []struct{ name, container, width string }{
		{"column", "flex flex-col", "w-fit"},
		{"column min-content", "flex flex-col", "w-min"},
		{"column max-content", "flex flex-col", "w-max"},
		{"column items-start", "flex flex-col items-start", "w-fit"},
		{"block", "", "w-fit"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			root := div(Key("c"), Class(tt.container), item("fit", tt.width), div(Key("after"), Class("h-[10px] w-full")))
			tr := newTestTree(t, 800, 600, root, nil)
			tr.Frame()
			wantRect(t, mustFind(t, tr, "fit"), 0, 0, 88, 56)
			wantRect(t, mustFind(t, tr, "after"), 0, 56, 800, 10)
		})
	}
}
