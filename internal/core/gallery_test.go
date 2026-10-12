package core

import (
	"fmt"
	"html"
	"strings"

	"github.com/kuronosu/tela/internal/style"
)

// The box gallery: small boxes styled with Tailwind classes that cover
// box painting (radii, borders, shadows, rings, gradients, effects,
// transforms, outlines, overflow clipping, z-order), plus the Counter card
// of ARCHITECTURE section 1. The same tree renders in Tela and, as HTML
// with Tailwind's compiled CSS, in Chromium (chromium_test.go).

// gnode is an element of the gallery: a tag, its classes, text and
// children. It converts to a Tela Node and to HTML.
type gnode struct {
	tag   string // div, h1 or button
	class string
	text  string
	kids  []gnode
}

func gdiv(class string, kids ...gnode) gnode { return gnode{tag: "div", class: class, kids: kids} }

var gnodeKinds = map[string]style.Kind{"div": style.KindDiv, "h1": style.KindH1, "button": style.KindButton}

func (g gnode) node() Node {
	var c []Child
	if g.class != "" {
		c = append(c, Class(g.class))
	}
	if g.text != "" {
		c = append(c, Text(g.text))
	}
	for _, k := range g.kids {
		c = append(c, k.node())
	}
	return Primitive(gnodeKinds[g.tag], c...)
}

func (g gnode) html(sb *strings.Builder, indent string) {
	fmt.Fprintf(sb, "%s<%s", indent, g.tag)
	if g.class != "" {
		fmt.Fprintf(sb, " class=\"%s\"", html.EscapeString(g.class))
	}
	sb.WriteString(">")
	sb.WriteString(html.EscapeString(g.text))
	if len(g.kids) > 0 {
		sb.WriteString("\n")
		for _, k := range g.kids {
			k.html(sb, indent+"  ")
		}
		sb.WriteString(indent)
	}
	fmt.Fprintf(sb, "</%s>\n", g.tag)
}

// galleryCase is one cell of the gallery.
type galleryCase struct {
	name string
	cell string // extra classes of the cell
	box  []gnode
}

// Gallery geometry, in logical pixels.
const (
	galleryCols   = 8
	galleryCell   = 96
	galleryWidth  = galleryCols * galleryCell
	counterHeight = 260
)

const glass = "bg-linear-to-br from-pink-500 to-violet-600"

