package core

import "testing"

// The divide gallery (ADR-203) compares divide-x, divide-y, their widths
// and -reverse, the colors and styles, a child's own borders, rtl and the
// dark: variant with Chromium and Tailwind v4, cell by cell: the lines are
// the end (or start) and bottom (or top) borders of every child but the
// last, :where() rules that a child's own border classes win over.

func divCell(class, label string) inode {
	return iel("div", "px-3 py-2 text-sm "+class, itext(label))
}

func divRowOf(class string, kids ...inode) inode {
	return iel("div", class, kids...)
}

var divideGallery = &cssGallery{
	name:    "divide",
	width:   360,
	fontCSS: "@custom-variant dark (&:is(.dark *));\n@theme { --font-sans: \"Inter\"; }\n" + cssGalleryFonts,
	// Text anti-aliasing (Skia's glyph coverage differs from ours) and the
	// antialiased ends of dashes are all that differs.
	mean: [2]float64{1.7, 1.1},
	geom: [2]float64{1.01, 1.01},
	rows: []cssRow{
		{name: "y", height: 120, kids: []inode{
			divRowOf("w-40 divide-y divide-zinc-300 rounded-md border border-zinc-300",
				divCell("", "One"), divCell("", "Two"), divCell("", "Three")),
			divRowOf("ml-4 w-40 divide-y-4 divide-rose-400",
				divCell("", "Four"), divCell("", "Five"), divCell("", "Six")),
		}},
		{name: "x", height: 56, kids: []inode{
			divRowOf("flex divide-x divide-zinc-400",
				divCell("", "A"), divCell("", "B"), divCell("", "C")),
			divRowOf("ml-4 flex divide-x-2 divide-dashed divide-sky-500/60",
				divCell("", "D"), divCell("", "E"), divCell("", "F")),
			divRowOf("ml-4 flex divide-x-[3px] divide-dotted divide-emerald-500",
				divCell("", "G"), divCell("", "H")),
		}},
		{name: "reverse", height: 56, kids: []inode{
			divRowOf("flex flex-row-reverse divide-x-2 divide-x-reverse divide-violet-500",
				divCell("", "1"), divCell("", "2"), divCell("", "3")),
			divRowOf("ml-4 flex divide-x divide-x-reverse divide-zinc-400",
				divCell("", "4"), divCell("", "5"), divCell("", "6")),
			divRowOf("ml-4 w-24 divide-y divide-y-reverse divide-zinc-400",
				divCell("", "7"), divCell("", "8")),
		}},
		{name: "styles", height: 120, kids: []inode{
			divRowOf("w-24 divide-y-2 divide-double divide-zinc-500",
				divCell("", "x"), divCell("", "y"), divCell("", "z")),
			divRowOf("ml-3 w-24 divide-y-2 divide-solid divide-red-500/40",
				divCell("", "x"), divCell("", "y"), divCell("", "z")),
			divRowOf("ml-3 w-24 divide-y-2 divide-none divide-zinc-500",
				divCell("", "x"), divCell("", "y"), divCell("", "z")),
			divRowOf("ml-3 w-24 divide-y-0 divide-zinc-500",
				divCell("", "x"), divCell("", "y"), divCell("", "z")),
		}},
		{name: "own-border", height: 160, kids: []inode{
			divRowOf("w-28 divide-y-2 divide-red-500",
				divCell("border-b-4 border-b-blue-500", "own b-4"),
				divCell("border-b-0", "b-0"),
				divCell("border-t", "border-t"),
				divCell("", "last")),
			divRowOf("ml-3 w-28 divide-y-2 divide-red-500",
				divCell("border border-green-500", "border"),
				divCell("border-0", "border-0"),
				divCell("border-dashed", "dashed"),
				divCell("", "last")),
			divRowOf("ml-3 w-28 divide-y divide-zinc-400",
				divCell("border-blue-500", "own color"),
				divCell("border-x-2 border-x-green-500", "x-2"),
				divCell("", "last")),
		}},
		{name: "rtl", height: 56, kids: []inode{
			idir("rtl", divRowOf("flex divide-x-2 divide-zinc-400",
				divCell("", "uno"), divCell("", "dos"), divCell("", "tres"))),
			idir("rtl", divRowOf("ml-4 flex divide-x-2 divide-x-reverse divide-sky-500",
				divCell("", "uno"), divCell("", "dos"), divCell("", "tres"))),
		}},
		{name: "dark", height: 130, kids: []inode{
			func() inode {
				p := divRowOf("-mx-4 flex h-full w-[360px] shrink-0 items-center gap-4 bg-zinc-900 px-4 text-zinc-100",
					divRowOf("w-28 divide-y divide-zinc-300 dark:divide-zinc-700",
						divCell("", "One"), divCell("", "Two"), divCell("", "Three")),
					divRowOf("flex divide-x-2 divide-zinc-300 dark:divide-amber-500/70",
						divCell("", "A"), divCell("", "B"), divCell("", "C")))
				p.dark = true
				return p
			}(),
		}},
	},
}

// TestChromiumDivide compares the divide gallery with Chromium.
func TestChromiumDivide(t *testing.T) { divideGallery.compare(t) }

// TestDivideGallery is the gallery's golden, at scales 1 and 2.
func TestDivideGallery(t *testing.T) { divideGallery.golden(t) }
