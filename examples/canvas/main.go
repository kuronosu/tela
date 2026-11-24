// Command canvas is three things drawn by Go functions with h.Canvas, the 2D
// drawing context of a browser's canvas, in one page of kit cards:
//
//   - an area chart (chart.go): a gradient under a smooth line, axes whose
//     labels are measured, a cursor that follows the pointer with its value
//     in a tooltip, and a spring when the series changes. The cursor is a
//     tela.Value that the draw reads, so moving it builds nothing.
//   - a particle field (particles.go): a simulation kept in a UseRef, moved
//     by Canvas.Delta and kept going by Canvas.RequestFrame, that is drawn to
//     the pointer. Paused, it asks for no frames; with reduced motion, it
//     starts paused.
//   - a sketch pad (sketch.go): freehand strokes kept in a Value, colors and
//     brush sizes, an eraser (CompositeDestinationOut), undo and clear.
//
// The colors are the theme's tokens (Canvas.Color), so the three follow the
// light and dark schemes. `tela snapshot ./examples/canvas` renders it
// without a window.
package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// Gallery is the page. The fields only choose the first state, for the
// previews: Still starts the particles paused, Probe says which point of the
// chart shows its value before the pointer touches it (-1: none), and Doodle
// puts a drawing on the sketch pad.
type Gallery struct {
	Still  bool
	Probe  int
	Doodle bool
}

func (g Gallery) Build() tela.Node {
	return Div(Class("h-screen overflow-y-auto"),
		Div(Class("mx-auto flex max-w-6xl flex-col gap-5 px-4 py-6 sm:px-8"),
			Div(Class("flex flex-wrap items-end justify-between gap-x-6 gap-y-3"),
				Div(Class("flex flex-col gap-1.5"),
					Span(Class("text-xs font-semibold tracking-[0.2em] text-primary uppercase"), Text("h.Canvas")),
					H1(Class("text-3xl font-semibold tracking-tight"), Text("Drawn by a Go function")),
					P(Class("text-sm text-muted-foreground"),
						Text("The 2D context of a browser's canvas, as a Go function that runs when something changes and follows the theme."))),
				kit.Badge{Variant: kit.Outline, Dot: true, DotClass: "bg-emerald-500", Children: Text("0% CPU when nothing moves")}),
			ChartCard{Probe: g.Probe},
			Div(Class("grid grid-cols-1 gap-6 lg:grid-cols-2"),
				ParticlesCard{Still: g.Still},
				SketchCard{Doodle: g.Doodle})))
}

// theme is kit's theme with cool neutrals, an indigo primary and five
// colors for the canvases (chart-1 to chart-5).
var theme = makeTheme()

func makeTheme() tela.Theme {
	t := kit.Theme()
	set := func(k, light, dark string) {
		t.Colors[k] = tela.SchemeColor{Light: tela.Hex(light), Dark: tela.Hex(dark)}
	}
	set("background", "#f5f6fb", "#0b0d19")
	set("foreground", "#0f1226", "#e8eaf6")
	set("card", "#ffffff", "#12152a")
	set("card-foreground", "#0f1226", "#e8eaf6")
	set("popover", "#ffffff", "#181c36")
	set("popover-foreground", "#0f1226", "#e8eaf6")
	set("secondary", "#eceef8", "#1c2040")
	set("secondary-foreground", "#0f1226", "#e8eaf6")
	set("muted", "#eceef8", "#171a33")
	set("muted-foreground", "#5d6483", "#9aa1c4")
	set("accent", "#e8eaf8", "#1f2447")
	set("accent-foreground", "#0f1226", "#e8eaf6")
	set("border", "#e0e3f1", "#242949")
	set("input", "#d3d7ea", "#2c3257")
	set("primary", "#4f46e5", "#8b8cf8")
	set("primary-foreground", "#ffffff", "#0b0d19")
	set("ring", "#4f46e5", "#8b8cf8")
	set("chart-1", "#4f46e5", "#8b8cf8")
	set("chart-2", "#0ea5e9", "#38bdf8")
	set("chart-3", "#ec4899", "#f472b6")
	set("chart-4", "#f59e0b", "#fbbf24")
	set("chart-5", "#10b981", "#34d399")
	return t
}

// App is the window; the color scheme is the system's.
var App = tela.App{
	Title:     "Tela — canvas",
	AppID:     "dev.telaui.canvas",
	Width:     1180,
	Height:    900,
	MinWidth:  520,
	MinHeight: 480,
	Theme:     &theme,
	RootClass: "bg-background text-foreground",
	Root:      Gallery{Probe: -1},
}

// Previews are the screens of `tela snapshot` and `tela inspect`: the page
// with its chart read at a point, its particles at rest and a drawing on the
// pad.
var Previews = tela.Previews{
	"Overview": {Root: Gallery{Still: true, Probe: 8, Doodle: true}},
}

// tight is the padding of the parts of a card, which narrows in a narrow window.
const tight = "px-4 sm:px-6"

// cardTitle is the head of the three cards: an icon and a title.
func cardTitle(icon tela.Icon, title string) tela.Component {
	return Div(Class("flex items-center gap-2"),
		Icon(icon, Class("size-4 text-primary")), Span(Text(title)))
}

func main() { tela.Main(App) }