var galleryCases = []galleryCase{
	{name: "bg", box: []gnode{gdiv("size-16 bg-sky-500")}},
	{name: "rounded-md", box: []gnode{gdiv("size-16 rounded-md bg-sky-500")}},
	{name: "rounded-xl", box: []gnode{gdiv("size-16 rounded-xl bg-indigo-500")}},
	{name: "rounded-3xl", box: []gnode{gdiv("size-16 rounded-3xl bg-violet-500")}},
	{name: "rounded-full", box: []gnode{gdiv("h-10 w-20 rounded-full bg-emerald-500")}},
	{name: "corners", box: []gnode{gdiv("size-16 rounded-tl-3xl rounded-br-3xl bg-rose-500")}},
	{name: "rounded-t", box: []gnode{gdiv("size-16 rounded-t-2xl bg-amber-400")}},
	{name: "ellipse", box: []gnode{gdiv("h-12 w-20 rounded-[50%] bg-teal-500")}},

	{name: "border", box: []gnode{gdiv("size-16 rounded-lg border border-slate-300 bg-white")}},
	{name: "border-2", box: []gnode{gdiv("size-16 rounded-xl border-2 border-indigo-500 bg-indigo-50")}},
	{name: "side-colors", box: []gnode{gdiv("size-16 border-4 border-t-red-500 border-r-green-500 border-b-blue-500 border-l-yellow-400")}},
	{name: "side-colors-rounded", box: []gnode{gdiv("size-16 rounded-2xl border-4 border-t-red-500 border-r-green-500 border-b-blue-500 border-l-yellow-400")}},
	{name: "side-widths", box: []gnode{gdiv("size-16 rounded-xl border-t-2 border-r-4 border-b-8 border-l-[6px] border-slate-700")}},
	{name: "widths-colors", box: []gnode{gdiv("size-16 rounded-lg border-t-8 border-l-8 border-t-sky-500 border-l-pink-500 bg-slate-100")}},
	{name: "border-x", box: []gnode{gdiv("size-16 border-x-4 border-fuchsia-500 bg-fuchsia-100")}},
	{name: "border-translucent", box: []gnode{gdiv("size-16 rounded-xl border-8 border-black/20 bg-sky-400")}},

	{name: "dashed", box: []gnode{gdiv("size-16 rounded-lg border-2 border-dashed border-slate-500")}},
	{name: "dashed-square", box: []gnode{gdiv("h-12 w-20 border-4 border-dashed border-indigo-500")}},
	{name: "dotted", box: []gnode{gdiv("size-16 rounded-full border-4 border-dotted border-orange-500")}},
	{name: "double", box: []gnode{gdiv("size-16 rounded-lg border-8 border-double border-emerald-600")}},
	{name: "shadow-sm", box: []gnode{gdiv("size-16 rounded-lg bg-white shadow-sm")}},
	{name: "shadow-md", box: []gnode{gdiv("size-16 rounded-lg bg-white shadow-md")}},
	{name: "shadow-lg", box: []gnode{gdiv("size-16 rounded-lg bg-white shadow-lg")}},
	{name: "shadow-xl", box: []gnode{gdiv("size-16 rounded-lg bg-white shadow-xl")}},

	{name: "shadow-2xl", box: []gnode{gdiv("size-16 rounded-lg bg-white shadow-2xl")}},
	{name: "shadow-inner", box: []gnode{gdiv("size-16 rounded-lg bg-slate-100 shadow-inner")}},
	{name: "inset-shadow", box: []gnode{gdiv("size-16 rounded-xl bg-white inset-shadow-sm")}},
	{name: "shadow-colored", box: []gnode{gdiv("size-16 rounded-xl bg-indigo-500 shadow-lg shadow-indigo-500/50")}},
	{name: "ring", box: []gnode{gdiv("size-16 rounded-lg bg-white ring-2 ring-indigo-500")}},
	{name: "ring-offset", box: []gnode{gdiv("size-14 rounded-full bg-sky-500 ring-2 ring-sky-500 ring-offset-2")}},
	{name: "ring-inset", box: []gnode{gdiv("size-16 rounded-xl bg-amber-100 ring-4 ring-amber-500 ring-inset")}},
	{name: "ring-shadow", box: []gnode{gdiv("size-16 rounded-xl bg-white shadow-md ring-1 ring-black/5")}},

	{name: "linear", box: []gnode{gdiv("h-16 w-20 rounded-lg bg-linear-to-r from-cyan-500 to-blue-500")}},
	{name: "linear-via", box: []gnode{gdiv("h-16 w-20 rounded-lg bg-linear-to-br from-indigo-500 via-purple-500 to-pink-500")}},
	{name: "linear-angle", box: []gnode{gdiv("h-16 w-20 bg-linear-65 from-purple-500 to-pink-500")}},
	{name: "radial", box: []gnode{gdiv("size-16 rounded-full bg-radial from-yellow-200 to-orange-600")}},
	{name: "conic", box: []gnode{gdiv("size-16 rounded-full bg-conic from-red-500 via-yellow-400 to-red-500")}},
	{name: "gradient-border", box: []gnode{gdiv("h-16 w-20 rounded-xl border-4 border-transparent bg-linear-to-r from-emerald-400 to-sky-500")}},
	{name: "opacity", box: []gnode{gdiv("size-16 rounded-lg bg-indigo-500 opacity-50", gdiv("mt-6 ml-6 size-8 bg-white"))}},
	{name: "blur", box: []gnode{gdiv("size-12 rounded-xl bg-rose-500 blur-sm")}},

	{name: "backdrop-blur", cell: "relative " + glass, box: []gnode{
		gdiv("absolute top-4 left-6 size-10 rounded-full bg-yellow-300"),
		gdiv("relative h-14 w-16 rounded-xl border border-white/40 bg-white/20 backdrop-blur-sm"),
	}},
	{name: "white-15", cell: glass, box: []gnode{gdiv("h-12 w-16 rounded-xl bg-white/15")}},
	{name: "rotate", box: []gnode{gdiv("size-12 rounded-lg bg-sky-500 shadow-md rotate-12")}},
	{name: "scale", box: []gnode{gdiv("size-16 rounded-lg bg-emerald-500 scale-75")}},
	{name: "transform-mix", box: []gnode{gdiv("size-12 bg-orange-500 translate-x-2 -rotate-6 scale-110")}},
	{name: "skew", box: []gnode{gdiv("size-12 bg-lime-500 skew-x-12")}},
	{name: "outline", box: []gnode{gdiv("size-16 rounded-lg bg-white outline-2 outline-offset-2 outline-sky-500")}},
	{name: "outline-dashed", box: []gnode{gdiv("size-16 bg-slate-100 outline-2 outline-dashed outline-rose-500")}},

	{name: "overflow", box: []gnode{gdiv("size-16 overflow-hidden rounded-2xl bg-slate-200", gdiv("mt-8 ml-8 size-16 bg-indigo-500"))}},
	{name: "overflow-abs", box: []gnode{gdiv("relative size-16 overflow-hidden rounded-full bg-slate-200",
		gdiv("absolute -right-4 -bottom-4 size-12 bg-pink-500"))}},
	{name: "z-index", cell: "relative", box: []gnode{
		gdiv("absolute top-4 left-5 size-10 bg-sky-500 z-20"),
		gdiv("absolute top-8 left-9 size-10 bg-pink-500 z-10"),
		gdiv("absolute top-12 left-13 size-10 bg-amber-400"),
	}},
	{name: "negative-z", cell: "relative isolate", box: []gnode{gdiv("relative size-14 bg-sky-200/80",
		gdiv("absolute -top-3 -left-3 size-10 rounded-full bg-rose-500 -z-10"))}},
	{name: "flex-z", box: []gnode{gdiv("flex",
		gdiv("relative z-30 size-10 rounded-full border-2 border-white bg-sky-500"),
		gdiv("relative z-20 -ml-4 size-10 rounded-full border-2 border-white bg-pink-500"),
		gdiv("relative z-10 -ml-4 size-10 rounded-full border-2 border-white bg-amber-400"),
	)}},
	{name: "invisible", box: []gnode{gdiv("invisible size-16 bg-red-500 p-2", gdiv("visible size-8 bg-green-500"))}},
	{name: "card", box: []gnode{gdiv("h-14 w-20 rounded-xl border border-slate-200 bg-white p-2 shadow-sm",
		gdiv("h-2 w-10 rounded-full bg-slate-300"), gdiv("mt-2 h-2 w-14 rounded-full bg-slate-200"))}},
	{name: "pill-ring", box: []gnode{gdiv("h-8 w-20 rounded-full bg-indigo-600 shadow-sm ring-1 ring-indigo-700/10 ring-inset")}},
}

