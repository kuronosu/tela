// Command board is a small kanban board built on package kit in its default
// style: three columns of cards that can be dragged from one column to
// another, or reordered inside a column, and moved, edited or deleted from
// a menu.
//
// Dragging uses the gesture handlers OnDragStart, OnDrag and OnDragEnd. The
// dragged card follows the pointer with tela.TranslateFn reading a
// tela.UseValue, so every pointer move only repaints that card; the
// column under the pointer and the card it hovers decide the drop target
// (see DragState), and the board, a comparable array of card IDs in
// UseState, is updated when the pointer is released.
//
// Each card is a kit.Card whose action is a kit.DropdownMenu (Move to,
// Edit, Delete) and whose area is a kit.ContextMenu with the same items;
// Edit opens a kit.Dialog with a kit.Field per input, Delete asks with a
// kit.AlertDialog, and both answer with a toast.
//
// `tela snapshot ./examples/board` renders it without a window; add --dark
// for the dark theme.
package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// Kanban is the page, inside the Toaster that the cards show their toasts in.
func Kanban() tela.Component { return kit.Toaster{Children: tela.C(kanban)} }

func kanban() tela.Node {
	board, setBoard := tela.UseState(Initial)
	cards, setCards := tela.UseState(tasks)
	drag := useDrag()
	full := board.Full()

	return Div(Class("flex min-h-screen flex-col overflow-hidden"),
		Div(Class("flex flex-wrap items-center justify-between gap-4 px-6 pt-6 pb-4 md:px-8 md:pt-8"),
			Div(Class("flex flex-col gap-1"),
				H1(Class("text-2xl font-bold tracking-tight"), Text("Roadmap")),
				P(Class("text-sm text-muted-foreground"), Text("Drag cards between columns to update their status.")),
			),
			Div(Class("flex items-center gap-4"),
				Div(Class("hidden items-center gap-1.5 sm:flex"),
					team("AL"), team("MK"), team("JS"), team("OM"),
				),
				kit.Button{Children: Text("Invite")},
			),
		),
		Div(Class("flex min-h-0 flex-1 gap-4 px-6 pb-6 md:px-8 md:pb-8"),
			MapKeyed([]int{0, 1, 2}, func(i int) int { return i }, func(i int) tela.Component {
				return Lane{Index: i, Cards: board[i], Tasks: cards, Full: full, D: drag, Set: setBoard, SetTasks: setCards}
			}),
		),
	)
}

// team is one avatar of the row in the header (kit.Avatar draws the initials
// on a muted disc).
func team(initials string) tela.Component {
	return kit.Avatar{Fallback: initials, Alt: initials}
}

var theme = kit.Theme()

// App is the window; `tela snapshot` and the tests read it too.
var App = tela.App{
	Title:     "Tela — board",
	AppID:     "dev.telaui.board",
	Width:     1280,
	Height:    800,
	Theme:     &theme,
	RootClass: "bg-background text-foreground",
	Root:      Kanban(),
}

func main() { tela.Main(App) }
