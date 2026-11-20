package devtools

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/internal/core"
)

// BoxModel draws the margin, border, padding and content of a box as
// nested rectangles with the size of each edge, like Chrome's box model
// diagram. The colors are the ones of the highlight pick mode draws over the
// app.
type BoxModel struct{ Box core.InspectNode }

// Build returns the diagram.
func (b BoxModel) Build() tela.Node {
	n := b.Box
	cw := n.Rect.Width() - n.Border.Left - n.Border.Right - n.Padding.Left - n.Padding.Right
	ch := n.Rect.Height() - n.Border.Top - n.Border.Bottom - n.Padding.Top - n.Padding.Bottom
	content := Div(Class("flex h-8 items-center justify-center rounded-sm border border-sky-500/50 bg-sky-500/25 font-mono text-[11px]"),
		Textf("%s × %s", fmtPx(max(cw, 0)), fmtPx(max(ch, 0))))
	return Div(Class("px-3 pt-1"), Role("img"),
		AriaLabel("Box model: margin, border, padding and content sizes"),
		BoxLayer{Label: "margin", E: n.Margin, Class: "border-orange-500/60 bg-orange-500/15", Inner: BoxLayer{
			Label: "border", E: n.Border, Class: "border-yellow-500/60 bg-yellow-500/20", Inner: BoxLayer{
				Label: "padding", E: n.Padding, Class: "border-emerald-500/60 bg-emerald-500/20", Inner: Wrap{content}}}},
		Div(Class("mt-2 flex justify-between font-mono text-[11px] text-muted-foreground"),
			Span(Textf("border box %s × %s", fmtPx(n.Rect.Width()), fmtPx(n.Rect.Height()))),
			Span(Textf("at %s, %s", fmtPx(n.Rect.X0), fmtPx(n.Rect.Y0)))),
	)
}

// Wrap makes a node a component, for a field that takes one.
type Wrap struct{ N tela.Node }

// Build returns the node.
func (w Wrap) Build() tela.Node { return w.N }

// BoxLayer is one rectangle of the box model: its four edges around what is
// inside.
type BoxLayer struct {
	Label string
	E     core.Edges
	Class string
	Inner tela.Component
}

// Build returns the layer.
func (l BoxLayer) Build() tela.Node {
	edge := func(v float32) tela.Node {
		if v == 0 {
			return Span(Class("text-muted-foreground/60"), Text("-"))
		}
		return Span(Text(fmtPx(v)))
	}
	side := Class("flex w-9 shrink-0 items-center justify-center")
	mid := Class("flex h-5 items-center justify-center")
	return Div(Class("relative rounded-md border border-dashed font-mono text-[11px]", l.Class),
		Span(Class("absolute top-1 left-2 text-[10px] text-muted-foreground"), Text(l.Label)),
		Div(mid, edge(l.E.Top)),
		Div(Class("flex items-center"),
			Div(side, edge(l.E.Left)),
			Div(Class("min-w-0 flex-1"), l.Inner),
			Div(side, edge(l.E.Right))),
		Div(mid, edge(l.E.Bottom)),
	)
}
