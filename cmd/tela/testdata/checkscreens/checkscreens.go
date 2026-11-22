// Package checkscreens is a test package of cmd/tela with the screens that
// tela check and tela verify judge: one with nothing to find, one with a
// problem of each kind, and two that break only at a narrow width or with
// large text.
package checkscreens

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Previews are the screens.
var Previews = tela.Previews{
	"Clean":  {Root: tela.C(Clean)},
	"Broken": {Root: tela.C(Broken)},
	"Narrow": {Root: tela.C(Narrow)},
	"Scaled": {Root: tela.C(Scaled)},
}

// Clean has nothing to report.
func Clean() tela.Node {
	return Div(Class("min-h-screen bg-white p-4 text-zinc-900"),
		H1(Class("text-xl font-semibold"), Text("Settings")),
		Button(Class("mt-4 h-9 rounded bg-zinc-900 px-4 text-white outline-none focus-visible:ring-2 focus-visible:ring-blue-500"), Text("Save")))
}

// Broken has a problem of several kinds.
func Broken() tela.Node {
	return Div(Class("flex min-h-screen flex-col items-start gap-3 bg-white p-4 text-zinc-900"),
		P(Class("text-sm text-zinc-400"), Text("Quarterly revenue")),
		Button(Class("size-9 rounded bg-zinc-900 focus-visible:ring-2")),
		Div(Class("w-20 overflow-hidden whitespace-nowrap"), Text("A very long caption that does not fit")),
		Button(Class("h-9 rounded bg-zinc-200 px-4 outline-none"), Text("Cancel")))
}

// Narrow is wider than a phone's window.
func Narrow() tela.Node {
	return Div(Class("flex min-h-screen gap-2 bg-white p-2 text-zinc-900"),
		Div(Class("w-[300px] shrink-0 whitespace-nowrap"), Text("A left column that stays wide")),
		Div(Class("shrink-0 whitespace-nowrap"), Text("Right column")))
}

// Scaled has a box too short for large text.
func Scaled() tela.Node {
	return Div(Class("min-h-screen bg-white p-4 text-zinc-900"),
		Div(Class("h-[24px] overflow-hidden"), Text("Fixed height")))
}
