// Command board is a small kanban board: three columns of cards that can be
// dragged from one column to another, or reordered inside a column.
//
// Dragging uses the gesture handlers OnDragStart, OnDrag and OnDragEnd. The
// dragged card follows the pointer with tela.TranslateFn reading a
// tela.UseValue, so every pointer move only repaints that card; the
// column under the pointer and the card it hovers decide the drop target
// (see DragState), and the board, a comparable array of card IDs in
// UseState, is updated when the pointer is released.
//
// `tela snapshot ./examples/board` renders it without a window; add --dark
// for the dark theme.
package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// Kanban is the page.
func Kanban() tela.Node {
	board, setBoard := tela.UseState(Initial)
	drag := useDrag()

	return Div(Class("flex h-screen flex-col overflow-hidden bg-white text-zinc-950 dark:bg-zinc-950 dark:text-zinc-50"),
		Div(Class("flex flex-wrap items-center justify-between gap-4 px-6 pt-6 pb-4 md:px-8 md:pt-8"),
			Div(Class("flex flex-col gap-1"),
				H1(Class("text-2xl font-bold tracking-tight"), Text("Roadmap")),
				P(Class("text-sm text-zinc-500 dark:text-zinc-400"), Text("Drag cards between columns to update their status.")),
			),
			Div(Class("flex items-center gap-4"),
				Div(Class("hidden items-center sm:flex"),
					avatar("AL", "from-violet-400 to-indigo-500"), avatar("MK", "from-sky-400 to-cyan-500"),
					avatar("JS", "from-amber-400 to-orange-500"), avatar("OM", "from-rose-400 to-pink-500"),
				),
				Button(Class("flex h-9 items-center rounded-md bg-zinc-900 px-4 text-sm font-medium text-zinc-50 shadow-sm hover:bg-zinc-700 active:bg-zinc-950 "+
					"dark:bg-zinc-50 dark:text-zinc-900 dark:hover:bg-zinc-200"),
					Text("Invite")),
			),
		),
		Div(Class("flex min-h-0 flex-1 gap-4 px-6 pb-6 md:px-8 md:pb-8"),
			Map([]int{0, 1, 2}, func(i int) tela.Node {
				return tela.WithKey(tela.CP(Lane, LaneProps{Index: i, Cards: board[i], D: drag, Set: setBoard}), i)
			}),
		),
	)
}

func avatar(initials, colors string) tela.Node {
	return Div(Class("flex size-8 items-center justify-center -ml-2 rounded-full border-2 border-white bg-linear-to-br first:ml-0 text-xs font-semibold text-white dark:border-zinc-950 "+colors),
		Text(initials))
}

// Preview is the screen of `tela snapshot` and `tela inspect`.
func Preview() tela.Node { return Kanban() }

func main() {
	tela.Run(tela.App{
		Title:  "Tela — board",
		AppID:  "dev.tela.board",
		Width:  1280,
		Height: 800,
		Root:   Kanban,
	})
}
