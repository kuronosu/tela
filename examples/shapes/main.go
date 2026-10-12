// Command shapes is a showcase of what Tela's styling can paint: a page of
// twelve cards with shadows, gradients (linear, radial and conic), rings,
// blur and backdrop blur, opacity, rounded and rotated shapes, all written
// as elements and Tailwind classes. `tela snapshot ./examples/shapes`
// renders it without a window.
//
// It started as the phase-1 scene drawn straight into a display list (that
// scene now lives on as a golden test of internal/gfx/raster) and was
// rewritten with the public API of phase 3. Nothing reacts to the pointer
// yet: events, hover states and transitions arrive in phase 4 (ADR-040).
package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Shapes is the page: twelve cards of a fixed size on a soft gradient, with
// two blurred glows in the corners. The cards wrap and center in the
// window: four columns at the default 960x640, three in a tiled window
// about 700 px wide (with less padding below the md breakpoint).
func Shapes() tela.Node {
	cards := []tela.Node{
		ProfileCard(), ChartCard(), PhotoCard(), TogglesCard(),
		ProgressCard(), SearchCard(), GradientCard(), SunsetCard(),
		ConicCard(), GlassCard(), ShapesCard(), DropCard(),
	}
	cells := []tela.Child{Class("relative flex h-full flex-wrap content-center justify-center gap-6 p-5 md:p-10")}
	for i, c := range cards {
		cells = append(cells, Div(Key(i), Class("flex h-[170px] w-[202px]"), c))
	}
	return Div(Class("relative h-screen overflow-hidden bg-linear-to-br from-slate-50 to-indigo-50"),
		// Glows: blurred discs behind the grid, clipped by the page.
		Div(Class("absolute -top-40 -right-24 size-[28rem] rounded-full bg-indigo-200/60 blur-3xl")),
		Div(Class("absolute -bottom-40 -left-24 size-[28rem] rounded-full bg-sky-200/70 blur-3xl")),
		// Grid layout arrives with phase 6: the cards wrap in a flex row,
		// each in a cell of the card size.
		Div(cells...),
	)
}

func main() {
	tela.Run(tela.App{
		Title:  "Tela — shapes",
		AppID:  "dev.tela.shapes",
		Width:  960,
		Height: 640,
		Root:   Shapes,
	})
}

// Preview is the screen of `tela snapshot` and `tela inspect`.
func Preview() tela.Node { return Shapes() }

// cell is what every card has: all the room of its cell.
const cell = "min-w-0 flex-1 "

// card is a white rounded card; shadow is a Tailwind shadow class and extra
// any other classes. The 12 cards share it, so they share their padding
// and radius.
func card(shadow, extra string, children ...tela.Child) tela.Node {
	return Div(append([]tela.Child{Class(cell + "flex flex-col rounded-2xl bg-white p-4 " + shadow + " " + extra)}, children...)...)
}

// title and subtitle are the two-line heading used by several cards.
func title(text, sub string) tela.Node {
	return Div(
		P(Class("text-sm leading-5 font-semibold text-slate-800"), Text(text)),
		P(Class("text-xs text-slate-400"), Text(sub)),
	)
}