// galleryCounter is the Counter card of ARCHITECTURE section 1 (count 0).
var galleryCounter = gdiv("flex h-[260px] items-center justify-center bg-slate-900",
	gdiv("flex flex-col items-center gap-6 p-10 rounded-3xl bg-linear-to-br from-indigo-500 to-fuchsia-600 shadow-2xl",
		gnode{tag: "h1", class: "text-5xl font-bold text-white tracking-tight", text: "Count: 0"},
		gnode{tag: "button", class: "px-6 py-3 rounded-xl bg-white/15 text-white backdrop-blur hover:bg-white/25 active:scale-95 transition", text: "Increment"},
	))

const galleryCellClass = "flex size-24 shrink-0 items-center justify-center"

// galleryRoot returns the gallery: a wrapping row of cells, then the
// Counter.
func galleryRoot() gnode {
	cells := make([]gnode, len(galleryCases))
	for i, c := range galleryCases {
		cl := galleryCellClass
		if c.cell != "" {
			cl += " " + c.cell
		}
		cells[i] = gdiv(cl, c.box...)
	}
	return gdiv(fmt.Sprintf("w-[%dpx]", galleryWidth), gdiv("flex flex-wrap", cells...), galleryCounter)
}

// gallerySize returns the logical size of the gallery page.
func gallerySize() (w, h float32) {
	rows := (len(galleryCases) + galleryCols - 1) / galleryCols
	return galleryWidth, float32(rows*galleryCell + counterHeight)
}

// galleryRects returns the logical rectangle of each case's cell, then
// the Counter's, with their names.
func galleryRects() (names []string, rects [][4]int) {
	for i, c := range galleryCases {
		x, y := (i%galleryCols)*galleryCell, (i/galleryCols)*galleryCell
		names = append(names, c.name)
		rects = append(rects, [4]int{x, y, x + galleryCell, y + galleryCell})
	}
	_, h := gallerySize()
	names = append(names, "counter")
	rects = append(rects, [4]int{0, int(h) - counterHeight, galleryWidth, int(h)})
	return names, rects
}

// galleryHTML returns the HTML page of the gallery; it links the compiled
// Tailwind CSS gallery.css.
func galleryHTML() string {
	var sb strings.Builder
	sb.WriteString("<!doctype html>\n<html>\n<head>\n<meta charset=\"utf-8\">\n" +
		"<link rel=\"stylesheet\" href=\"gallery.css\">\n</head>\n<body class=\"bg-white\">\n")
	galleryRoot().html(&sb, "")
	sb.WriteString("</body>\n</html>\n")
	return sb.String()
}
