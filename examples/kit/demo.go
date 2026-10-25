package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/kit/icons"
	. "github.com/kuronosu/tela/h"
)

// The pages of the gallery are made of demos: a small caption and a stage
// that holds the components, the way shadcn/ui's documentation shows them.
// Everything is laid out with h elements and the theme's tokens
// (bg-card, border-border, text-muted-foreground), so one set of classes
// serves both color schemes and both styles.

// Stage classes: a row that wraps, a column and a grid of columns.
const (
	row = "flex flex-wrap items-center gap-4"
	col = "flex flex-col gap-4"
	// colStart is a column whose items are as wide as they are, not stretched
	// to the stage: a button that opens something. A w-fit calendar needs it
	// too, since in a stretching column it is laid out taller than it should
	// be (see the README).
	colStart = "flex flex-col items-start gap-4"
	// rowTop is a row whose items start at the top, such as a few calendars.
	rowTop = "flex flex-wrap items-start gap-6"
)

// demo is a captioned stage. class replaces the default layout of the
// stage (a wrapping row) when it is not empty.
func demo(title, note, class string, children ...tela.Child) tela.Node {
	if class == "" {
		class = row
	}
	return Div(Class("flex min-w-0 flex-col gap-3"),
		Div(Class("flex flex-col gap-1"),
			H3(Class("text-sm leading-none font-medium"), Text(title)),
			If(note != "", P(Class("text-sm text-muted-foreground"), Text(note))),
		),
		Div(Class("min-w-0 rounded-xl border border-border bg-card/50 p-6 dark:bg-card/30", class), Fragment(children...)),
	)
}

// grid lays out demos in two columns when the window is wide enough.
func grid(demos ...tela.Child) tela.Node {
	return Div(Class("grid grid-cols-1 items-start gap-x-8 gap-y-8 xl:grid-cols-2"), Fragment(demos...))
}

// stack lays out demos in one column.
func stack(demos ...tela.Child) tela.Node {
	return Div(Class("flex flex-col gap-8"), Fragment(demos...))
}

// ic is an icon of the given size class, in the text color.
func ic(icon tela.Icon, class ...string) tela.Node {
	return Icon(icon, Class("shrink-0"), Class(class...))
}

// muted is a line of secondary text.
func muted(s string) tela.Node {
	return P(Class("text-sm text-muted-foreground"), Text(s))
}

// readout is a small line that shows the state a demo has just changed:
// "Selected: October 25, 2026".
func readout(s string) tela.Node {
	return P(Class("text-sm text-muted-foreground tabular-nums"), Text(s))
}

// spinner is the loading icon of a button.
func spinner() tela.Node { return ic(icons.LoaderCircle, "size-4 animate-spin") }
