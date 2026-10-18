package core

import "testing"

// The grid gallery: CSS grid layouts written with Tailwind classes (a
// dashboard shell with a sidebar column and a card grid, named lines,
// spans and dense packing, intrinsic tracks with text, alignment, grids
// inside flex and paragraphs, absolutely positioned items in grid areas),
// compared with Chromium (cssGallery).

func gridCells(n int, class string) []inode {
	colors := []string{"bg-indigo-300", "bg-indigo-400", "bg-indigo-500", "bg-indigo-600"}
	out := make([]inode, n)
	for i := range out {
		out[i] = iel("div", colors[i%len(colors)]+" "+class)
	}
	return out
}

var gridGallery = &cssGallery{
	name:  "grid",
	width: 520,
	rows: []cssRow{
		// The dashboard shell: a header and a footer across both columns,
		// a 160px sidebar and a main area holding a card grid of
		// repeat(auto-fill, minmax(60px, 1fr)) columns.
		{name: "dashboard", height: 176, kids: []inode{
			iel("div", "grid h-[160px] w-[480px] grid-cols-[160px_1fr] grid-rows-[auto_1fr_auto] gap-2 bg-slate-100",
				iel("div", "col-span-2 h-8 bg-slate-800"),
				iel("div", "flex flex-col gap-1 bg-slate-200 p-2",
					iel("div", "h-3 w-24 rounded bg-slate-400"),
					iel("div", "h-3 w-16 rounded bg-slate-400")),
				iel("div", "grid grid-cols-[repeat(auto-fill,minmax(60px,1fr))] content-start gap-2 p-2",
					gridCells(5, "h-8 rounded-md")...),
				iel("div", "col-span-2 h-6 bg-slate-300")),
		}},
		// Named lines: full-bleed and content columns.
		{name: "named-lines", height: 88, kids: []inode{
			iel("div", "grid w-[300px] grid-cols-[[full-start]_24px_[content-start]_1fr_[content-end]_24px_[full-end]] gap-y-1 bg-slate-100",
				iel("div", "col-[content] h-4 bg-emerald-300"),
				iel("div", "col-[full] h-4 bg-emerald-400"),
				iel("div", "col-start-[content] col-end-[full-end] h-4 bg-emerald-500"),
				iel("div", "col-[full-start/content-end] h-4 bg-emerald-600")),
			iel("div", "ml-4 grid w-[150px] grid-cols-[repeat(3,[col]_40px)] gap-x-1",
				iel("div", "col-[col_2/span_col_2] h-4 bg-teal-400"),
				iel("div", "col-[col_-1] h-4 bg-teal-600"),
				iel("div", "col-span-[col] h-4 bg-teal-300")),
		}},
		// Spans, a negative line, row spans and dense packing.
		{name: "spans", height: 96, kids: []inode{
			iel("div", "grid w-[200px] grid-cols-4 auto-rows-[18px] gap-1",
				iel("div", "col-span-2 bg-amber-300"),
				iel("div", "col-span-3 bg-amber-400"),
				iel("div", "row-span-2 bg-amber-500"),
				iel("div", "-col-start-1 bg-amber-600"),
				iel("div", "bg-amber-300")),
			iel("div", "ml-4 grid w-[200px] grid-flow-dense grid-cols-4 auto-rows-[18px] gap-1",
				iel("div", "col-span-2 bg-orange-300"),
				iel("div", "col-span-3 bg-orange-400"),
				iel("div", "bg-orange-500"),
				iel("div", "col-span-2 bg-orange-600"),
				iel("div", "bg-orange-300")),
		}},
		// Intrinsic tracks sized by their text: label, field, value.
		{name: "auto-tracks", height: 80, kids: []inode{
			iel("div", "grid w-[280px] grid-cols-[auto_1fr_auto] items-center gap-x-3 gap-y-1 text-sm",
				iel("div", "", itext("Name")),
				iel("div", "h-6 rounded border border-slate-300"),
				iel("div", "", itext("12")),
				iel("div", "", itext("Email address")),
				iel("div", "h-6 rounded border border-slate-300"),
				iel("div", "", itext("1,024"))),
			iel("div", "ml-4 grid grid-cols-[max-content_minmax(0,1fr)] gap-2 text-sm w-[160px]",
				iel("div", "bg-sky-100", itext("Status")),
				iel("div", "truncate bg-sky-200", itext("Deployed to production"))),
		}},
		// Self and content alignment.
		{name: "alignment", height: 96, kids: []inode{
			iel("div", "grid h-[80px] w-[150px] grid-cols-3 place-items-center bg-slate-100",
				iel("div", "size-4 bg-rose-400"),
				iel("div", "size-6 self-start bg-rose-500"),
				iel("div", "size-3 justify-self-end self-end bg-rose-600"),
				iel("div", "h-3 justify-self-stretch bg-rose-300")),
			iel("div", "ml-4 grid h-[80px] w-[150px] grid-cols-[30px_30px] grid-rows-[20px_20px] place-content-center gap-1 bg-slate-100",
				gridCells(4, "")...),
			iel("div", "ml-4 grid h-[80px] w-[150px] grid-cols-[30px_30px] justify-between content-end gap-1 bg-slate-100",
				gridCells(3, "h-4")...),
		}},
		// A grid as a flex item, a flex container in a grid, an inline
		// grid in a paragraph (aligned by its first baseline) and a
		// card grid that auto-fits two items.
		{name: "nesting", height: 64, kids: []inode{
			iel("div", "flex w-[150px] items-start gap-2",
				iel("div", "grid shrink-0 grid-cols-2 gap-1",
					iel("div", "size-5 bg-violet-400"),
					iel("div", "size-5 bg-violet-500")),
				iel("div", "grid grow grid-cols-[1fr_2fr] gap-1",
					iel("div", "h-5 bg-violet-300"),
					iel("div", "flex h-5 items-center justify-end bg-violet-200",
						iel("div", "size-3 bg-violet-600")))),
			iel("div", "ml-4 text-sm",
				itext("Total "),
				iel("div", "inline-grid grid-cols-2 gap-1",
					iel("div", "bg-lime-200 px-1", itext("A")),
					iel("div", "bg-lime-300 px-1", itext("B")))),
			iel("div", "ml-4 grid w-[150px] grid-cols-[repeat(auto-fit,minmax(40px,1fr))] gap-1",
				iel("div", "h-5 bg-cyan-400"),
				iel("div", "h-5 bg-cyan-500")),
		}},
		// Absolutely positioned items: a grid area as the containing block
		// (relative grid), its padding box for auto lines, and a static
		// position in a static grid.
		{name: "absolute", height: 88, kids: []inode{
			iel("div", "relative grid grid-cols-[60px_60px] grid-rows-[30px_30px] gap-2 bg-slate-100 p-1",
				iel("div", "bg-fuchsia-200"),
				iel("div", "absolute col-start-2 row-start-1 inset-1 bg-fuchsia-500"),
				iel("div", "absolute right-0 bottom-0 size-3 bg-fuchsia-700"),
				iel("div", "absolute row-start-2 col-[1/3] top-2 left-2 h-2 w-1/2 bg-fuchsia-400")),
			iel("div", "relative ml-4 h-[60px] w-[120px] bg-slate-200",
				iel("div", "grid h-full grid-cols-[40px] place-items-end p-2",
					iel("div", "absolute size-4 bg-pink-500"))),
		}},
	},
	// Box edges agree within 0.04 px (text widths in auto tracks). The
	// pixel differences are glyph anti-aliasing (Skia's against ours, as
	// in the inline gallery) and, in the nesting row, the snapping of the
	// boxes after the inline grid, whose text is 0.04 px narrower here.
	mean: [2]float64{1.5, 1.5},
	geom: [2]float64{0.1, 0.1},
}

func TestChromiumGrid(t *testing.T) { gridGallery.compare(t) }

func TestGridGallery(t *testing.T) { gridGallery.golden(t) }
