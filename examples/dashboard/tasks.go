package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/examples/dashboard/icons"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// Sprint is the strip above the tasks: how many are done, as a bar whose
// width is a spring read in a layout closure (tela.WidthFn), the team and
// the New task button.
type Sprint struct {
	Done, Total int
	Compose     tela.Setter[int]
	Full        bool // To do cannot take another card
}

func (p Sprint) Build() tela.Node {
	pct := float32(0)
	if p.Total > 0 {
		pct = float32(p.Done) / float32(p.Total)
	}
	w := tela.UseSpring(pct, tela.Stiffness(120), tela.Damping(20))
	track := tela.UseElementRef()
	size := tela.UseSize(track)
	return Div(Class("flex items-center gap-8 rounded-xl border border-border bg-card px-5 py-4"),
		Div(
			H2(Class(eyebrow), Text("Sprint 24")),
			Div(Class("mt-1 text-lg font-light tracking-tight"), Text("Oct 28 - Nov 8"))),
		Div(Class("flex-1"),
			Div(Class("mb-2 flex justify-between text-xs text-muted-foreground"),
				Span(Textf("%d of %d done", p.Done, p.Total)),
				Span(Class("tabular-nums"), Textf("%.0f%%", pct*100))),
			Div(Ref(track), Role("progressbar"), AriaLabel("Sprint progress"), AriaValueNow(float64(pct*100)),
				Class("relative h-1.5 overflow-hidden rounded-full bg-border"),
				Div(Class("absolute inset-y-0 left-0 rounded-full bg-primary"),
					Style(tela.WidthFn(func() float32 { return w.Get() * size.Get().Width }))))),
		Div(Class("flex -space-x-1.5"), AriaHidden(true),
			Map([]string{"AL", "MK", "JS", "OM", "WK"}, func(i string) tela.Node {
				return Div(Key(i), Class("rounded-full ring-2 ring-background"), kit.Avatar{Fallback: i, Class: "size-8"})
			})),
		kit.Button{Icon: icons.Plus, Disabled: p.Full, OnClick: func() { p.Compose.Set(todo) }, Children: Text("New task")})
}

// TasksTable is the tasks as a kit.Table: a checkbox that finishes a task
// (its card moves to Done, and back to To do), and columns that sort.
// The table's columns are Sortable: it shows and reports the sort, and the
// page orders the rows (rows), so that finished tasks stay last. Every
// cell has AnimateLayout: sorting, or finishing a task, lays the table out
// once and each cell slides to its new place, a transform that only
// composites.
type TasksTable struct {
	Board Board
	Tasks Tasks
	Set   tela.Setter[Board]
}

func (p TasksTable) Build() tela.Node {
	sort, setSort := tela.UseState(kit.SortState{Column: "priority", Direction: kit.Ascending})
	slide := func(Row) tela.Child { return AnimateLayout() }
	finish := func(r Row) {
		p.Set.Update(func(b Board) Board {
			if r.Done {
				return b.Move(r.ID, Target{Col: todo, Valid: true})
			}
			return b.Move(r.ID, Target{Col: done, Valid: true})
		})
	}
	return Div(Class("overflow-hidden rounded-xl border border-border bg-card"),
		kit.Table[Row]{
			Rows: rows(p.Board, p.Tasks, sort), RowKey: func(r Row) any { return r.ID }, AriaLabel: "Tasks",
			Sort: &sort, OnSortChange: setSort.Set,
			Columns: []kit.Column[Row]{
				{ID: "done", Header: "Done", HeaderContent: Span(Class("sr-only"), Text("Done")), Width: "2.75rem", CellProps: slide,
					Cell: func(r Row) tela.Node {
						d := r.Done
						return kit.Checkbox{Checked: &d, OnCheckedChange: func(bool) { finish(r) }, AriaLabel: r.Task.Title}
					}},
				{ID: "key", Header: "ID", Width: "6rem", CellProps: slide, Cell: func(r Row) tela.Node {
					return Span(Class("font-mono text-xs text-muted-foreground"), Text(r.Task.Key))
				}},
				{ID: "title", Header: "Task", Width: "minmax(16rem,1fr)", Sortable: true, CellProps: slide, Cell: func(r Row) tela.Node {
					return Span(Class("text-[13px] font-medium transition-colors", when(r.Done, "text-muted-foreground line-through")), Text(r.Task.Title))
				}},
				{ID: "tag", Header: "Team", Width: "8rem", Sortable: true, CellProps: slide, Cell: func(r Row) tela.Node {
					return Span(Class("text-[13px] text-muted-foreground"), Text(r.Task.Tag))
				}},
				{ID: "priority", Header: "Priority", Width: "8.5rem", Sortable: true, CellProps: slide, Cell: func(r Row) tela.Node {
					t := r.Task
					return Span(Class("inline-flex items-center gap-2 text-[13px]"),
						Icon(priorityIcon[t.Priority], Class("size-3.5", when(t.Priority == 1, "text-primary"))), Text(priorityNames[t.Priority]))
				}},
				{ID: "due", Header: "Due", Width: "6.5rem", Align: kit.AlignRight, Sortable: true, CellProps: slide, Cell: func(r Row) tela.Node {
					return Span(Class("text-[13px] text-muted-foreground"), Text(r.Task.Due))
				}},
			},
		})
}

// TasksView is the tasks page: the sprint strip and the table or the
// board. The board and the tasks are the page's state (Dashboard), so both
// layouts and the palette see the same tasks.
type TasksView struct {
	Mode     string
	Board    Board
	Tasks    Tasks
	Set      tela.Setter[Board]
	SetTasks tela.Setter[Tasks]
	Compose  tela.Setter[int]
}

func (p TasksView) Build() tela.Node {
	return Fragment(
		Sprint{Done: p.Board[done].Len(), Total: p.Board.Count(), Compose: p.Compose, Full: p.Board.Full()[todo]},
		IfElse(p.Mode == "board",
			BoardView{Board: p.Board, Tasks: p.Tasks, Set: p.Set, SetTasks: p.SetTasks, Compose: p.Compose},
			TasksTable{Board: p.Board, Tasks: p.Tasks, Set: p.Set}),
	)
}
