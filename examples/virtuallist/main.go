// Command virtuallist is a directory of 100,000 people in an h.VirtualList:
// only the rows in view (plus a few) are built, laid out and painted, rows
// with a note are taller and measured when they appear, and the header's
// buttons jump to a row with ElementRef.ScrollToIndex.
//
// `tela snapshot ./examples/virtuallist` renders it without a window; add
// --dark for the dark theme.
package main

import (
	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
)

// rowHeight is the estimated height of a row (a 40 px avatar, 12 px of
// padding above and below and a 1 px border); noteHeight that of a row
// with a note, whose real height depends on how the note wraps.
const rowHeight, noteHeight = 65, 110

// estimate is the estimated height of row i.
func estimate(i int) float32 {
	if HasNote(i) {
		return noteHeight
	}
	return rowHeight
}

// renderRow returns row i: a struct component with comparable fields, so a
// build of the list that only moves its window skips the rows that stay.
func renderRow(i int) tela.Node { return Fragment(Row{Index: i}) }

// Directory is the page: a header with the jump buttons over the list.
func Directory() tela.Node {
	list := tela.UseElementRef()
	jump := func(label string, i int, align tela.ScrollAlign) tela.Node {
		return Button(Class("flex h-8 items-center rounded-md border border-zinc-200 bg-white px-3 text-sm font-medium text-zinc-700 shadow-xs",
			"hover:bg-zinc-100 active:bg-zinc-200 dark:border-zinc-800 dark:bg-zinc-900 dark:text-zinc-200 dark:hover:bg-zinc-800"),
			OnClick(func() { list.ScrollToIndex(i, align) }),
			Text(label))
	}
	return Div(Class("flex h-screen flex-col bg-zinc-50 text-zinc-950 dark:bg-zinc-950 dark:text-zinc-50"),
		Div(Class("flex shrink-0 justify-center px-6 pt-6 pb-4"),
			Div(Class("flex w-full max-w-3xl flex-wrap items-end justify-between gap-4"),
				Div(Class("flex flex-col gap-1"),
					Div(Class("flex items-center gap-2"),
						H1(Class("text-2xl font-bold tracking-tight"), Text("Directory")),
						Span(Class("rounded-full bg-indigo-50 px-2 py-0.5 text-xs font-medium text-indigo-700 tabular-nums dark:bg-indigo-950 dark:text-indigo-300"),
							Text(Grouped(Count))),
					),
					P(Class("text-sm text-zinc-500 dark:text-zinc-400"), Text("Only the rows in view are built. Rows with a note are measured as they appear.")),
				),
				Div(Class("flex flex-wrap items-center gap-2"),
					Span(Class("text-sm text-zinc-500 dark:text-zinc-400"), Text("Jump to")),
					jump("First", 0, tela.AlignStart),
					jump("Row 1,000", 999, tela.AlignStart),
					jump("Row 50,000", 49_999, tela.AlignCenter),
					jump("Last", Count-1, tela.AlignEnd),
				),
			),
		),
		Div(Class("flex min-h-0 flex-1 justify-center px-6 pb-6"),
			VirtualList(Count, estimate, renderRow,
				Ref(list), AriaLabel("People"),
				Class("w-full max-w-3xl rounded-xl border border-zinc-200 bg-white shadow-xs dark:border-zinc-800 dark:bg-zinc-900")),
		),
	)
}

// Row is one person: avatar, name and number, role and city, and the note
// of one row in seven.
type Row struct {
	Index int
}

func (p Row) Build() tela.Node {
	person := PersonAt(p.Index)
	return Div(Class("flex items-start gap-3 border-b border-zinc-100 px-4 py-3 dark:border-zinc-800"),
		Div(Class("flex size-10 shrink-0 items-center justify-center rounded-full bg-linear-to-br text-sm font-semibold text-white", person.Colors),
			Text(person.Initials)),
		Div(Class("flex min-w-0 flex-1 flex-col"),
			Div(Class("flex items-baseline gap-2"),
				P(Class("truncate text-sm font-medium"), Text(person.Name)),
				Span(Class("ml-auto shrink-0 text-xs text-zinc-400 tabular-nums dark:text-zinc-500"), Text("#"+Grouped(p.Index+1))),
			),
			P(Class("truncate text-sm text-zinc-500 dark:text-zinc-400"), Text(person.Role+" · "+person.City)),
			If(person.Note != "",
				P(Class("mt-2 rounded-md bg-zinc-50 px-3 py-2 text-sm text-zinc-600 dark:bg-zinc-800/60 dark:text-zinc-300"), Text(person.Note))),
		),
	)
}

// Preview is the screen of `tela snapshot` and `tela inspect`.
func Preview() tela.Node { return tela.C(Directory) }

func main() {
	tela.Main(tela.App{
		Title:  "Tela — virtual list",
		AppID:  "dev.telaui.virtuallist",
		Width:  1280,
		Height: 800,
		Root:   Directory,
	})
}
