package core

import "testing"

// Right-to-left layout compared with Chromium (ADR-090): pages of
// Tailwind elements with dir attributes, measured and screenshot like the
// positioned scenes (positioned_chromium_test.go, which also says how to
// regenerate them; run -run TestChromiumRTL instead). Every element's
// border box is compared, and the pixels of the whole page.

var rtlScenes = []*posScene{
	{
		// Flex rows, logical spacing, borders and radii, space-x-*,
		// grid placement, positioned boxes with logical and physical
		// insets, nested direction changes and the rtl:/ltr: variants.
		name: "rtl_layout", w: 500, h: 480,
		root: iel("div", "space-y-3 p-3",
			idir("rtl", iel("div", "flex gap-2 rounded-lg bg-slate-100 py-2 ps-4 pe-1",
				iel("div", "ms-2 size-8 rounded-s-lg bg-indigo-500"),
				iel("div", "me-3 h-8 w-16 border-s-4 border-indigo-700 bg-indigo-200"),
				iel("div", "size-8 rounded-ss-xl rounded-ee-xl bg-rose-400"),
				iel("div", "ms-auto size-8 rounded-e-full bg-emerald-400"),
			)),
			idir("rtl", iel("div", "flex space-x-4 bg-slate-50",
				iel("div", "h-6 w-12 bg-amber-300"),
				iel("div", "h-6 w-12 bg-amber-400"),
				iel("div", "h-6 w-12 bg-amber-500"),
			)),
			idir("rtl", iel("div", "space-x-4 bg-slate-50",
				iel("div", "inline-block h-6 w-12 bg-lime-300"),
				iel("div", "inline-block h-6 w-12 bg-lime-500"),
			)),
			idir("rtl", iel("div", "grid grid-cols-4 gap-2",
				iel("div", "col-span-2 h-6 bg-sky-300"),
				iel("div", "h-6 bg-sky-500"),
				iel("div", "col-start-1 h-6 bg-sky-700"),
				iel("div", "col-end-5 h-6 bg-sky-200"),
				iel("div", "col-span-3 h-6 justify-self-end bg-sky-400 w-20"),
			)),
			idir("rtl", iel("div", "relative h-16 bg-slate-100",
				iel("div", "absolute top-2 start-2 size-6 bg-violet-500"),
				iel("div", "absolute bottom-2 end-4 size-6 bg-violet-300"),
				iel("div", "absolute top-2 left-2 size-4 bg-slate-900"),
				iel("div", "absolute inset-s-20 top-4 size-4 bg-pink-400"),
				iel("div", "absolute top-8 size-4 bg-pink-600"),
			)),
			idir("rtl", iel("div", "flex items-center justify-between bg-slate-100 p-2",
				iel("div", "h-6 w-10 bg-teal-400"),
				idir("ltr", iel("div", "flex gap-1",
					iel("div", "size-4 bg-teal-700"),
					iel("div", "size-4 bg-teal-300"),
				)),
				iel("div", "h-6 w-10 bg-blue-500 rtl:bg-red-500"),
			)),
			iel("div", "flex gap-2",
				iel("div", "size-6 bg-slate-300 ltr:bg-green-500 rtl:bg-red-500"),
				idir("rtl", iel("div", "flex w-40 justify-end gap-2 bg-slate-100",
					iel("div", "size-6 bg-slate-300 ltr:bg-green-500 rtl:bg-red-500"),
					iel("div", "size-6 border-e-4 border-orange-500 bg-orange-200"),
				)),
			),
			idir("rtl", iel("div", "flex flex-row-reverse flex-wrap gap-x-3 gap-y-1 bg-slate-50",
				iel("div", "h-5 w-32 bg-fuchsia-300"),
				iel("div", "h-5 w-32 bg-fuchsia-400"),
				iel("div", "h-5 w-32 bg-fuchsia-500"),
				iel("div", "h-5 w-32 bg-fuchsia-600"),
			)),
		),
		steps: [][]posScroll{{}},
		// Every edge is exact at x1; at x2 the line box of the
		// inline-blocks is half a pixel shorter than Blink's (ADR-048),
		// which moves the rows below it.
		geom: 0.02, geomY2: 0.5,
		mean: [2]float64{0.1, 0.7}, maxd: [2]int{64, 255},
	},
	{
		// Paragraphs: right-to-left base direction, start/end/left
		// alignment, mixed Latin and Arabic in both base directions,
		// dir="auto", inline elements and an inline-block, wrapping.
		name: "rtl_text", w: 500, h: 350, arabic: true,
		root: iel("div", "space-y-2 p-3 text-sm",
			idir("rtl", iel("p", "bg-slate-50", itext("مرحبا بالعالم، هذا نص عربي للاختبار"))),
			idir("rtl", iel("p", "", itext("النسخة 2.5 من Tela تدعم RTL بالكامل"))),
			idir("rtl", iel("p", "text-left", itext("نص إلى اليسار"))),
			idir("rtl", iel("p", "text-end", itext("نص في النهاية"))),
			idir("rtl", iel("p", "text-center", itext("نص في الوسط"))),
			iel("p", "", itext("Hello مرحبا بكم world, version 3")),
			idir("auto", iel("p", "", itext("عربي أولا then English"))),
			idir("auto", iel("p", "", itext("English first ثم عربي"))),
			idir("rtl", iel("p", "",
				// Inline elements have no box of their own to compare
				// (their fragments are checked in internal/text).
				itext("قبل "), noGeom(iel("span", "font-bold text-indigo-700", itext("غامق"))),
				itext(" و "), noGeom(iel("code", "rounded bg-slate-100 px-1 font-mono text-xs", itext("code()"))),
				itext(" ثم "), iel("button", "rounded bg-indigo-500 px-2 text-white", itext("زر")),
				itext(" بعد"))),
			idir("rtl", iel("p", "w-48 bg-amber-50",
				itext("هذه فقرة طويلة تلتف على عدة أسطر لاختبار كسر الأسطر من اليمين إلى اليسار"))),
		),
		steps: [][]posScroll{{}},
		// Paragraph edges within 0.05 px of Chromium (glyph advances,
		// ADR-034), vertical ones at x2 within the line rounding of
		// ADR-048; pixels differ by glyph anti-aliasing (ADR-029).
		geom: 0.06, geomY2: 0.5,
		mean: [2]float64{1.1, 1.4}, maxd: [2]int{255, 255},
	},
	{
		// Right-to-left scroll containers: the scroll origin is at the
		// right and scrollLeft goes negative toward the left (scrollTo
		// takes Chromium's coordinates); a sticky box at the inline
		// start (start-0) sticks to the right edge.
		name: "rtl_scroll", w: 500, h: 300,
		root: iel("div", "space-y-3 p-3",
			idir("rtl", iel("div", "flex gap-2 overflow-x-auto rounded-lg bg-slate-100 p-2",
				rowsOf(10, "h-12 w-20 shrink-0 rounded bg-indigo-300 p-2 text-xs", "Item")...)),
			idir("rtl", iel("div", "h-24 overflow-y-auto rounded-lg border border-slate-300",
				rowsOf(8, "h-8 border-b border-slate-200 px-2 text-xs/8", "Row")...)),
			idir("rtl", iel("div", "flex overflow-x-auto bg-slate-50",
				append([]inode{iel("div", "sticky start-0 z-10 h-10 w-16 shrink-0 bg-amber-300 p-2 text-xs", itext("Name"))},
					rowsOf(8, "h-10 w-20 shrink-0 border-e border-slate-300 p-2 text-xs", "Col")...)...)),
		),
		// e1 is the strip of items, e12 the list, e21 the sticky strip.
		steps: [][]posScroll{
			{},
			{{1, -100, 0}, {12, 0, 50}, {21, -150, 0}},
			{{1, -1000, 0}, {12, 0, 1000}, {21, -1000, 0}},
		},
		geom: 0.02,
		mean: [2]float64{0.5, 0.5}, maxd: [2]int{255, 255},
	},
}

// noGeom leaves n out of the geometry comparison.
func noGeom(n inode) inode { n.noGeom = true; return n }

func TestChromiumRTL(t *testing.T) {
	for _, s := range rtlScenes {
		t.Run(s.name, s.compare)
	}
}

func TestRTLGolden(t *testing.T) {
	for _, s := range rtlScenes {
		t.Run(s.name, s.golden)
	}
}
